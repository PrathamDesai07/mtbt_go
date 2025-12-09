package core

import (
	"encoding/binary"
	"fmt"
)

// MTBT v6.7 Message Type Constants
const (
	MTBTMsgTypeOrder       = 'N' // New Order
	MTBTMsgTypeModify      = 'M' // Modify Order  
	MTBTMsgTypeCancel      = 'X' // Cancel Order
	MTBTMsgTypeTrade       = 'T' // Trade
	MTBTMsgTypeSpreadOrder = 'G' // Spread Order
	MTBTMsgTypeSpreadTrade = 'S' // Spread Trade
	MTBTMsgTypeTradeCancel = 'C' // Trade Cancel
	MTBTMsgTypeHeartbeat   = 'H' // Heartbeat
)

// MTBT Order Message Structure (MTBT v6.7 specification)
type MTBTOrderMessage struct {
	MessageType   byte   // 'N', 'M', 'X'
	Token         uint32 // Security token
	OrderNumber   uint64 // Unique order number
	Price         uint32 // Price in paise (divide by 100 for rupees)
	Quantity      uint32 // Order quantity
	DisclosedQty  uint32 // Disclosed quantity
	Side          byte   // 'B' for Buy, 'S' for Sell
	OrderType     byte   // Order type
	TimeStamp     uint64 // Timestamp
}

// MTBT Trade Message Structure (MTBT v6.7 specification)
type MTBTTradeMessage struct {
	MessageType  byte   // 'T'
	Token        uint32 // Security token
	TradeNumber  uint64 // Unique trade number
	Price        uint32 // Trade price in paise
	Quantity     uint32 // Trade quantity
	BuyerOrder   uint64 // Buyer order number
	SellerOrder  uint64 // Seller order number
	TimeStamp    uint64 // Timestamp
}

// MTBT Spread Order Message Structure (MTBT v6.7 specification)
type MTBTSpreadOrderMessage struct {
	MessageType   byte   // 'G'
	Token         uint32 // Spread contract token
	OrderNumber   uint64 // Unique order number
	Price         uint32 // Price in paise
	Quantity      uint32 // Order quantity
	DisclosedQty  uint32 // Disclosed quantity
	Side          byte   // 'B' for Buy, 'S' for Sell
	OrderType     byte   // Order type
	TimeStamp     uint64 // Timestamp
}

// MTBT Spread Trade Message Structure (MTBT v6.7 specification)
type MTBTSpreadTradeMessage struct {
	MessageType  byte   // 'S'
	Token        uint32 // Spread contract token
	TradeNumber  uint64 // Unique trade number
	Price        uint32 // Trade price in paise
	Quantity     uint32 // Trade quantity
	BuyerOrder   uint64 // Buyer order number
	SellerOrder  uint64 // Seller order number
	TimeStamp    uint64 // Timestamp
}

// MTBT Trade Cancel Message Structure (MTBT v6.7 specification)
type MTBTTradeCancelMessage struct {
	MessageType  byte   // 'C'
	Token        uint32 // Security token
	TradeNumber  uint64 // Trade number to cancel
	TimeStamp    uint64 // Timestamp
}

// MTBT Heartbeat Message Structure
type MTBTHeartbeatMessage struct {
	MessageType byte   // 'H'
	TimeStamp   uint64 // Timestamp
}

// MTBTDecoder provides methods to decode MTBT v6.7 protocol messages
type MTBTDecoder struct{}

// NewMTBTDecoder creates a new MTBT decoder
func NewMTBTDecoder() *MTBTDecoder {
	return &MTBTDecoder{}
}

// DecodeOrderMessage decodes MTBT order messages (N, M, X)
func (d *MTBTDecoder) DecodeOrderMessage(payload []byte) (*MTBTOrderMessage, error) {
	if len(payload) < 37 { // Minimum size for order message
		return nil, ErrInvalidMessageSize
	}

	msg := &MTBTOrderMessage{
		MessageType:  payload[0],
		Token:        binary.LittleEndian.Uint32(payload[1:5]),
		OrderNumber:  binary.LittleEndian.Uint64(payload[5:13]),
		Price:        binary.LittleEndian.Uint32(payload[13:17]),
		Quantity:     binary.LittleEndian.Uint32(payload[17:21]),
		DisclosedQty: binary.LittleEndian.Uint32(payload[21:25]),
		Side:         payload[25],
		OrderType:    payload[26],
		TimeStamp:    binary.LittleEndian.Uint64(payload[27:35]),
	}

	return msg, nil
}

// DecodeTradeMessage decodes MTBT trade messages (T)
func (d *MTBTDecoder) DecodeTradeMessage(payload []byte) (*MTBTTradeMessage, error) {
	if len(payload) < 41 { // Minimum size for trade message
		return nil, ErrInvalidMessageSize
	}

	msg := &MTBTTradeMessage{
		MessageType: payload[0],
		Token:       binary.LittleEndian.Uint32(payload[1:5]),
		TradeNumber: binary.LittleEndian.Uint64(payload[5:13]),
		Price:       binary.LittleEndian.Uint32(payload[13:17]),
		Quantity:    binary.LittleEndian.Uint32(payload[17:21]),
		BuyerOrder:  binary.LittleEndian.Uint64(payload[21:29]),
		SellerOrder: binary.LittleEndian.Uint64(payload[29:37]),
		TimeStamp:   binary.LittleEndian.Uint64(payload[37:45]),
	}

	return msg, nil
}

