package orderbook

import (
	"sync"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
)

// OrderAllocator provides memory pool management for orders
type OrderAllocator struct {
	orderPool    sync.Pool
	messagePool  sync.Pool
	levelPool    sync.Pool
}

// NewOrderAllocator creates a new order allocator
func NewOrderAllocator() *OrderAllocator {
	return &OrderAllocator{
		orderPool: sync.Pool{
			New: func() interface{} {
				return &core.Order{}
			},
		},
		messagePool: sync.Pool{
			New: func() interface{} {
				return &core.RawMessage{}
			},
		},
		levelPool: sync.Pool{
			New: func() interface{} {
				return &core.PriceLevel{}
			},
		},
	}
}

// AllocateOrder gets an order from the pool
func (oa *OrderAllocator) AllocateOrder() *core.Order {
	order := oa.orderPool.Get().(*core.Order)
	// Reset fields to zero values
	*order = core.Order{}
	return order
}

// FreeOrder returns an order to the pool
func (oa *OrderAllocator) FreeOrder(order *core.Order) {
	// Clear any references to prevent memory leaks
	order.PrevOrder = nil
	order.NextOrder = nil
	order.PriceLevel = nil
	oa.orderPool.Put(order)
}

// AllocateMessage gets a raw message from the pool
func (oa *OrderAllocator) AllocateMessage() *core.RawMessage {
	msg := oa.messagePool.Get().(*core.RawMessage)
	*msg = core.RawMessage{}
	return msg
}

// FreeMessage returns a message to the pool
func (oa *OrderAllocator) FreeMessage(msg *core.RawMessage) {
	msg.Payload = nil // Clear slice reference
	oa.messagePool.Put(msg)
}

// AllocatePriceLevel gets a price level from the pool
func (oa *OrderAllocator) AllocatePriceLevel() *core.PriceLevel {
	level := oa.levelPool.Get().(*core.PriceLevel)
	*level = core.PriceLevel{}
	return level
}

// FreePriceLevel returns a price level to the pool
func (oa *OrderAllocator) FreePriceLevel(level *core.PriceLevel) {
	level.FirstOrder = nil
	level.LastOrder = nil
	oa.levelPool.Put(level)
}