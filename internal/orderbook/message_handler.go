package orderbook

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
)

// MessageHandler processes MTBT messages and updates orderbooks
type MessageHandler struct {
	contracts   map[uint32]*core.Contract // token -> contract
	allocator   *OrderAllocator
	sequencer   *SequenceMerger
	metrics     *core.Metrics
	running     int64
}

// NewMessageHandler creates a new message handler
func NewMessageHandler() *MessageHandler {
	return &MessageHandler{
		contracts: make(map[uint32]*core.Contract, core.MaxContracts),
		allocator: NewOrderAllocator(),
		sequencer: NewSequenceMerger(),
		metrics:   &core.Metrics{},
	}
}

// Start begins message processing on core 5
func (mh *MessageHandler) Start(queues [4]*core.SPSCRingBuffer) error {
	if !atomic.CompareAndSwapInt64(&mh.running, 0, 1) {
		return fmt.Errorf("message handler already running")
	}

	// Pin to core 5
	err := mh.pinToCPU(4) // Core 5 (0-indexed)
	if err != nil {
		return fmt.Errorf("failed to pin to CPU: %w", err)
	}

	// Initialize sequencer with input queues
	mh.sequencer.SetQueues(queues)

	// Start processing loop
	go mh.processLoop()
	return nil
}

// Stop terminates message processing
func (mh *MessageHandler) Stop() {
	atomic.StoreInt64(&mh.running, 0)
}

// processLoop is the main message processing loop
func (mh *MessageHandler) processLoop() {
	runtime.LockOSThread() // Stay on assigned core

	for atomic.LoadInt64(&mh.running) == 1 {
		msg := mh.sequencer.GetNextMessage()
		if msg == nil {
			runtime.Gosched() // Brief yield if no messages
			continue
		}

		start := time.Now()
		mh.ProcessMessage(msg)
		latency := uint64(time.Since(start).Nanoseconds())
		
		atomic.AddUint64(&mh.metrics.PacketsProcessed, 1)
		atomic.StoreUint64(&mh.metrics.ProcessingLatencyNs, latency)
	}
}

// ProcessMessage handles a single MTBT message
func (mh *MessageHandler) ProcessMessage(msg *core.RawMessage) {
	switch msg.MsgType {
	case core.MsgTypeNewOrder:
		mh.handleNewOrder(msg)
	case core.MsgTypeModifyOrder:
		mh.handleModifyOrder(msg)
	case core.MsgTypeCancelOrder:
		mh.handleCancelOrder(msg)
	case core.MsgTypeTrade:
		mh.handleTrade(msg)
	case core.MsgTypeTradeCancel:
		mh.handleTradeCancel(msg)
	case core.MsgTypeSpreadOrder:
		mh.handleNewOrder(msg) // Same logic as N
	case core.MsgTypeSpreadModify:
		mh.handleModifyOrder(msg) // Same logic as M
	case core.MsgTypeSpreadCancel:
		mh.handleCancelOrder(msg) // Same logic as X
	case core.MsgTypeHeartbeat:
		mh.handleHeartbeat(msg)
	default:
		// Unknown message type - ignore
	}
}

// handleNewOrder processes new order messages (N/G)
func (mh *MessageHandler) handleNewOrder(msg *core.RawMessage) {
	order := mh.parseOrderMessage(msg)
	if order == nil {
		return
	}

	contract := mh.getContract(order.Token)
	
	// Create new order
	newOrder := mh.allocator.AllocateOrder()
	*newOrder = core.Order{
		OrderID:      order.OrderID,
		Token:        order.Token,
		Side:         order.Side,
		Price:        order.Price,
		Quantity:     order.Quantity,
		LastUpdateTs: order.Timestamp,
	}

	// Add to order map
	contract.OrdersById.Put(order.OrderID, newOrder)

	// Add to appropriate price tree
	var tree *PriceTree
	if order.Side == core.SideBuy {
		tree = contract.Bids
	} else {
		tree = contract.Asks
	}

	priceLevel := tree.GetOrCreate(order.Price)
	AddOrderToLevel(priceLevel, newOrder)

	contract.LastUpdate = msg.Timestamp
}