// DecodeSpreadOrderMessage decodes MTBT spread order messages (G)
func (d *MTBTDecoder) DecodeSpreadOrderMessage(payload []byte) (*MTBTSpreadOrderMessage, error) {
	if len(payload) < 37 { // Minimum size for spread order message
		return nil, ErrInvalidMessageSize
	}

	msg := &MTBTSpreadOrderMessage{
		MessageType:  payload[0],
		Token:        binary.LittleEndian.Uint32(payload[1:5]),
		OrderNumber:  binary.LittleEndian.Uint64(payload[5:13]),
		Price:        binary.LittleEndian.Uint32(payload[13:17]),
		Quantity:     binary.LittleEndian.Uint32(payload[17:21]),
		DisclosedQty: binary.LittleEndian.Uint32(payload[21:25]),
		Side:         payload[25],
		OrderType:    payload[26],
		TimeStamp:    binary.LittleEndian.Uint64(payload[27:35]),
	}

	return msg, nil
}

// DecodeSpreadTradeMessage decodes MTBT spread trade messages (S)
func (d *MTBTDecoder) DecodeSpreadTradeMessage(payload []byte) (*MTBTSpreadTradeMessage, error) {
	if len(payload) < 41 { // Minimum size for spread trade message
		return nil, ErrInvalidMessageSize
	}

	msg := &MTBTSpreadTradeMessage{
		MessageType: payload[0],
		Token:       binary.LittleEndian.Uint32(payload[1:5]),
		TradeNumber: binary.LittleEndian.Uint64(payload[5:13]),
		Price:       binary.LittleEndian.Uint32(payload[13:17]),
		Quantity:    binary.LittleEndian.Uint32(payload[17:21]),
		BuyerOrder:  binary.LittleEndian.Uint64(payload[21:29]),
		SellerOrder: binary.LittleEndian.Uint64(payload[29:37]),
		TimeStamp:   binary.LittleEndian.Uint64(payload[37:45]),
	}

	return msg, nil
}

// DecodeTradeCancelMessage decodes MTBT trade cancel messages (C)
func (d *MTBTDecoder) DecodeTradeCancelMessage(payload []byte) (*MTBTTradeCancelMessage, error) {
	if len(payload) < 17 { // Minimum size for trade cancel message
		return nil, ErrInvalidMessageSize
	}

	msg := &MTBTTradeCancelMessage{
		MessageType: payload[0],
		Token:       binary.LittleEndian.Uint32(payload[1:5]),
		TradeNumber: binary.LittleEndian.Uint64(payload[5:13]),
		TimeStamp:   binary.LittleEndian.Uint64(payload[13:21]),
	}

	return msg, nil
}

// DecodeHeartbeatMessage decodes MTBT heartbeat messages (H)
func (d *MTBTDecoder) DecodeHeartbeatMessage(payload []byte) (*MTBTHeartbeatMessage, error) {
	if len(payload) < 9 { // Minimum size for heartbeat message
		return nil, ErrInvalidMessageSize
	}

	msg := &MTBTHeartbeatMessage{
		MessageType: payload[0],
		TimeStamp:   binary.LittleEndian.Uint64(payload[1:9]),
	}

	return msg, nil
}

// DecodeMessage is a generic decoder that routes to specific decoders based on message type
func (d *MTBTDecoder) DecodeMessage(payload []byte) (interface{}, error) {
	if len(payload) < 1 {
		return nil, ErrInvalidMessageSize
	}

	messageType := payload[0]
	
	switch messageType {
	case MTBTMsgTypeOrder, MTBTMsgTypeModify, MTBTMsgTypeCancel:
		return d.DecodeOrderMessage(payload)
	case MTBTMsgTypeTrade:
		return d.DecodeTradeMessage(payload)
	case MTBTMsgTypeSpreadOrder:
		return d.DecodeSpreadOrderMessage(payload)
	case MTBTMsgTypeSpreadTrade:
		return d.DecodeSpreadTradeMessage(payload)
	case MTBTMsgTypeTradeCancel:
		return d.DecodeTradeCancelMessage(payload)
	case MTBTMsgTypeHeartbeat:
		return d.DecodeHeartbeatMessage(payload)
	default:
		return nil, ErrUnknownMessageType
	}
}

// ConvertPriceToRupees converts price from paise to rupees
func ConvertPriceToRupees(priceInPaise uint32) float64 {
	return float64(priceInPaise) / 100.0
}

// Error constants
var (
	ErrInvalidMessageSize  = fmt.Errorf("invalid message size")
	ErrUnknownMessageType  = fmt.Errorf("unknown message type")
)