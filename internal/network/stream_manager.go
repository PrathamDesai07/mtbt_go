package network

import (
	"fmt"
	"log"
	"runtime"
	"sync"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
)

// StreamConfig holds configuration for a single MTBT stream
type StreamConfig struct {
	StreamID     int
	MulticastIP  string
	Port         int
	Bandwidth    string // For monitoring purposes
}

// FOStreamConfig contains all 18 FO stream configurations from NSE MSD67677
var FOStreamConfig = []StreamConfig{
	{StreamID: 1, MulticastIP: "239.70.70.41", Port: 17741, Bandwidth: "20 Mbps"},
	{StreamID: 2, MulticastIP: "239.70.70.42", Port: 17742, Bandwidth: "6 Mbps"},
	{StreamID: 3, MulticastIP: "239.70.70.43", Port: 17743, Bandwidth: "40 Mbps"},
	{StreamID: 4, MulticastIP: "239.70.70.44", Port: 17744, Bandwidth: "18 Mbps"},
	{StreamID: 5, MulticastIP: "239.70.70.45", Port: 17745, Bandwidth: "40 Mbps"},
	{StreamID: 6, MulticastIP: "239.70.70.46", Port: 17746, Bandwidth: "18 Mbps"},
	{StreamID: 7, MulticastIP: "239.70.70.47", Port: 17747, Bandwidth: "40 Mbps"},
	{StreamID: 8, MulticastIP: "239.70.70.48", Port: 17748, Bandwidth: "40 Mbps"},
	{StreamID: 9, MulticastIP: "239.70.70.49", Port: 17749, Bandwidth: "40 Mbps"},
	{StreamID: 10, MulticastIP: "239.70.70.50", Port: 17750, Bandwidth: "40 Mbps"},
	{StreamID: 11, MulticastIP: "239.70.70.61", Port: 17761, Bandwidth: "40 Mbps"},
	{StreamID: 12, MulticastIP: "239.70.70.62", Port: 17762, Bandwidth: "40 Mbps"},
	{StreamID: 13, MulticastIP: "239.70.70.63", Port: 17763, Bandwidth: "40 Mbps"},
	{StreamID: 14, MulticastIP: "239.70.70.64", Port: 17764, Bandwidth: "40 Mbps"},
	{StreamID: 15, MulticastIP: "239.70.70.65", Port: 17765, Bandwidth: "40 Mbps"},
	{StreamID: 16, MulticastIP: "239.70.70.66", Port: 17766, Bandwidth: "40 Mbps"},
	{StreamID: 17, MulticastIP: "239.70.70.67", Port: 17767, Bandwidth: "40 Mbps"},
	{StreamID: 18, MulticastIP: "239.70.70.68", Port: 17768, Bandwidth: "40 Mbps"},
}

// StreamManager manages multiple MTBT receiver cores
type StreamManager struct {
	receivers   []*ReceiverCore
	config      []StreamConfig
	coreMapping []int
	running     bool
	mu          sync.RWMutex
}

// NewStreamManager creates a new stream manager with CPU core distribution
func NewStreamManager(streams []StreamConfig, availableCores int) (*StreamManager, error) {
	if len(streams) == 0 {
		return nil, fmt.Errorf("no streams configured")
	}

	if availableCores < 1 {
		availableCores = runtime.NumCPU() - 1 // Reserve one core for orderbook
	}

	sm := &StreamManager{
		receivers:   make([]*ReceiverCore, len(streams)),
		config:      streams,
		coreMapping: distributeCores(len(streams), availableCores),
	}

	// Create receiver cores
	for i, streamConfig := range streams {
		coreID := sm.coreMapping[i]
		
		receiver, err := NewReceiverCore(coreID, streamConfig.MulticastIP, streamConfig.Port)
		if err != nil {
			return nil, fmt.Errorf("failed to create receiver for stream %d: %w", 
				streamConfig.StreamID, err)
		}
		
		// Set stream ID for proper identification
		receiver.SetStreamID(streamConfig.StreamID)
		sm.receivers[i] = receiver
		
		log.Printf("Created receiver for FO Stream %d (IP: %s, Port: %d) on CPU %d", 
			streamConfig.StreamID, streamConfig.MulticastIP, streamConfig.Port, coreID)
	}

	return sm, nil
}

// distributeCores distributes stream receivers across available CPU cores
func distributeCores(numStreams, availableCores int) []int {
	mapping := make([]int, numStreams)
	
	for i := 0; i < numStreams; i++ {
		// Distribute streams round-robin across available cores
		mapping[i] = i % availableCores
	}
	
	return mapping
}

