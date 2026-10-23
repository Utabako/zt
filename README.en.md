# zt

Go bindings for the [ZeroTier One](https://github.com/zerotier/ZeroTierOne) C core via CGo. Embed a ZeroTier node in any Go application.

> 中文文档：[README.md](README.md)

## How it works

This repository vendors the required subset of upstream ZeroTier One sources under `zerotier/` and lets CGo compile them directly, exposing a Go-idiomatic API. For normal use you do not need to initialize a submodule or build a separate static library first.

```
Your Go code
    ↓
zt (this library)
    ↓  CGo
zerotier/ (vendored subset of ZeroTier C++ sources)
    ↓
Physical network / virtual network
```

## Requirements

- Go 1.21+
- g++ or clang++ (to compile the vendored ZeroTier C++ sources)

## Quick Start

```bash
go build ./...
go test ./...
```

Import `github.com/Utabako/zt` and start coding.

### Write your code

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


    node, err := zt.NewNode(zt.NodeCallbacks{
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

    // Background goroutine: feed incoming UDP packets to the initialized node
    go func() {
        buf := make([]byte, 16384)
        for {
            n, addr, err := conn.ReadFrom(buf)
            if err != nil {
                return
            }
            ap := addr.(*net.UDPAddr).AddrPort()
            _ = node.ProcessWirePacket(0, ap, buf[:n], nil)
        }
    }()

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

If any required callback is missing, `NewNode` returns an error listing every missing field and does not create a node. Only `nil` passed to `StatePut` means deletion; a non-nil empty slice is a present zero-length object and should be returned by `StateGet` as `[]byte{}, true`.

The ZeroTier C++ core invokes callbacks, and some frame callbacks run on background threads. A callback panic cannot cross the CGo boundary; when `Logger` is configured it receives a diagnostic with the stack, and callbacks with return values fail closed. Do not call Node methods from callbacks except `Address()` because the upstream core may hold internal locks while invoking them.

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

IPv6 link-local addresses retain their scope. Input zones may be interface names or decimal interface indexes; callbacks expose zones as decimal interface indexes.

## API Reference

### Node lifecycle

```go
node, err := zt.NewNode(callbacks, config) // pass nil config for defaults
node.Close()                                // shut down; safe to call multiple times
major, minor, rev := zt.Version()          // ZeroTier core version
```

> `Close()` waits for in-flight calls and emits exactly one `EventDown` before destruction. Do not call Node methods while handling `EventDown`; calling any Node method after `Close()` returns will panic.

> Known upstream issue: ZeroTier 1.16.2 runs some frame callbacks on detached threads. `Close()` does not wait for those threads, so node destruction can race with callbacks. This lifecycle defect remains unresolved.

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

This repository no longer ships `make` shortcuts for cross-compilation. Because the build now involves both cgo and vendored C++ sources, cross-compiling depends on having the target platform's C/C++ toolchain and CGO configuration available.

If you need cross-compilation, set the target `CC`, `CXX`, `CGO_ENABLED`, and related environment variables explicitly, or build natively on the target platform.

## License

The Go binding code in this repository remains licensed under the **MIT** license.

The upstream ZeroTier One sources vendored under `zerotier/`, together with any third-party code they contain, keep their original licenses and are not relicensed under MIT. See `zerotier/LICENSE.txt` and the license files in its subdirectories for details.

`nonfree/` is not included in this repository.
