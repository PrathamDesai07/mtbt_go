package orderbook

import (
	"runtime"
	"sync/atomic"
	"time"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
)

// SequenceMerger handles strict sequence ordering across multiple streams
type SequenceMerger struct {
	queues         []*core.SPSCRingBuffer
	lastAppliedSeq map[uint16]uint32  // streamID -> last applied sequence
	expectedSeq    map[uint16]uint32  // streamID -> next expected sequence
	gapBuffer      map[uint16][]*core.RawMessage // Out-of-order messages
	metrics        *core.Metrics
}

// NewSequenceMerger creates a new sequence merger
func NewSequenceMerger() *SequenceMerger {
	return &SequenceMerger{
		lastAppliedSeq: make(map[uint16]uint32),
		expectedSeq:    make(map[uint16]uint32),
		gapBuffer:      make(map[uint16][]*core.RawMessage),
		metrics:        &core.Metrics{},
	}
}

// SetQueues configures input queues from receiver cores
func (sm *SequenceMerger) SetQueues(queues []*core.SPSCRingBuffer) {
	sm.queues = queues
	
	// Initialize sequence tracking for each stream
	for i := range queues {
		// Stream IDs will be determined dynamically from incoming messages
		_ = i // Suppress unused variable warning for now
	}
}

// GetNextMessage returns the next message in strict sequence order
func (sm *SequenceMerger) GetNextMessage() *core.RawMessage {
	// Find message with oldest timestamp across all queues
	var oldest *core.RawMessage
	var oldestQueue int = -1

	// Check all available queues
	for i, queue := range sm.queues {
		if msg := queue.Peek(); msg != nil {
			if oldest == nil || msg.SeqNo < oldest.SeqNo || 
			   (msg.SeqNo == oldest.SeqNo && msg.StreamID < oldest.StreamID) {
				oldest = msg
				oldestQueue = i
			}
		}
	}

	if oldest == nil {
		return nil // No messages available
	}

	streamID := oldest.StreamID
	expected := sm.expectedSeq[streamID]

	// Initialize sequence tracking for new streams
	if expected == 0 {
		sm.expectedSeq[streamID] = 1
		expected = 1
	}

	if oldest.SeqNo == expected {
		// Perfect sequence - process immediately
		sm.queues[oldestQueue].Dequeue()
		sm.lastAppliedSeq[streamID] = oldest.SeqNo
		sm.expectedSeq[streamID] = oldest.SeqNo + 1
		
		// Check if any buffered messages can now be processed
		sm.processBufferedMessages(streamID)
		
		return oldest
		
	} else if oldest.SeqNo > expected {
		// Gap detected - need recovery or wait
		gapSize := oldest.SeqNo - expected
		
		if gapSize <= core.MaxSpinWaitGap {
			// Small gap - spin wait briefly
			if sm.spinWaitForGap(streamID, expected, oldest.SeqNo-1) {
				// Gap filled during wait - try again
				return sm.GetNextMessage()
			}
		}
		
		// Large gap or timeout - trigger recovery
		sm.handleGap(streamID, expected, oldest.SeqNo-1)
		return nil
		
	} else {
		// Old duplicate or out-of-order - ignore
		sm.queues[oldestQueue].Dequeue()
		return sm.GetNextMessage() // Recurse
	}
}

// spinWaitForGap waits briefly for missing messages
func (sm *SequenceMerger) spinWaitForGap(streamID uint16, startSeq, endSeq uint32) bool {
	deadline := time.Now().Add(time.Duration(core.SpinWaitTimeoutNs))
	
	for time.Now().Before(deadline) {
		if sm.checkForMissingMessages(streamID, startSeq, endSeq) {
			return true // Gap filled
		}
		runtime.Gosched() // Brief yield
	}
	
	return false // Timeout
}

// checkForMissingMessages looks for missing sequences in input queues
func (sm *SequenceMerger) checkForMissingMessages(streamID uint16, startSeq, endSeq uint32) bool {
	// Scan all queues for missing messages
	for i := 0; i < 4; i++ {
		depth := sm.queues[i].Depth()
		for j := uint64(0); j < depth && j < 100; j++ { // Limit scan depth
			// This would require queue peeking at arbitrary positions
			// For now, simplified implementation
		}
	}
	return false
}

// handleGap processes sequence gaps
func (sm *SequenceMerger) handleGap(streamID uint16, startSeq, endSeq uint32) {
	atomic.AddUint64(&sm.metrics.GapsDetected, 1)
	
	// In production, this would trigger tick recovery
	// For now, advance expected sequence to handle the gap
	sm.expectedSeq[streamID] = endSeq + 1
	atomic.AddUint64(&sm.metrics.RecoveriesTriggered, 1)
}

// processBufferedMessages processes any out-of-order messages that are now in sequence
func (sm *SequenceMerger) processBufferedMessages(streamID uint16) {
	buffer, exists := sm.gapBuffer[streamID]
	if !exists || len(buffer) == 0 {
		return
	}

	expected := sm.expectedSeq[streamID]
	processed := 0

	// Process messages that are now in sequence
	for i, msg := range buffer {
		if msg != nil && msg.SeqNo == expected {
			// This message is now in sequence
			sm.lastAppliedSeq[streamID] = msg.SeqNo
			sm.expectedSeq[streamID] = msg.SeqNo + 1
			expected = msg.SeqNo + 1
			
			buffer[i] = nil // Mark as processed
			processed++
		}
	}

	if processed > 0 {
		// Compact buffer to remove processed messages
		sm.compactBuffer(streamID)
	}
}

// compactBuffer removes nil entries from gap buffer
func (sm *SequenceMerger) compactBuffer(streamID uint16) {
	buffer := sm.gapBuffer[streamID]
	writeIndex := 0
	
	for _, msg := range buffer {
		if msg != nil {
			buffer[writeIndex] = msg
			writeIndex++
		}
	}
	
	// Truncate buffer
	sm.gapBuffer[streamID] = buffer[:writeIndex]
}

// SetLastAppliedSeq sets the last applied sequence for a stream
func (sm *SequenceMerger) SetLastAppliedSeq(streamID uint16, seqNo uint32) {
	sm.lastAppliedSeq[streamID] = seqNo
}

// SetExpectedSeq sets the expected sequence for a stream
func (sm *SequenceMerger) SetExpectedSeq(streamID uint16, seqNo uint32) {
	sm.expectedSeq[streamID] = seqNo
}

// GetLastAppliedSeq returns the last applied sequence for a stream
func (sm *SequenceMerger) GetLastAppliedSeq(streamID uint16) uint32 {
	return sm.lastAppliedSeq[streamID]
}

// UpdateLastSeq updates last sequence from heartbeat
func (sm *SequenceMerger) UpdateLastSeq(streamID uint16, lastSeq uint32) {
	// Update tracking based on heartbeat information
	current := sm.lastAppliedSeq[streamID]
	if lastSeq > current {
		// There might be missing messages
		gap := lastSeq - current
		if gap > 1 {
			sm.handleGap(streamID, current+1, lastSeq-1)
		}
	}
}

// GetGapCounts returns gap statistics per stream
func (sm *SequenceMerger) GetGapCounts() map[uint16]uint64 {
	// Return gap statistics for monitoring
	result := make(map[uint16]uint64)
	for streamID := range sm.lastAppliedSeq {
		result[streamID] = 0 // Simplified - would track actual gaps
	}
	return result
}