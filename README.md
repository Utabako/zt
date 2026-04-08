# zt

Go bindings for the [ZeroTier One](https://github.com/zerotier/ZeroTierOne) core C API via CGo.

This library provides a complete, general-purpose Go wrapper around the ZeroTier node engine, suitable for embedding ZeroTier networking into any Go application.

## Features

- Complete binding of `ZeroTierOne.h` C API
- Go-idiomatic types with `net/netip` for addresses
- Safe callback dispatch via `runtime/cgo.Handle`
- Thread-safe Node operations
- Cross-compilation support (Linux, Android)

## Prerequisites

- Go 1.21+
- C++ compiler (g++ or clang++)
- ZeroTier One source code (included as a git submodule)

## Quick Start

### 1. Initialize the ZeroTierOne submodule

```bash
git submodule update --init --recursive
```

### 2. Build the static library

```bash
make lib
# or specify a different ZeroTierOne source tree:
make lib ZEROTIER_SRC=/path/to/ZeroTierOne
```

### 3. Use in your Go code

```go
package main

import (
    "fmt"
    "log"
    "net/netip"
    "os"
    "os/signal"
    "time"

    "github.com/Utabako/zt"
)

func main() {
    major, minor, rev := zt.Version()
    fmt.Printf("ZeroTier Core %d.%d.%d\n", major, minor, rev)

    node, err := zt.NewNode(zt.NodeCallbacks{
        StatePut: func(objType zt.StateObjectType, id [2]uint64, data []byte) {
            // Persist state objects to disk
        },
        StateGet: func(objType zt.StateObjectType, id [2]uint64) ([]byte, bool) {
            // Load state objects from disk
            return nil, false
        },
        WirePacketSend: func(localSocket int64, addr netip.AddrPort, data []byte, ttl uint) error {
            // Send UDP packet to physical network
            return nil
        },
        VirtualNetworkFrame: func(nwid uint64, srcMAC, dstMAC uint64, etherType, vlanID uint, data []byte) {
            // Process incoming virtual network Ethernet frame
        },
        VirtualNetworkConfig: func(nwid uint64, op zt.VirtualNetworkConfigOperation, config *zt.VirtualNetworkConfig) int {
            fmt.Printf("Network %016x: %s\n", nwid, op)
            return 0
        },
        Event: func(event zt.Event, data any) {
            fmt.Printf("Event: %s\n", event)
        },
    })
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()

    fmt.Printf("Node address: %010x\n", node.Address())

    // Join a network
    if err := node.Join(0x8056c2e21c000001); err != nil {
        log.Fatal(err)
    }

    // Run background tasks
    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt)

    ticker := time.NewTicker(250 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-sig:
            return
        case <-ticker.C:
            node.ProcessBackgroundTasks(nil)
        }
    }
}
```

## Cross-compilation

```bash
# Linux ARM64
make linux-arm64

# Android ARM64 (requires NDK)
NDK=/path/to/ndk make android-arm64
```

## API Overview

### Node Lifecycle

| Method | Description |
|--------|-------------|
| `NewNode(callbacks)` | Create a new ZeroTier node |
| `node.Close()` | Shut down and free the node |
| `Version()` | Get ZeroTier core version |

### Network Operations

| Method | Description |
|--------|-------------|
| `node.Join(nwid)` | Join a virtual network |
| `node.Leave(nwid)` | Leave a virtual network |
| `node.Networks()` | List all joined networks |
| `node.NetworkConfig(nwid)` | Get network configuration |

### Packet Processing

| Method | Description |
|--------|-------------|
| `node.ProcessWirePacket(...)` | Process incoming physical packet |
| `node.ProcessVirtualNetworkFrame(...)` | Process outgoing virtual frame |
| `node.ProcessBackgroundTasks(...)` | Run periodic maintenance |

### Peer Management

| Method | Description |
|--------|-------------|
| `node.Peers()` | List all known peers |
| `node.Orbit(moonID, seed)` | Add a moon |
| `node.Deorbit(moonID)` | Remove a moon |
| `node.SendUserMessage(...)` | Send P2P user message |

### Path Management

| Method | Description |
|--------|-------------|
| `node.AddLocalInterfaceAddress(addr)` | Register local address |
| `node.ClearLocalInterfaceAddresses()` | Clear local addresses |
| `node.SetPhysicalPathConfiguration(...)` | Configure physical path |

### Multicast

| Method | Description |
|--------|-------------|
| `node.MulticastSubscribe(...)` | Subscribe to multicast group |
| `node.MulticastUnsubscribe(...)` | Unsubscribe from multicast |

## License

This repository's Go binding code is licensed under the **MIT** license.

The bundled `ZeroTierOne/` git submodule is distributed as-is under its original upstream licenses and is **not** relicensed under MIT.

See the repository `LICENSE` file for this project's license, and see `ZeroTierOne/LICENSE.txt` (plus licenses under `ZeroTierOne/ext/` and `ZeroTierOne/nonfree/` where applicable) for the submodule's original licensing terms.
