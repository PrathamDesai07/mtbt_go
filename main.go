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
	streamManager  *network.StreamManager
	messageHandler *orderbook.MessageHandler
	config         *SystemConfig
	running        bool
}

// SystemConfig holds system configuration
type SystemConfig struct {
	NumCPUCores     int      // Total CPU cores available
	ReceiverCores   int      // Cores dedicated to receivers  
	OrderbookCore   int      // Core for orderbook processing
	EnableFOStreams bool     // Enable FO MTBT streams
	LogLevel        string
	DebugMode       bool     // Enable debug logging
}

// DefaultConfig returns a default system configuration
func DefaultConfig() *SystemConfig {
	numCPU := runtime.NumCPU()
	if numCPU < 2 {
		numCPU = 2 // Minimum required
	}
	
	return &SystemConfig{
		NumCPUCores:     numCPU,
		ReceiverCores:   numCPU - 1, // Reserve 1 core for orderbook
		OrderbookCore:   numCPU - 1, // Last core for orderbook
		EnableFOStreams: true,
		LogLevel:        "INFO",
		DebugMode:       false,
	}
}

// NewOrderbookSystem creates a new orderbook system
func NewOrderbookSystem(config *SystemConfig) (*OrderbookSystem, error) {
	// Set GOMAXPROCS to use all available cores
	runtime.GOMAXPROCS(config.NumCPUCores)

	log.Printf("Initializing HFT Orderbook System with %d CPU cores", config.NumCPUCores)
	log.Printf("Receiver cores: %d, Orderbook core: %d", config.ReceiverCores, config.OrderbookCore)

	system := &OrderbookSystem{
		config: config,
	}

	// Create stream manager for FO streams
	if config.EnableFOStreams {
		streamManager, err := network.NewStreamManager(
			network.FOStreamConfig, 
			config.ReceiverCores,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create stream manager: %w", err)
		}
		system.streamManager = streamManager
		log.Printf("Created stream manager for %d FO streams", len(network.FOStreamConfig))
	} else {
		return nil, fmt.Errorf("FO streams must be enabled for this system")
	}

	// Create message handler
	system.messageHandler = orderbook.NewMessageHandler()
	log.Println("Message handler created")

	return system, nil
}

// Start initializes and starts the orderbook system
func (sys *OrderbookSystem) Start() error {
	log.Println("Starting Ultra-Low-Latency Orderbook System...")
	log.Printf("Target: 18 FO MTBT streams, %d CPU cores", sys.config.NumCPUCores)

	// Start stream manager
	err := sys.streamManager.Start()
	if err != nil {
		return fmt.Errorf("failed to start stream manager: %w", err)
	}

	// Allow receivers to initialize
	time.Sleep(100 * time.Millisecond)

	// Get all receiver queues
	queues := sys.streamManager.GetQueues()
	log.Printf("Collected %d receiver queues", len(queues))

	// Start message handler on dedicated core
	log.Printf("Starting message handler on CPU %d", sys.config.OrderbookCore)
	err = sys.messageHandler.Start(queues)
	if err != nil {
		return fmt.Errorf("failed to start message handler: %w", err)
	}

	sys.running = true
	log.Printf("Orderbook system started successfully - monitoring %d FO streams", len(network.FOStreamConfig))

	return nil
}

// Stop gracefully shuts down the system
func (sys *OrderbookSystem) Stop() {
	log.Println("Stopping orderbook system...")
	
	sys.running = false

	// Stop message handler
	if sys.messageHandler != nil {
		sys.messageHandler.Stop()
	}

	// Stop stream manager
	if sys.streamManager != nil {
		sys.streamManager.Stop()
	}

	log.Println("Orderbook system stopped")
}

// GetMetrics returns system performance metrics
func (sys *OrderbookSystem) GetMetrics() SystemMetrics {
	metrics := SystemMetrics{
		Timestamp:    time.Now(),
		StreamCount:  len(network.FOStreamConfig),
		ActiveCores:  sys.config.ReceiverCores + 1, // +1 for orderbook
	}

	// Collect stream manager metrics
	streamMetrics := sys.streamManager.GetMetrics()
	metrics.TotalPacketsReceived = streamMetrics.TotalPacketsReceived
	metrics.TotalPacketsProcessed = streamMetrics.TotalPacketsProcessed
	metrics.TotalPacketsDropped = streamMetrics.TotalPacketsDropped
	metrics.TotalBandwidth = streamMetrics.TotalBandwidth
	
	// Resize QueueDepths to match actual streams
	metrics.QueueDepths = make([]uint64, len(streamMetrics.QueueDepths))
	copy(metrics.QueueDepths, streamMetrics.QueueDepths)

	// Collect handler metrics
	handlerMetrics := sys.messageHandler.GetMetrics()
	metrics.OrderbookLatencyNs = handlerMetrics.ProcessingLatencyNs
	metrics.GapsDetected = handlerMetrics.GapsDetected

	return metrics
}

// SystemMetrics holds comprehensive system metrics
type SystemMetrics struct {
	Timestamp              time.Time
	StreamCount            int
	ActiveCores            int
	TotalPacketsReceived   uint64
	TotalPacketsProcessed  uint64
	TotalPacketsDropped    uint64
	QueueDepths            []uint64 // Dynamic size based on streams
	OrderbookLatencyNs     uint64
	GapsDetected           uint64
	TotalBandwidth         string
}

// PrintMetrics displays current system metrics
func (metrics SystemMetrics) Print() {
	fmt.Printf("\n=== HFT Orderbook System Metrics ===\n")
	fmt.Printf("Timestamp: %s\n", metrics.Timestamp.Format("2006-01-02 15:04:05.000"))
	fmt.Printf("FO Streams Active: %d\n", metrics.StreamCount)
	fmt.Printf("CPU Cores Used: %d\n", metrics.ActiveCores)
	fmt.Printf("Total Bandwidth: %s\n", metrics.TotalBandwidth)
	fmt.Printf("Packets Received: %d\n", metrics.TotalPacketsReceived)
	fmt.Printf("Packets Processed: %d\n", metrics.TotalPacketsProcessed)
	fmt.Printf("Packets Dropped: %d\n", metrics.TotalPacketsDropped)
	fmt.Printf("Orderbook Latency: %d ns\n", metrics.OrderbookLatencyNs)
	fmt.Printf("Gaps Detected: %d\n", metrics.GapsDetected)
	
	if metrics.TotalPacketsReceived > 0 {
		dropRate := float64(metrics.TotalPacketsDropped) / float64(metrics.TotalPacketsReceived) * 100
		fmt.Printf("Drop Rate: %.4f%%\n", dropRate)
	}
	
	// Display queue depths (truncated if too many)
	if len(metrics.QueueDepths) <= 8 {
		fmt.Printf("Queue Depths: %v\n", metrics.QueueDepths)
	} else {
		fmt.Printf("Queue Depths [1-8]: %v\n", metrics.QueueDepths[:8])
		fmt.Printf("Queue Depths [9-%d]: %v\n", len(metrics.QueueDepths), metrics.QueueDepths[8:])
	}
	fmt.Println("====================================")
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