// Start starts all receiver cores
func (sm *StreamManager) Start() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if sm.running {
		return fmt.Errorf("stream manager already running")
	}

	log.Printf("Starting stream manager with %d FO streams...", len(sm.receivers))
	
	// Start all receivers
	for i, receiver := range sm.receivers {
		err := receiver.Start()
		if err != nil {
			// Stop already started receivers
			for j := 0; j < i; j++ {
				sm.receivers[j].Stop()
			}
			return fmt.Errorf("failed to start receiver %d: %w", i, err)
		}
	}
	
	sm.running = true
	log.Printf("All %d FO stream receivers started successfully", len(sm.receivers))
	return nil
}

// Stop stops all receiver cores
func (sm *StreamManager) Stop() {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	
	if !sm.running {
		return
	}
	
	log.Println("Stopping stream manager...")
	
	for i, receiver := range sm.receivers {
		if receiver != nil {
			log.Printf("Stopping FO Stream %d receiver...", sm.config[i].StreamID)
			receiver.Stop()
		}
	}
	
	sm.running = false
	log.Println("Stream manager stopped")
}

// GetQueues returns all receiver queues for the message handler
func (sm *StreamManager) GetQueues() []*core.SPSCRingBuffer {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	queues := make([]*core.SPSCRingBuffer, len(sm.receivers))
	for i, receiver := range sm.receivers {
		queues[i] = receiver.GetQueue()
	}
	
	return queues
}

// GetMetrics returns aggregated metrics from all receivers
func (sm *StreamManager) GetMetrics() StreamManagerMetrics {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	
	metrics := StreamManagerMetrics{
		StreamCount:     len(sm.receivers),
		StreamMetrics:   make([]core.Metrics, len(sm.receivers)),
		QueueDepths:     make([]uint64, len(sm.receivers)),
		CoreMapping:     sm.coreMapping,
		TotalBandwidth:  calculateTotalBandwidth(sm.config),
	}
	
	for i, receiver := range sm.receivers {
		metrics.StreamMetrics[i] = receiver.GetMetrics()
		metrics.QueueDepths[i] = receiver.GetQueueDepth()
		
		// Aggregate totals
		metrics.TotalPacketsReceived += metrics.StreamMetrics[i].PacketsReceived
		metrics.TotalPacketsProcessed += metrics.StreamMetrics[i].PacketsProcessed
		metrics.TotalPacketsDropped += metrics.StreamMetrics[i].PacketsDropped
	}
	
	return metrics
}

// StreamManagerMetrics holds comprehensive metrics for all streams
type StreamManagerMetrics struct {
	StreamCount           int
	StreamMetrics         []core.Metrics
	QueueDepths          []uint64
	CoreMapping          []int
	TotalPacketsReceived  uint64
	TotalPacketsProcessed uint64
	TotalPacketsDropped   uint64
	TotalBandwidth       string
}

// calculateTotalBandwidth estimates total bandwidth from all streams
func calculateTotalBandwidth(configs []StreamConfig) string {
	total := 0
	for _, config := range configs {
		switch config.Bandwidth {
		case "6 Mbps":
			total += 6
		case "18 Mbps":
			total += 18
		case "20 Mbps":
			total += 20
		case "40 Mbps":
			total += 40
		}
	}
	return fmt.Sprintf("%d Mbps", total)
}

// PrintMetrics displays detailed metrics for all streams
func (metrics StreamManagerMetrics) Print() {
	fmt.Printf("\n=== MTBT Stream Manager Metrics ===\n")
	fmt.Printf("Active FO Streams: %d\n", metrics.StreamCount)
	fmt.Printf("Total Bandwidth: %s\n", metrics.TotalBandwidth)
	fmt.Printf("Total Packets Received: %d\n", metrics.TotalPacketsReceived)
	fmt.Printf("Total Packets Processed: %d\n", metrics.TotalPacketsProcessed)
	fmt.Printf("Total Packets Dropped: %d\n", metrics.TotalPacketsDropped)
	
	if metrics.TotalPacketsReceived > 0 {
		dropRate := float64(metrics.TotalPacketsDropped) / float64(metrics.TotalPacketsReceived) * 100
		fmt.Printf("Overall Drop Rate: %.4f%%\n", dropRate)
	}
	
	fmt.Printf("Queue Depths: %v\n", metrics.QueueDepths)
	fmt.Printf("CPU Core Mapping: %v\n", metrics.CoreMapping)
	fmt.Println("=====================================")
}