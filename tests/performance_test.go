package tests

import (
	"runtime"
	"testing"
	"time"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
	"github.com/PrathamDesai07/mtbt_go/internal/orderbook"
)

// BenchmarkOrderProcessing tests orderbook processing performance
func BenchmarkOrderProcessing(b *testing.B) {
	handler := orderbook.NewMessageHandler()
	
	// Create test order message
	testPayload := make([]byte, 30)
	testPayload[0] = core.MsgTypeNewOrder // 'N'
	// Fill with test order data...
	
	msg := &core.RawMessage{
		StreamID:  1,
		SeqNo:     1,
		MsgType:   core.MsgTypeNewOrder,
		Payload:   testPayload,
		Timestamp: uint64(time.Now().UnixNano()),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg.SeqNo = uint32(i + 1)
		handler.ProcessMessage(msg)
	}
}

// BenchmarkSPSCQueue tests lock-free queue performance
func BenchmarkSPSCQueue(b *testing.B) {
	queue := core.NewSPSCRingBuffer(core.QueueSize)
	
	msg := &core.RawMessage{
		StreamID: 1,
		SeqNo:    1,
		MsgType:  core.MsgTypeNewOrder,
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Producer
			for !queue.Enqueue(msg) {
				// Retry if queue full
			}
			
			// Consumer
			for queue.Dequeue() == nil {
				// Retry if queue empty
			}
		}
	})
}

// TestOrderbookConsistency verifies orderbook state integrity
func TestOrderbookConsistency(t *testing.T) {
	handler := orderbook.NewMessageHandler()
	
	// Test scenario: Add order, modify price, trade, cancel
	testCases := []struct {
		msgType  byte
		orderID  uint64
		token    uint32
		side     byte
		price    int32
		quantity int32
	}{
		{core.MsgTypeNewOrder, 12345, 1001, core.SideBuy, 10000, 100},    // New buy order
		{core.MsgTypeNewOrder, 12346, 1001, core.SideSell, 10100, 50},   // New sell order
		{core.MsgTypeModifyOrder, 12345, 1001, core.SideBuy, 9950, 150}, // Modify buy price
		{core.MsgTypeTrade, 0, 1001, 0, 10000, 25},                      // Trade 25 shares
		{core.MsgTypeCancelOrder, 12346, 1001, core.SideSell, 0, 0},     // Cancel sell order
	}

	for i, tc := range testCases {
		payload := createTestPayload(tc.msgType, tc.orderID, tc.token, tc.side, tc.price, tc.quantity)
		msg := &core.RawMessage{
			StreamID:  1,
			SeqNo:     uint32(i + 1),
			MsgType:   tc.msgType,
			Payload:   payload,
			Timestamp: uint64(time.Now().UnixNano()),
		}
		
		handler.ProcessMessage(msg)
	}

	// Verify final orderbook state
	contract := handler.GetContract(1001)
	if contract == nil {
		t.Fatal("Contract not found")
	}

	// Check that modified order exists with correct price and quantity
	order := contract.OrdersById.Get(12345)
	if order == nil {
		t.Fatal("Order 12345 should exist")
	}
	
	if order.Price != 9950 {
		t.Errorf("Expected price 9950, got %d", order.Price)
	}
	
	if order.Quantity != 125 { // 150 - 25 traded
		t.Errorf("Expected quantity 125, got %d", order.Quantity)
	}

	// Check that cancelled order is removed
	cancelledOrder := contract.OrdersById.Get(12346)
	if cancelledOrder != nil {
		t.Error("Order 12346 should be cancelled")
	}
}

