package orderbook

import (
	"sync"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
)

const (
	BucketSize = 8  // Orders per hash bucket
	MaxLevel   = 16 // Skip list levels
)

// OrderMap implements a high-performance hash map for orders
type OrderMap struct {
	buckets []OrderBucket
	mask    uint64
	size    int
	pool    sync.Pool
}

// Embed OrderMap methods to make it compatible with core.OrderMap
func (om *OrderMap) Get(orderID uint64) *core.Order {
	return om.get(orderID)
}

func (om *OrderMap) Put(orderID uint64, order *core.Order) bool {
	return om.put(orderID, order)
}

func (om *OrderMap) Delete(orderID uint64) bool {
	return om.delete(orderID)
}

type OrderBucket struct {
	orders [BucketSize]*core.Order
	count  int
}

// NewOrderMap creates a new order map
func NewOrderMap(capacity int) *OrderMap {
	// Round up to next power of 2
	size := 1
	for size < capacity {
		size <<= 1
	}

	om := &OrderMap{
		buckets: make([]OrderBucket, size),
		mask:    uint64(size - 1),
		size:    size,
	}

	om.pool = sync.Pool{
		New: func() interface{} {
			return &core.Order{}
		},
	}

	return om
}

// get retrieves an order by ID (internal method)
func (om *OrderMap) get(orderID uint64) *core.Order {
	hash := core.FastHash64(orderID)
	bucket := &om.buckets[hash&om.mask]

	for i := 0; i < bucket.count; i++ {
		if bucket.orders[i] != nil && bucket.orders[i].OrderID == orderID {
			return bucket.orders[i]
		}
	}
	return nil
}

// put stores an order (internal method)
func (om *OrderMap) put(orderID uint64, order *core.Order) bool {
	hash := core.FastHash64(orderID)
	bucket := &om.buckets[hash&om.mask]

	// Check if order already exists
	for i := 0; i < bucket.count; i++ {
		if bucket.orders[i] != nil && bucket.orders[i].OrderID == orderID {
			bucket.orders[i] = order
			return true
		}
	}

	// Add new order if space available
	if bucket.count < BucketSize {
		bucket.orders[bucket.count] = order
		bucket.count++
		return true
	}

	return false // Bucket full
}

// delete removes an order (internal method)
func (om *OrderMap) delete(orderID uint64) bool {
	hash := core.FastHash64(orderID)
	bucket := &om.buckets[hash&om.mask]

	for i := 0; i < bucket.count; i++ {
		if bucket.orders[i] != nil && bucket.orders[i].OrderID == orderID {
			// Shift remaining orders down
			for j := i; j < bucket.count-1; j++ {
				bucket.orders[j] = bucket.orders[j+1]
			}
			bucket.orders[bucket.count-1] = nil
			bucket.count--
			return true
		}
	}
	return false
}

// PriceTree implements a skip list for price levels
type PriceTree struct {
	header    *PriceNode
	maxLevel  int
	ascending bool // true for asks, false for bids
	levels    [][]*PriceNode
	pool      sync.Pool
}

// Embed PriceTree methods to make it compatible
func (pt *PriceTree) GetOrCreate(price int32) *core.PriceLevel {
	return pt.getOrCreate(price)
}

func (pt *PriceTree) GetBestBid() *core.PriceLevel {
	return pt.getBestBid()
}

func (pt *PriceTree) GetBestAsk() *core.PriceLevel {
	return pt.getBestAsk()
}

func (pt *PriceTree) Remove(price int32) {
	pt.remove(price)
}

type PriceNode struct {
	price   int32
	level   *core.PriceLevel
	forward []*PriceNode
}

// NewPriceTree creates a new price tree
func NewPriceTree(ascending bool) *PriceTree {
	header := &PriceNode{
		forward: make([]*PriceNode, MaxLevel+1),
	}

	pt := &PriceTree{
		header:    header,
		maxLevel:  0,
		ascending: ascending,
		levels:    make([][]*PriceNode, MaxLevel+1),
	}

	pt.pool = sync.Pool{
		New: func() interface{} {
			return &core.PriceLevel{}
		},
	}

	return pt
}

// getOrCreate retrieves or creates a price level (internal method)
func (pt *PriceTree) getOrCreate(price int32) *core.PriceLevel {
	node := pt.find(price)
	if node != nil && node.price == price {
		return node.level
	}

	// Create new price level
	level := pt.pool.Get().(*core.PriceLevel)
	*level = core.PriceLevel{
		Price:      price,
		TotalQty:   0,
		OrderCount: 0,
		FirstOrder: nil,
		LastOrder:  nil,
	}

	pt.insert(price, level)
	return level
}

