# zt

Go bindings for the [ZeroTier One](https://github.com/zerotier/ZeroTierOne) C core via CGo. Embed a ZeroTier node in any Go application.

> 中文文档：[README.md](README.md)

## How it works

ZeroTier's core logic is implemented in C++ (`libzerotiercore.a`). This library wraps that static library via CGo and exposes a Go-idiomatic API. You need to build the static library once before running `go build`.

```
Your Go code
    ↓
zt (this library)
    ↓  CGo
libzerotiercore.a (ZeroTier C++ core)
    ↓
Physical network / virtual network
```

## Requirements

- Go 1.21+
- g++ or clang++
- git submodule (ZeroTier One source)

## Quick Start

### 1. Initialize the submodule

```bash
git submodule update --init --recursive
```

### 2. Build the static library

```bash
make lib
```

The output is `lib/libzerotiercore.a`. `go build` links it automatically.

### 3. Write your code

A minimal example — start a node, join a network, keep it running:

```go
package main

import (
    "fmt"
    "log"
    "net"
    "net/netip"
    "os"
    "os/signal"
    "time"

    "github.com/Utabako/zt"
)

func main() {
    major, minor, rev := zt.Version()
    fmt.Printf("ZeroTier Core %d.%d.%d\n", major, minor, rev)

    // UDP socket for sending/receiving ZeroTier wire packets
    conn, err := net.ListenPacket("udp", ":0")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    var node *zt.Node

    // Background goroutine: feed incoming UDP packets to the node
    go func() {
        buf := make([]byte, 16384)
        for {
            n, addr, err := conn.ReadFrom(buf)
            if err != nil {
                return
            }
            if node != nil {
                ap := addr.(*net.UDPAddr).AddrPort()
                node.ProcessWirePacket(0, ap, buf[:n], nil)
            }
        }
    }()

    node, err = zt.NewNode(zt.NodeCallbacks{
        // StatePut / StateGet: persist node identity and network configs
        // (discard here; production code should write to disk)
        StatePut: func(t zt.StateObjectType, id [2]uint64, data []byte) {},
        StateGet: func(t zt.StateObjectType, id [2]uint64) ([]byte, bool) {
            return nil, false
        },

        // WirePacketSend: send a packet out onto the physical network
        WirePacketSend: func(_ int64, addr netip.AddrPort, data []byte, _ uint) error {
            _, err := conn.WriteTo(data, net.UDPAddrFromAddrPort(addr))
            return err
        },

        // VirtualNetworkFrame: an Ethernet frame arrived on the virtual network
        VirtualNetworkFrame: func(nwid uint64, srcMAC, dstMAC uint64, etherType, vlanID uint, data []byte) {
            fmt.Printf("frame nwid=%016x src=%012x dst=%012x type=0x%04x len=%d\n",
                nwid, srcMAC, dstMAC, etherType, len(data))
        },

        // VirtualNetworkConfig: network configuration changed (join, update, leave)
        VirtualNetworkConfig: func(nwid uint64, op zt.VirtualNetworkConfigOperation, cfg *zt.VirtualNetworkConfig) int {
            fmt.Printf("network %016x %s, status=%s\n", nwid, op, cfg.Status)
            return 0
        },

        // Event: node state changes (online, offline, errors, etc.)
        Event: func(ev zt.Event, data any) {
            fmt.Printf("event: %s\n", ev)
        },
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()

    fmt.Printf("node address: %010x\n", node.Address())

    // Join a network (replace with your own network ID)
    if err := node.Join(0x8056c2e21c000001); err != nil {
        log.Fatal(err)
    }

    // Periodically run background tasks (keepalives, path probing, etc.)
    ticker := time.NewTicker(200 * time.Millisecond)
    defer ticker.Stop()

    sig := make(chan os.Signal, 1)
    signal.Notify(sig, os.Interrupt)

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

See [`example/main.go`](example/main.go) for a complete example.

## Callbacks

`NewNode` takes a `NodeCallbacks` struct. The ZeroTier core calls back into your code through these functions.

### Required callbacks

| Callback | When called | What to do |
|----------|-------------|------------|
| `StatePut(type, id, data)` | ZeroTier wants to save or delete state (identity, network config, etc.) | Write to persistent storage; delete when `data == nil` |
| `StateGet(type, id)` | ZeroTier wants to load previously saved state | Return the data and `true`; return `nil, false` if not found |
| `WirePacketSend(socket, addr, data, ttl)` | ZeroTier wants to send a packet onto the physical network | Send `data` as a UDP packet to `addr` |
| `VirtualNetworkFrame(nwid, srcMAC, dstMAC, etherType, vlanID, data)` | An Ethernet frame arrived on a virtual network | Inject into your TAP/TUN device or handle directly |
| `VirtualNetworkConfig(nwid, op, config)` | Network configuration changed | React to `op` (UP / CONFIG_UPDATE / DOWN / DESTROY); return `0` on success |
| `Event(event, data)` | Node state event | Handle online/offline/error events; `data` type depends on `event` (see below) |

**Event `data` types:**

| Event | `data` type |
|-------|-------------|
| `EventUserMessage` | `*zt.UserMessage` |
| `EventRemoteTrace` | `*zt.RemoteTrace` |
| `EventTrace` | `string` |
| others | `nil` |

### Optional callbacks

| Callback | Purpose |
|----------|---------|
| `PathCheck(ztAddr, socket, addr)` | Filter physical paths; return `false` to reject a path |
| `PathLookup(ztAddr, family)` | Provide address hints for reaching a peer (like an address book) |
| `Logger(msg)` | Receive internal diagnostic messages (e.g. buffer overflow warnings) |

## API Reference

### Node lifecycle

```go
node, err := zt.NewNode(callbacks, config) // pass nil config for defaults
node.Close()                                // shut down; safe to call multiple times
major, minor, rev := zt.Version()          // ZeroTier core version
```

> Calling any method after `Close()` will panic.

### Networks

```go
node.Join(nwid)          // join a virtual network
node.Leave(nwid)         // leave a virtual network
node.Networks()          // list all joined networks
node.NetworkConfig(nwid) // get config for one network (nil if not joined)
```

### Packet processing

```go
// Call when a UDP packet arrives from the physical network
node.ProcessWirePacket(localSocket, remoteAddr, data, &nextDeadline)

// Call when an Ethernet frame arrives from your TAP/TUN device
node.ProcessVirtualNetworkFrame(nwid, srcMAC, dstMAC, etherType, vlanID, data, &nextDeadline)

// Call periodically to handle keepalives, path probing, etc.
node.ProcessBackgroundTasks(&nextDeadline)
```

`nextDeadline` is the Unix millisecond timestamp by which you must call `ProcessBackgroundTasks` again. Pass `nil` to ignore it.

### Peers and paths

```go
node.Peers()                                        // list known peers
node.Orbit(moonWorldID, moonSeed)                   // add a moon (self-hosted root)
node.Deorbit(moonWorldID)                           // remove a moon
node.SendUserMessage(dest, typeID, data)            // send a P2P message; returns ErrUserMessageRejected on failure
node.AddLocalInterfaceAddress(addr)                 // register a local address; returns ErrLocalInterfaceAddressRejected on failure
node.ClearLocalInterfaceAddresses()                 // remove all registered local addresses
node.SetPhysicalPathConfiguration(addr, id, mtu)   // configure a physical path (trusted path ID, MTU)
```

### Multicast

```go
node.MulticastSubscribe(nwid, group, adi)    // subscribe to an Ethernet multicast group
node.MulticastUnsubscribe(nwid, group, adi)  // unsubscribe
```

## Cross-compilation

```bash
# Linux ARM64
make linux-arm64

# Android ARM64 (requires NDK)
NDK=/path/to/ndk make android-arm64

# Custom ZeroTier source path
make lib ZEROTIER_SRC=/path/to/ZeroTierOne
```

## License

The Go binding code in this repository is licensed under the **MIT** license.

The `ZeroTierOne/` git submodule retains its original upstream licenses and is not relicensed under MIT. See `ZeroTierOne/LICENSE.txt` and the license files under `ZeroTierOne/ext/` for details.
