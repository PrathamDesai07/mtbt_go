package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/PrathamDesai07/mtbt_go/internal/core"
	"github.com/PrathamDesai07/mtbt_go/internal/network"
	"github.com/PrathamDesai07/mtbt_go/internal/orderbook"
)

// OrderbookSystem orchestrates the entire HFT orderbook system
type OrderbookSystem struct {
	receivers      [4]*network.ReceiverCore
	messageHandler *orderbook.MessageHandler
	config         *SystemConfig
	running        bool
}

// SystemConfig holds system configuration
type SystemConfig struct {
	MulticastIPs   [4]string // Primary multicast IPs for 4 receivers
	MulticastPorts [4]int    // Ports for each receiver
	CoreMapping    [5]int    // CPU cores for each component (0-3: receivers, 4: engine)
	LogLevel       string
}

// DefaultConfig returns a default system configuration
func DefaultConfig() *SystemConfig {
	return &SystemConfig{
		MulticastIPs: [4]string{
			"224.0.1.1", // Stream 1
			"224.0.1.2", // Stream 2  
			"224.0.1.3", // Stream 3
			"224.0.1.4", // Stream 4
		},
		MulticastPorts: [4]int{9001, 9002, 9003, 9004},
		CoreMapping:    [5]int{0, 1, 2, 3, 4}, // Cores 0-4
		LogLevel:       "INFO",
	}
}

// NewOrderbookSystem creates a new orderbook system
func NewOrderbookSystem(config *SystemConfig) (*OrderbookSystem, error) {
	// Set GOMAXPROCS to exactly 5 cores
	runtime.GOMAXPROCS(5)

	system := &OrderbookSystem{
		config: config,
	}

	// Create receiver cores
	for i := 0; i < 4; i++ {
		receiver, err := network.NewReceiverCore(
			config.CoreMapping[i],
			config.MulticastIPs[i],
			config.MulticastPorts[i],
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create receiver core %d: %w", i, err)
		}
		system.receivers[i] = receiver
	}

	// Create message handler
	system.messageHandler = orderbook.NewMessageHandler()

	return system, nil
}

// Start initializes and starts the orderbook system
func (sys *OrderbookSystem) Start() error {
	log.Println("Starting Ultra-Low-Latency Orderbook System...")

	// Start receiver cores first
	for i := 0; i < 4; i++ {
		log.Printf("Starting receiver core %d on CPU %d", i, sys.config.CoreMapping[i])
		err := sys.receivers[i].Start()
		if err != nil {
			return fmt.Errorf("failed to start receiver core %d: %w", i, err)
		}
	}

	// Allow receivers to initialize
	time.Sleep(100 * time.Millisecond)

	// Collect receiver queues
	var queues [4]*core.SPSCRingBuffer
	for i := 0; i < 4; i++ {
		queues[i] = sys.receivers[i].GetQueue()
	}

	// Start message handler on core 5
	log.Printf("Starting message handler on CPU %d", sys.config.CoreMapping[4])
	err := sys.messageHandler.Start(queues)
	if err != nil {
		return fmt.Errorf("failed to start message handler: %w", err)
	}

	sys.running = true
	log.Println("Orderbook system started successfully")

	return nil
}

// Stop gracefully shuts down the system
func (sys *OrderbookSystem) Stop() {
	log.Println("Stopping orderbook system...")
	
	sys.running = false

	// Stop message handler
	sys.messageHandler.Stop()

	// Stop receiver cores
	for i := 0; i < 4; i++ {
		sys.receivers[i].Stop()
	}

	log.Println("Orderbook system stopped")
}

// GetMetrics returns system performance metrics
func (sys *OrderbookSystem) GetMetrics() SystemMetrics {
	metrics := SystemMetrics{
		Timestamp: time.Now(),
	}

	// Collect receiver metrics
	for i := 0; i < 4; i++ {
		receiverMetrics := sys.receivers[i].GetMetrics()
		metrics.TotalPacketsReceived += receiverMetrics.PacketsReceived
		metrics.TotalPacketsProcessed += receiverMetrics.PacketsProcessed
		metrics.TotalPacketsDropped += receiverMetrics.PacketsDropped
		metrics.QueueDepths[i] = sys.receivers[i].GetQueueDepth()
	}

	// Collect handler metrics
	handlerMetrics := sys.messageHandler.GetMetrics()
	metrics.OrderbookLatencyNs = handlerMetrics.ProcessingLatencyNs
	metrics.GapsDetected = handlerMetrics.GapsDetected

	return metrics
}

// SystemMetrics holds comprehensive system metrics
type SystemMetrics struct {
	Timestamp              time.Time
	TotalPacketsReceived   uint64
	TotalPacketsProcessed  uint64
	TotalPacketsDropped    uint64
	QueueDepths            [4]uint64
	OrderbookLatencyNs     uint64
	GapsDetected           uint64
}

// PrintMetrics displays current system metrics
func (metrics SystemMetrics) Print() {
	fmt.Printf("\n=== Orderbook System Metrics ===\n")
	fmt.Printf("Timestamp: %s\n", metrics.Timestamp.Format("2006-01-02 15:04:05.000"))
	fmt.Printf("Packets Received: %d\n", metrics.TotalPacketsReceived)
	fmt.Printf("Packets Processed: %d\n", metrics.TotalPacketsProcessed)
	fmt.Printf("Packets Dropped: %d\n", metrics.TotalPacketsDropped)
	fmt.Printf("Queue Depths: %v\n", metrics.QueueDepths)
	fmt.Printf("Orderbook Latency: %d ns\n", metrics.OrderbookLatencyNs)
	fmt.Printf("Gaps Detected: %d\n", metrics.GapsDetected)
	
	if metrics.TotalPacketsReceived > 0 {
		dropRate := float64(metrics.TotalPacketsDropped) / float64(metrics.TotalPacketsReceived) * 100
		fmt.Printf("Drop Rate: %.4f%%\n", dropRate)
	}
	fmt.Println("==============================")
}

// RunMetricsMonitor starts a goroutine that periodically prints metrics
func (sys *OrderbookSystem) RunMetricsMonitor() {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for sys.running {
			select {
			case <-ticker.C:
				metrics := sys.GetMetrics()
				metrics.Print()
			}
		}
	}()
}

// GetOrderbook returns the orderbook for a specific token
func (sys *OrderbookSystem) GetOrderbook(token uint32) *core.Contract {
	return sys.messageHandler.GetContract(token)
}

func main() {
	log.Println("Ultra-Low-Latency HFT Orderbook System")
	log.Println("======================================")

	// Load configuration
	config := DefaultConfig()

	// Create system
	system, err := NewOrderbookSystem(config)
	if err != nil {
		log.Fatalf("Failed to create orderbook system: %v", err)
	}

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the system
	err = system.Start()
	if err != nil {
		log.Fatalf("Failed to start orderbook system: %v", err)
	}

	// Start metrics monitoring
	system.RunMetricsMonitor()

	// Initial metrics display
	time.Sleep(2 * time.Second)
	initialMetrics := system.GetMetrics()
	initialMetrics.Print()

	// Wait for shutdown signal
	log.Println("System running... Press Ctrl+C to shutdown")
	<-sigChan

	// Graceful shutdown
	system.Stop()
	log.Println("Shutdown complete")
}