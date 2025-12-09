# Ultra-Low-Latency HFT-Grade Orderbook Management System

A production-grade, ultra-low-latency orderbook management system designed for NSE's MTBT (Market Tick-by-Tick) FO (Futures & Options) feed processing.

## 🎯 Key Performance Metrics

- **Latency**: Sub-microsecond internal operations (target: <1μs P99)
- **Throughput**: 500k-1.5M packets/sec burst capacity
- **Instruments**: ~85,000 contracts support
- **Architecture**: 5-core pinned parallelism
- **Recovery**: <10ms gap recovery for <1000 messages
- **Memory**: <8GB for full instrument set

## 🏗 Architecture Overview

```
┌─────────────┬─────────────┬─────────────┬─────────────┐
│   Core 1    │   Core 2    │   Core 3    │   Core 4    │
│UDP Receiver │UDP Receiver │UDP Receiver │UDP Receiver │
│ RSS Queue   │ RSS Queue   │ RSS Queue   │ RSS Queue   │
└─────────────┴─────────────┴─────────────┴─────────────┘
                              │
                              ▼
                    ┌─────────────────┐
                    │     Core 5      │
                    │ Orderbook Engine│
                    │   & Sequencer   │
                    └─────────────────┘
```

### Data Flow Pipeline
```
Multicast   →   RSS       →   SPSC Ring   →   Sequence   →   Orderbook
Packets         Queues        Buffers         Merger         Engine
(4 feeds)       (4 cores)     (Lock-free)     (Core 5)       (Core 5)
```

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- Linux system with 5+ CPU cores
- Mellanox NIC with RSS support (recommended)
- Root access for system tuning

### System Tuning (Required)
```bash
# Increase kernel receiver backlog
sudo sysctl -w net.core.netdev_max_backlog=50000

# Increase OS buffers
sudo sysctl -w net.core.rmem_max=134217728
sudo sysctl -w net.ipv4.tcp_mem="10240 87380 134217728"

# CPU isolation (cores 0-4)
echo "0-4" | sudo tee /sys/devices/system/cpu/isolated

# Performance governor
echo "performance" | sudo tee /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor

# NIC tuning (replace ethX with your interface)
sudo ethtool -G ethX rx 4096
```

### Build & Run
```bash
# Build with optimizations
go build -ldflags="-s -w" -o orderbook main.go

# Run with CPU pinning
sudo taskset -c 0-4 ./orderbook
```

## 📁 Project Structure

```
mtbt_go/
├── main.go                     # System orchestrator
├── go.mod                      # Go module definition  
├── README.md                   # This file
├── Plans/
│   └── 01_Orderbook_System_Design.md  # Detailed design document
├── Docs/                       # NSE MTBT protocol documentation
├── internal/
│   ├── core/                   # Core data structures and types
│   │   ├── types.go           # MTBT protocol types and constants
│   │   └── spsc_queue.go      # Lock-free SPSC ring buffer
│   ├── network/                # Network layer
│   │   └── receiver.go        # UDP multicast receivers
│   └── orderbook/             # Orderbook engine
│       ├── orderbook.go       # Price trees and order maps
│       ├── message_handler.go # MTBT message processing
│       ├── sequencer.go       # Multi-stream sequence merger
│       └── allocator.go       # Memory pool management
```

For complete implementation details, see `Plans/01_Orderbook_System_Design.md`