// handleModifyOrder processes order modification messages (M/H)
func (mh *MessageHandler) handleModifyOrder(msg *core.RawMessage) {
	orderMsg := mh.parseOrderMessage(msg)
	if orderMsg == nil {
		return
	}

	contract := mh.getContract(orderMsg.Token)
	existingOrder := contract.OrdersById.Get(orderMsg.OrderID)
	
	if existingOrder == nil {
		// Treat as new order per NSE specification
		mh.handleNewOrder(msg)
		return
	}

	// Handle price change
	if existingOrder.Price != orderMsg.Price {
		RemoveOrderFromLevel(existingOrder)
		existingOrder.Price = orderMsg.Price

		var tree *PriceTree
		if existingOrder.Side == core.SideBuy {
			tree = contract.Bids
		} else {
			tree = contract.Asks
		}

		newLevel := tree.GetOrCreate(orderMsg.Price)
		AddOrderToLevel(newLevel, existingOrder)
	}

	// Handle quantity change
	if existingOrder.Quantity != orderMsg.Quantity {
		oldQty := existingOrder.Quantity
		existingOrder.Quantity = orderMsg.Quantity
		if existingOrder.PriceLevel != nil {
			existingOrder.PriceLevel.TotalQty += int64(orderMsg.Quantity - oldQty)
		}
	}

	existingOrder.LastUpdateTs = msg.Timestamp
}

// handleCancelOrder processes order cancellation messages (X/J)
func (mh *MessageHandler) handleCancelOrder(msg *core.RawMessage) {
	orderMsg := mh.parseOrderMessage(msg)
	if orderMsg == nil {
		return
	}

	contract := mh.getContract(orderMsg.Token)
	order := contract.OrdersById.Get(orderMsg.OrderID)
	
	if order == nil {
		return // Order not found - ignore per NSE spec
	}

	// Remove from price level
	RemoveOrderFromLevel(order)

	// Remove from order map
	contract.OrdersById.Delete(orderMsg.OrderID)

	// Return to memory pool
	mh.allocator.FreeOrder(order)
}

// handleTrade processes trade messages (T/K)
func (mh *MessageHandler) handleTrade(msg *core.RawMessage) {
	trade := mh.parseTradeMessage(msg)
	if trade == nil {
		return
	}

	contract := mh.getContract(trade.Token)

	// Process buy side
	if trade.BuyOrderID != 0 {
		buyOrder := contract.OrdersById.Get(trade.BuyOrderID)
		if buyOrder != nil {
			mh.reduceOrderQuantity(contract, buyOrder, trade.Quantity)
		}
	}

	// Process sell side
	if trade.SellOrderID != 0 {
		sellOrder := contract.OrdersById.Get(trade.SellOrderID)
		if sellOrder != nil {
			mh.reduceOrderQuantity(contract, sellOrder, trade.Quantity)
		}
	}
}

// handleTradeCancel processes trade cancellation messages (C)
func (mh *MessageHandler) handleTradeCancel(msg *core.RawMessage) {
	trade := mh.parseTradeMessage(msg)
	if trade == nil {
		return
	}

	contract := mh.getContract(trade.Token)

	// Restore buy order quantity
	if trade.BuyOrderID != 0 {
		mh.restoreOrderQuantity(contract, trade.BuyOrderID, trade.Quantity, trade.Price, core.SideBuy)
	}

	// Restore sell order quantity
	if trade.SellOrderID != 0 {
		mh.restoreOrderQuantity(contract, trade.SellOrderID, trade.Quantity, trade.Price, core.SideSell)
	}
}

// handleHeartbeat processes heartbeat messages (Z)
func (mh *MessageHandler) handleHeartbeat(msg *core.RawMessage) {
	// Update last sequence number for stream
	if len(msg.Payload) >= 4 {
		lastSeq := binary.LittleEndian.Uint32(msg.Payload[1:5])
		mh.sequencer.UpdateLastSeq(msg.StreamID, lastSeq)
	}
}

// reduceOrderQuantity reduces order quantity due to trade
func (mh *MessageHandler) reduceOrderQuantity(contract *core.Contract, order *core.Order, tradeQty int32) {
	order.Quantity -= tradeQty
	if order.PriceLevel != nil {
		order.PriceLevel.TotalQty -= int64(tradeQty)
	}

	if order.Quantity <= 0 {
		// Fully filled - remove from book
		RemoveOrderFromLevel(order)
		contract.OrdersById.Delete(order.OrderID)
		mh.allocator.FreeOrder(order)
	}
}