// find locates a price node
func (pt *PriceTree) find(price int32) *PriceNode {
	current := pt.header

	for i := pt.maxLevel; i >= 0; i-- {
		for current.forward[i] != nil && pt.comparePrice(current.forward[i].price, price) {
			current = current.forward[i]
		}
	}

	if current.forward[0] != nil {
		return current.forward[0]
	}
	return nil
}

// insert adds a new price level
func (pt *PriceTree) insert(price int32, level *core.PriceLevel) {
	update := make([]*PriceNode, MaxLevel+1)
	current := pt.header

	// Find insertion point
	for i := pt.maxLevel; i >= 0; i-- {
		for current.forward[i] != nil && pt.comparePrice(current.forward[i].price, price) {
			current = current.forward[i]
		}
		update[i] = current
	}

	// Create new node
	newLevel := pt.randomLevel()
	if newLevel > pt.maxLevel {
		for i := pt.maxLevel + 1; i <= newLevel; i++ {
			update[i] = pt.header
		}
		pt.maxLevel = newLevel
	}

	node := &PriceNode{
		price:   price,
		level:   level,
		forward: make([]*PriceNode, newLevel+1),
	}

	// Insert node
	for i := 0; i <= newLevel; i++ {
		node.forward[i] = update[i].forward[i]
		update[i].forward[i] = node
	}
}

// comparePrice compares prices based on tree direction
func (pt *PriceTree) comparePrice(a, b int32) bool {
	if pt.ascending {
		return a < b // Ascending for asks
	}
	return a > b // Descending for bids
}

// randomLevel generates random skip list level
func (pt *PriceTree) randomLevel() int {
	level := 0
	for level < MaxLevel && (fastRandom()&1) == 1 {
		level++
	}
	return level
}

// Simple fast random number generator
var rngState uint64 = 1

func fastRandom() uint64 {
	rngState = rngState*1103515245 + 12345
	return rngState
}

// AddOrderToLevel adds an order to a price level (FIFO)
func AddOrderToLevel(level *core.PriceLevel, order *core.Order) {
	order.PriceLevel = level
	order.NextOrder = nil
	order.PrevOrder = level.LastOrder

	if level.LastOrder != nil {
		level.LastOrder.NextOrder = order
	} else {
		level.FirstOrder = order
	}

	level.LastOrder = order
	level.OrderCount++
	level.TotalQty += int64(order.Quantity)
}

// RemoveOrderFromLevel removes an order from a price level
func RemoveOrderFromLevel(order *core.Order) {
	level := order.PriceLevel
	if level == nil {
		return
	}

	// Update linked list
	if order.PrevOrder != nil {
		order.PrevOrder.NextOrder = order.NextOrder
	} else {
		level.FirstOrder = order.NextOrder
	}

	if order.NextOrder != nil {
		order.NextOrder.PrevOrder = order.PrevOrder
	} else {
		level.LastOrder = order.PrevOrder
	}

	// Update level stats
	level.OrderCount--
	level.TotalQty -= int64(order.Quantity)

	// Clear order references
	order.PriceLevel = nil
	order.NextOrder = nil
	order.PrevOrder = nil
}

// getBestBid returns the highest bid price (internal method)
func (pt *PriceTree) getBestBid() *core.PriceLevel {
	if pt.ascending {
		return nil // This is an ask tree
	}

	if pt.header.forward[0] != nil {
		return pt.header.forward[0].level
	}
	return nil
}

// getBestAsk returns the lowest ask price (internal method)
func (pt *PriceTree) getBestAsk() *core.PriceLevel {
	if !pt.ascending {
		return nil // This is a bid tree
	}

	if pt.header.forward[0] != nil {
		return pt.header.forward[0].level
	}
	return nil
}

// remove deletes a price level if empty (internal method)
func (pt *PriceTree) remove(price int32) {
	update := make([]*PriceNode, MaxLevel+1)
	current := pt.header

	// Find node to delete
	for i := pt.maxLevel; i >= 0; i-- {
		for current.forward[i] != nil && pt.comparePrice(current.forward[i].price, price) {
			current = current.forward[i]
		}
		update[i] = current
	}

	target := current.forward[0]
	if target == nil || target.price != price {
		return // Not found
	}

	// Remove node
	for i := 0; i <= pt.maxLevel && update[i].forward[i] == target; i++ {
		update[i].forward[i] = target.forward[i]
	}

	// Update max level
	for pt.maxLevel > 0 && pt.header.forward[pt.maxLevel] == nil {
		pt.maxLevel--
	}

	// Return price level to pool
	pt.pool.Put(target.level)
}