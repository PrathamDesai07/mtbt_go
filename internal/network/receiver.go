package network

import (
	"encoding/binary"
	"fmt"
	"net"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
)

// ReceiverCore handles UDP packet reception on a single core
type ReceiverCore struct {
	coreID   int
	queue    *core.SPSCRingBuffer
	socket   *net.UDPConn
	fd       int
	metrics  *core.Metrics
	running  int64
	buffer   []byte // Preallocated receive buffer
}

// NewReceiverCore creates a new receiver core
func NewReceiverCore(coreID int, multicastIP string, port int) (*ReceiverCore, error) {
	queue := core.NewSPSCRingBuffer(core.QueueSize)
	
	// Create UDP socket
	addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", multicastIP, port))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	socket, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to create UDP socket: %w", err)
	}

	// Get file descriptor for low-level tuning
	file, err := socket.File()
	if err != nil {
		return nil, fmt.Errorf("failed to get socket file descriptor: %w", err)
	}
	fd := int(file.Fd())

	receiver := &ReceiverCore{
		coreID:  coreID,
		queue:   queue,
		socket:  socket,
		fd:      fd,
		metrics: &core.Metrics{},
		buffer:  make([]byte, 9000), // Jumbo frame support
	}

	// Tune socket for low latency
	err = receiver.tuneSocket()
	if err != nil {
		return nil, fmt.Errorf("failed to tune socket: %w", err)
	}

	return receiver, nil
}

// tuneSocket applies low-latency socket optimizations
func (rc *ReceiverCore) tuneSocket() error {
	// Set large receive buffer
	err := syscall.SetsockoptInt(rc.fd, syscall.SOL_SOCKET, 
		syscall.SO_RCVBUF, 134*1024*1024)
	if err != nil {
		return fmt.Errorf("failed to set SO_RCVBUF: %w", err)
	}

	// Enable SO_REUSEPORT for RSS
	err = syscall.SetsockoptInt(rc.fd, syscall.SOL_SOCKET, 
		15, 1) // SO_REUSEPORT = 15
	if err != nil {
		return fmt.Errorf("failed to set SO_REUSEPORT: %w", err)
	}

	// Set SO_BUSY_POLL for low latency (50μs)
	err = syscall.SetsockoptInt(rc.fd, syscall.SOL_SOCKET, 
		46, 50) // SO_BUSY_POLL = 46
	if err != nil {
		// Not critical, continue without busy polling
		fmt.Printf("Warning: failed to set SO_BUSY_POLL: %v\n", err)
	}

	return nil
}

// Start begins packet reception on the assigned core
func (rc *ReceiverCore) Start() error {
	if !atomic.CompareAndSwapInt64(&rc.running, 0, 1) {
		return fmt.Errorf("receiver core %d already running", rc.coreID)
	}

	// Pin to CPU core
	err := rc.pinToCPU()
	if err != nil {
		return fmt.Errorf("failed to pin to CPU: %w", err)
	}

	// Start receive loop
	go rc.receiveLoop()
	return nil
}

// Stop terminates packet reception
func (rc *ReceiverCore) Stop() {
	atomic.StoreInt64(&rc.running, 0)
	rc.socket.Close()
}

// receiveLoop is the main packet processing loop
func (rc *ReceiverCore) receiveLoop() {
	runtime.LockOSThread() // Ensure thread stays on assigned core

	for atomic.LoadInt64(&rc.running) == 1 {
		n, err := rc.socket.Read(rc.buffer)
		if err != nil {
			if atomic.LoadInt64(&rc.running) == 0 {
				break // Shutdown requested
			}
			atomic.AddUint64(&rc.metrics.PacketsDropped, 1)
			continue
		}

		atomic.AddUint64(&rc.metrics.PacketsReceived, 1)

		// Parse minimal header for fast processing
		msg := rc.parsePacket(rc.buffer[:n])
		if msg == nil {
			atomic.AddUint64(&rc.metrics.PacketsDropped, 1)
			continue
		}

		// Enqueue for processing
		if !rc.queue.Enqueue(msg) {
			// Queue full - drop packet
			atomic.AddUint64(&rc.metrics.PacketsDropped, 1)
			continue
		}

		atomic.AddUint64(&rc.metrics.PacketsProcessed, 1)
	}
}

// parsePacket extracts key fields from MTBT packet
func (rc *ReceiverCore) parsePacket(data []byte) *core.RawMessage {
	if len(data) < 8 { // Minimum header size
		return nil
	}

	// Parse stream header (little endian)
	header := core.StreamHeader{
		MsgLen:   binary.LittleEndian.Uint16(data[0:2]),
		StreamID: binary.LittleEndian.Uint16(data[2:4]),
		SeqNo:    binary.LittleEndian.Uint32(data[4:8]),
	}

	if len(data) < int(header.MsgLen) {
		return nil // Invalid packet
	}

	if len(data) < 9 { // Need at least message type
		return nil
	}

	// Create message with minimal processing
	msg := &core.RawMessage{
		StreamID:  header.StreamID,
		SeqNo:     header.SeqNo,
		MsgType:   data[8], // First byte after header
		Payload:   data[8:header.MsgLen], // Copy payload
		Timestamp: core.RDTSC(),
	}

	return msg
}

// pinToCPU pins the current goroutine to the assigned CPU core
func (rc *ReceiverCore) pinToCPU() error {
	runtime.LockOSThread()

	// Linux-specific CPU affinity setting
	var mask [128]uintptr
	mask[rc.coreID/64] |= 1 << (rc.coreID % 64)

	_, _, errno := syscall.Syscall(syscall.SYS_SCHED_SETAFFINITY,
		uintptr(0), // Current thread
		uintptr(len(mask)*8),
		uintptr(unsafe.Pointer(&mask[0])))

	if errno != 0 {
		return fmt.Errorf("failed to set CPU affinity: %v", errno)
	}

	return nil
}

// GetQueue returns the receiver's output queue
func (rc *ReceiverCore) GetQueue() *core.SPSCRingBuffer {
	return rc.queue
}

// GetMetrics returns receiver performance metrics
func (rc *ReceiverCore) GetMetrics() core.Metrics {
	return *rc.metrics
}

// GetQueueDepth returns current queue utilization
func (rc *ReceiverCore) GetQueueDepth() uint64 {
	return rc.queue.Depth()
}