// TestBurstCapacity tests system under high load
func TestBurstCapacity(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping burst capacity test in short mode")
	}

	handler := orderbook.NewMessageHandler()
	queues := [4]*core.SPSCRingBuffer{
		core.NewSPSCRingBuffer(core.QueueSize),
		core.NewSPSCRingBuffer(core.QueueSize),
		core.NewSPSCRingBuffer(core.QueueSize),
		core.NewSPSCRingBuffer(core.QueueSize),
	}

	err := handler.Start(queues)
	if err != nil {
		t.Fatalf("Failed to start handler: %v", err)
	}
	defer handler.Stop()

	// Generate burst load
	numMessages := 1000000
	startTime := time.Now()

	for i := 0; i < numMessages; i++ {
		payload := createTestPayload(core.MsgTypeNewOrder, uint64(i), uint32(i%1000), core.SideBuy, 10000, 100)
		msg := &core.RawMessage{
			StreamID:  uint16(i % 4 + 1),
			SeqNo:     uint32(i + 1),
			MsgType:   core.MsgTypeNewOrder,
			Payload:   payload,
			Timestamp: uint64(time.Now().UnixNano()),
		}

		queueIndex := i % 4
		for !queues[queueIndex].Enqueue(msg) {
			// Retry if queue full
			time.Sleep(time.Microsecond)
		}
	}

	duration := time.Since(startTime)
	rate := float64(numMessages) / duration.Seconds()

	t.Logf("Processed %d messages in %v", numMessages, duration)
	t.Logf("Processing rate: %.0f messages/second", rate)

	if rate < 500000 { // 500K messages/sec minimum
		t.Errorf("Processing rate too low: %.0f msg/sec, expected ≥500K", rate)
	}
}

// TestMemoryUsage validates memory allocation patterns
func TestMemoryUsage(t *testing.T) {
	handler := orderbook.NewMessageHandler()
	
	// Measure initial memory
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Process many orders
	numOrders := 10000
	for i := 0; i < numOrders; i++ {
		payload := createTestPayload(core.MsgTypeNewOrder, uint64(i), 1001, core.SideBuy, int32(10000+i), 100)
		msg := &core.RawMessage{
			StreamID:  1,
			SeqNo:     uint32(i + 1),
			MsgType:   core.MsgTypeNewOrder,
			Payload:   payload,
			Timestamp: uint64(time.Now().UnixNano()),
		}
		handler.ProcessMessage(msg)
	}

	// Measure final memory
	runtime.GC()
	runtime.ReadMemStats(&m2)

	allocatedMB := float64(m2.Alloc-m1.Alloc) / 1024 / 1024
	t.Logf("Memory allocated for %d orders: %.2f MB", numOrders, allocatedMB)

	// Expect reasonable memory usage (< 100MB for 10K orders)
	if allocatedMB > 100 {
		t.Errorf("Memory usage too high: %.2f MB for %d orders", allocatedMB, numOrders)
	}
}

// createTestPayload creates a test MTBT message payload
func createTestPayload(msgType byte, orderID uint64, token uint32, side byte, price, quantity int32) []byte {
	payload := make([]byte, 30)
	payload[0] = msgType
	
	// Simplified payload creation - in real implementation,
	// would use binary.LittleEndian to encode fields properly
	return payload
}

// TestSequenceOrdering verifies strict sequence processing
func TestSequenceOrdering(t *testing.T) {
	sequencer := orderbook.NewSequenceMerger()
	
	queues := [4]*core.SPSCRingBuffer{
		core.NewSPSCRingBuffer(1024),
		core.NewSPSCRingBuffer(1024),
		core.NewSPSCRingBuffer(1024),
		core.NewSPSCRingBuffer(1024),
	}
	sequencer.SetQueues(queues)

	// Add messages in mixed order across queues
	messages := []*core.RawMessage{
		{StreamID: 1, SeqNo: 1, MsgType: core.MsgTypeNewOrder},
		{StreamID: 1, SeqNo: 3, MsgType: core.MsgTypeNewOrder}, // Gap
		{StreamID: 1, SeqNo: 2, MsgType: core.MsgTypeNewOrder}, // Fill gap
		{StreamID: 2, SeqNo: 1, MsgType: core.MsgTypeNewOrder},
	}

	// Distribute messages across queues
	queues[0].Enqueue(messages[0]) // Stream 1, Seq 1
	queues[1].Enqueue(messages[1]) // Stream 1, Seq 3 (gap)
	queues[2].Enqueue(messages[2]) // Stream 1, Seq 2 (fill)
	queues[3].Enqueue(messages[3]) // Stream 2, Seq 1

	// Should get messages in sequence order
	expectedSeqs := []uint32{1, 2, 3, 1}
	expectedStreams := []uint16{1, 1, 1, 2}

	for i := 0; i < 4; i++ {
		msg := sequencer.GetNextMessage()
		if msg == nil {
			t.Fatalf("Expected message %d, got nil", i)
		}

		if msg.SeqNo != expectedSeqs[i] || msg.StreamID != expectedStreams[i] {
			t.Errorf("Message %d: expected Stream %d Seq %d, got Stream %d Seq %d",
				i, expectedStreams[i], expectedSeqs[i], msg.StreamID, msg.SeqNo)
		}
	}
}