package core

import (
	"sync/atomic"
	"unsafe"
)

// SPSCRingBuffer implements a lock-free single-producer single-consumer ring buffer
type SPSCRingBuffer struct {
	buffer []unsafe.Pointer
	mask   uint64
	_      [56]byte // Cache line padding

	// Producer cache line
	head     uint64
	cachedTail uint64
	_        [48]byte // Cache line padding

	// Consumer cache line  
	tail     uint64
	cachedHead uint64
	_        [48]byte // Cache line padding
}

// NewSPSCRingBuffer creates a new lock-free ring buffer
// Size must be a power of 2
func NewSPSCRingBuffer(size uint64) *SPSCRingBuffer {
	if size&(size-1) != 0 {
		panic("Size must be power of 2")
	}

	return &SPSCRingBuffer{
		buffer: make([]unsafe.Pointer, size),
		mask:   size - 1,
	}
}

// Enqueue adds an item to the buffer (producer side)
func (rb *SPSCRingBuffer) Enqueue(item *RawMessage) bool {
	head := atomic.LoadUint64(&rb.head)
	nextHead := head + 1

	// Check if queue is full
	if nextHead-rb.cachedTail > uint64(len(rb.buffer)) {
		// Refresh cached tail
		rb.cachedTail = atomic.LoadUint64(&rb.tail)
		if nextHead-rb.cachedTail > uint64(len(rb.buffer)) {
			return false // Still full
		}
	}

	// Store item
	rb.buffer[head&rb.mask] = unsafe.Pointer(item)
	
	// Release store of head
	atomic.StoreUint64(&rb.head, nextHead)
	return true
}

// Dequeue removes an item from the buffer (consumer side)
func (rb *SPSCRingBuffer) Dequeue() *RawMessage {
	tail := atomic.LoadUint64(&rb.tail)

	// Check if queue is empty
	if tail >= rb.cachedHead {
		// Refresh cached head
		rb.cachedHead = atomic.LoadUint64(&rb.head)
		if tail >= rb.cachedHead {
			return nil // Still empty
		}
	}

	// Load item
	item := (*RawMessage)(rb.buffer[tail&rb.mask])

	// Release store of tail
	atomic.StoreUint64(&rb.tail, tail+1)
	return item
}

// Peek returns the next item without removing it
func (rb *SPSCRingBuffer) Peek() *RawMessage {
	tail := atomic.LoadUint64(&rb.tail)

	// Check if queue is empty
	if tail >= rb.cachedHead {
		// Refresh cached head
		rb.cachedHead = atomic.LoadUint64(&rb.head)
		if tail >= rb.cachedHead {
			return nil // Still empty
		}
	}

	// Load item without advancing tail
	return (*RawMessage)(rb.buffer[tail&rb.mask])
}

// Depth returns approximate queue depth
func (rb *SPSCRingBuffer) Depth() uint64 {
	head := atomic.LoadUint64(&rb.head)
	tail := atomic.LoadUint64(&rb.tail)
	return head - tail
}

// IsEmpty returns true if queue is empty
func (rb *SPSCRingBuffer) IsEmpty() bool {
	head := atomic.LoadUint64(&rb.head)
	tail := atomic.LoadUint64(&rb.tail)
	return head == tail
}

// IsFull returns true if queue is full
func (rb *SPSCRingBuffer) IsFull() bool {
	head := atomic.LoadUint64(&rb.head)
	tail := atomic.LoadUint64(&rb.tail)
	return head-tail >= uint64(len(rb.buffer))
}

// Reset clears the queue
func (rb *SPSCRingBuffer) Reset() {
	atomic.StoreUint64(&rb.head, 0)
	atomic.StoreUint64(&rb.tail, 0)
	rb.cachedHead = 0
	rb.cachedTail = 0
}