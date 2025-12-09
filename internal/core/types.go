package core

import (
	"unsafe"
)

// MTBT Protocol Constants
const (
	// Message Types
	MsgTypeNewOrder       = 'N'
	MsgTypeModifyOrder    = 'M'
	MsgTypeCancelOrder    = 'X'
	MsgTypeTrade          = 'T'
	MsgTypeTradeCancel    = 'C'
	MsgTypeSpreadOrder    = 'G'
	MsgTypeSpreadModify   = 'H'
	MsgTypeSpreadCancel   = 'J'
	MsgTypeSpreadTrade    = 'K'
	MsgTypeHeartbeat      = 'Z'
	MsgTypeRecoveryReq    = 'R'
	MsgTypeRecoveryResp   = 'Y'
	MsgTypeSnapshotReq    = 'O'
	MsgTypeSnapshotResp   = 'B'

	// Side Types
	SideBuy  = 'B'
	SideSell = 'S'

	// Configuration
	MaxStreams         = 18
	MaxContracts       = 85000
	QueueSize          = 524288  // 512K entries, must be power of 2
	SpinWaitTimeoutNs  = 1000000 // 1ms
	MaxSpinWaitGap     = 8
)

// StreamHeader represents the MTBT packet header
type StreamHeader struct {
	MsgLen   uint16 // Total packet length
	StreamID uint16 // Stream identifier
	SeqNo    uint32 // Sequence number (UINT for FO segment)
}

// RawMessage represents a decoded MTBT message
type RawMessage struct {
	StreamID  uint16
	SeqNo     uint32
	MsgType   byte
	Payload   []byte
	Timestamp uint64 // RDTSC timestamp
	_         [24]byte // Cache line padding
}

// Order represents an individual order in the orderbook
type Order struct {
	OrderID      uint64
	Token        uint32
	Side         byte
	Price        int32  // In paise
	Quantity     int32
	LastUpdateTs uint64

	// Intrusive linking for price levels
	PrevOrder  *Order
	NextOrder  *Order
	PriceLevel *PriceLevel
	_          [16]byte // Cache line padding
}

// PriceLevel represents a price level in the orderbook
type PriceLevel struct {
	Price      int32
	TotalQty   int64
	OrderCount int32
	FirstOrder *Order // Head of FIFO list
	LastOrder  *Order // Tail of FIFO list
	_          [24]byte // Cache line padding
}

// OrderMapInterface defines the order map interface
type OrderMapInterface interface {
	Get(uint64) *Order
	Put(uint64, *Order) bool
	Delete(uint64) bool
}

// PriceTreeInterface defines the price tree interface  
type PriceTreeInterface interface {
	GetOrCreate(int32) *PriceLevel
	GetBestBid() *PriceLevel
	GetBestAsk() *PriceLevel
	Remove(int32)
}

// Contract represents the orderbook for a single instrument
type Contract struct {
	Token       uint32
	OrdersById  OrderMapInterface
	Bids        PriceTreeInterface
	Asks        PriceTreeInterface
	LastUpdate  uint64
	_           [32]byte // Cache line padding
}

// OrderMessage represents parsed order data
type OrderMessage struct {
	Timestamp uint64
	OrderID   uint64
	Token     uint32
	Side      byte
	Price     int32
	Quantity  int32
}

// TradeMessage represents parsed trade data
type TradeMessage struct {
	Timestamp   uint64
	BuyOrderID  uint64
	SellOrderID uint64
	Token       uint32
	Price       int32
	Quantity    int32
}

// SnapshotHeader represents snapshot recovery header
type SnapshotHeader struct {
	TransCode    uint16
	Size         uint32
	NumRecords   uint32
	LastSeqNo    uint32
	StreamID     uint16
	_            [2]byte // Padding
}

// RecoveryRequest represents tick recovery request
type RecoveryRequest struct {
	MessageType byte
	StreamID    uint16
	StartSeqNo  uint32
	EndSeqNo    uint32
	_           [3]byte // Padding
}

// RecoveryResponse represents recovery response status
type RecoveryResponse struct {
	Header StreamHeader
	MsgType byte
	Status  byte // 'S' success, 'E' error
	_       [6]byte // Padding
}

// Performance metrics structure
type Metrics struct {
	PacketsReceived     uint64
	PacketsProcessed    uint64
	PacketsDropped      uint64
	GapsDetected        uint64
	RecoveriesTriggered uint64
	QueueDepth          [4]uint64
	ProcessingLatencyNs uint64
	_                   [16]byte // Cache line padding
}

// Fast hash function for order IDs
func FastHash64(key uint64) uint64 {
	key ^= key >> 33
	key *= 0xff51afd7ed558ccd
	key ^= key >> 33
	key *= 0xc4ceb9fe1a85ec53
	key ^= key >> 33
	return key
}

// RDTSC for high-precision timestamps
func RDTSC() uint64 {
	// Assembly implementation would go here
	// For now, using a placeholder
	return uint64(0)
}

// Memory barrier operations
func CompilerBarrier() {
	// Prevents compiler reordering
}

func LoadBarrier() {
	// CPU load barrier
}

func StoreBarrier() {
	// CPU store barrier
}

// Cache line size for alignment
const CacheLineSize = 64

// Align pointer to cache line boundary
func AlignToCacheLine(ptr unsafe.Pointer) unsafe.Pointer {
	addr := uintptr(ptr)
	aligned := (addr + CacheLineSize - 1) &^ (CacheLineSize - 1)
	return unsafe.Pointer(aligned)
}

// Forward declarations removed - using concrete types instead