// restoreOrderQuantity restores order quantity due to trade cancellation
func (mh *MessageHandler) restoreOrderQuantity(contract *core.Contract, orderID uint64, quantity int32, price int32, side byte) {
	order := contract.OrdersById.Get(orderID)
	
	if order != nil {
		// Order still exists - restore quantity
		order.Quantity += quantity
		if order.PriceLevel != nil {
			order.PriceLevel.TotalQty += int64(quantity)
		}
	} else {
		// Order was fully consumed - recreate it
		restoredOrder := mh.allocator.AllocateOrder()
		*restoredOrder = core.Order{
			OrderID:  orderID,
			Token:    contract.Token,
			Side:     side,
			Price:    price,
			Quantity: quantity,
		}

		contract.OrdersById.Put(orderID, restoredOrder)

		var tree *PriceTree
		if side == core.SideBuy {
			tree = contract.Bids
		} else {
			tree = contract.Asks
		}

		priceLevel := tree.GetOrCreate(price)
		AddOrderToLevel(priceLevel, restoredOrder)
	}
}

// parseOrderMessage parses order-type messages
func (mh *MessageHandler) parseOrderMessage(msg *core.RawMessage) *core.OrderMessage {
	if len(msg.Payload) < 25 { // Minimum order message size
		return nil
	}

	// Parse fields according to MTBT specification
	timestamp := binary.LittleEndian.Uint64(msg.Payload[1:9])
	orderID := binary.LittleEndian.Uint64(msg.Payload[9:17])
	token := binary.LittleEndian.Uint32(msg.Payload[17:21])
	side := msg.Payload[21]
	price := int32(binary.LittleEndian.Uint32(msg.Payload[22:26]))
	quantity := int32(binary.LittleEndian.Uint32(msg.Payload[26:30]))

	return &core.OrderMessage{
		Timestamp: timestamp,
		OrderID:   orderID,
		Token:     token,
		Side:      side,
		Price:     price,
		Quantity:  quantity,
	}
}

// parseTradeMessage parses trade-type messages
func (mh *MessageHandler) parseTradeMessage(msg *core.RawMessage) *core.TradeMessage {
	if len(msg.Payload) < 29 { // Minimum trade message size
		return nil
	}

	// Parse fields according to MTBT specification
	timestamp := binary.LittleEndian.Uint64(msg.Payload[1:9])
	buyOrderID := binary.LittleEndian.Uint64(msg.Payload[9:17])
	sellOrderID := binary.LittleEndian.Uint64(msg.Payload[17:25])
	token := binary.LittleEndian.Uint32(msg.Payload[25:29])
	price := int32(binary.LittleEndian.Uint32(msg.Payload[29:33]))
	quantity := int32(binary.LittleEndian.Uint32(msg.Payload[33:37]))

	return &core.TradeMessage{
		Timestamp:   timestamp,
		BuyOrderID:  buyOrderID,
		SellOrderID: sellOrderID,
		Token:       token,
		Price:       price,
		Quantity:    quantity,
	}
}

// getContract retrieves or creates a contract
func (mh *MessageHandler) getContract(token uint32) *core.Contract {
	contract, exists := mh.contracts[token]
	if !exists {
		// Create new contract
		contract = &core.Contract{
			Token:      token,
			OrdersById: NewOrderMap(1024), // Start with reasonable size
			Bids:       NewPriceTree(false), // Descending for bids
			Asks:       NewPriceTree(true),  // Ascending for asks
		}
		mh.contracts[token] = contract
	}
	return contract
}

// pinToCPU pins current thread to specified CPU
func (mh *MessageHandler) pinToCPU(cpuID int) error {
	runtime.LockOSThread()
	
	// Implementation would use syscalls for CPU affinity
	// This is a placeholder
	return nil
}

// GetContract returns contract by token (for external access)
func (mh *MessageHandler) GetContract(token uint32) *core.Contract {
	return mh.contracts[token]
}

// GetMetrics returns processing metrics
func (mh *MessageHandler) GetMetrics() core.Metrics {
	return *mh.metrics
}