# zt

用 Go 写 ZeroTier 应用的 CGo 绑定库。把 [ZeroTier One](https://github.com/zerotier/ZeroTierOne) 的 C 核心包成 Go 接口，让你可以在任意 Go 程序里嵌入 ZeroTier 节点。

> 英文版文档：[README.en.md](README.en.md)

## 工作原理

ZeroTier 的核心逻辑是 C++ 实现的（`libzerotiercore.a`）。本库通过 CGo 调用这个静态库，对外暴露 Go 风格的 API。你需要先编译这个静态库，然后才能 `go build`。

```
你的 Go 代码
    ↓
zt（本库）
    ↓  CGo
libzerotiercore.a（ZeroTier C++ 核心）
    ↓
物理网络 / 虚拟网络
```

## 环境要求

- Go 1.21+
- g++ 或 clang++（编译 ZeroTier C++ 核心用）
- git submodule（ZeroTier One 源码）

## 快速开始

### 第一步：拉取 ZeroTier 源码

```bash
git submodule update --init --recursive
```

### 第二步：编译静态库

```bash
make lib
```

编译产物在 `lib/libzerotiercore.a`，之后 `go build` 会自动链接它。

### 第三步：写代码

一个最小可运行的例子——启动节点、加入网络、保持运行：

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

    // 用 UDP 套接字收发物理网络上的 ZeroTier 数据包
    conn, err := net.ListenPacket("udp", ":0")
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    var node *zt.Node

    // 后台 goroutine：把收到的 UDP 包喂给节点
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
        // StatePut / StateGet：持久化节点身份和网络配置
        // 这里简单丢弃，生产代码里应该写到磁盘
        StatePut: func(t zt.StateObjectType, id [2]uint64, data []byte) {},
        StateGet: func(t zt.StateObjectType, id [2]uint64) ([]byte, bool) {
            return nil, false
        },

        // WirePacketSend：把数据包发到物理网络
        WirePacketSend: func(_ int64, addr netip.AddrPort, data []byte, _ uint) error {
            _, err := conn.WriteTo(data, net.UDPAddrFromAddrPort(addr))
            return err
        },

        // VirtualNetworkFrame：收到虚拟网络上的以太帧
        VirtualNetworkFrame: func(nwid uint64, srcMAC, dstMAC uint64, etherType, vlanID uint, data []byte) {
            fmt.Printf("帧 nwid=%016x src=%012x dst=%012x type=0x%04x len=%d\n",
                nwid, srcMAC, dstMAC, etherType, len(data))
        },

        // VirtualNetworkConfig：网络配置变更（加入、更新、离开）
        VirtualNetworkConfig: func(nwid uint64, op zt.VirtualNetworkConfigOperation, cfg *zt.VirtualNetworkConfig) int {
            fmt.Printf("网络 %016x %s，状态=%s\n", nwid, op, cfg.Status)
            return 0
        },

        // Event：节点状态变化（上线、下线等）
        Event: func(ev zt.Event, data any) {
            fmt.Printf("事件: %s\n", ev)
        },
    }, nil)
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()

    fmt.Printf("节点地址: %010x\n", node.Address())

    // 加入一个网络（替换成你自己的网络 ID）
    if err := node.Join(0x8056c2e21c000001); err != nil {
        log.Fatal(err)
    }

    // 周期性执行后台任务（心跳、路径探测等）
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

完整示例见 [`example/main.go`](example/main.go)。

## 回调说明

`NewNode` 需要传入一个 `NodeCallbacks` 结构体，ZeroTier 核心通过这些回调和你的代码交互。

### 必填回调

| 回调 | 何时调用 | 你需要做什么 |
|------|---------|-------------|
| `StatePut(type, id, data)` | 需要保存或删除状态（节点身份、网络配置等）| `data == nil` 时删除，否则写入持久化存储 |
| `StateGet(type, id)` | 启动时加载已保存的状态 | 返回数据和 `true`；找不到时返回 `nil, false` |
| `WirePacketSend(socket, addr, data, ttl)` | 需要往物理网络发包 | 用 UDP 把 `data` 发到 `addr` |
| `VirtualNetworkFrame(nwid, srcMAC, dstMAC, etherType, vlanID, data)` | 收到虚拟网络以太帧 | 把帧注入你的 TAP/TUN 设备，或自行处理 |
| `VirtualNetworkConfig(nwid, op, config)` | 网络配置变更 | 根据 `op`（UP/CONFIG_UPDATE/DOWN/DESTROY）调整你这端的网络接口；成功返回 `0` |
| `Event(event, data)` | 节点状态事件 | 处理上线/下线/错误等事件；`data` 的类型取决于 `event`（见下表） |

**Event 的 data 类型**：

| Event | data 类型 |
|-------|-----------|
| `EventUserMessage` | `*zt.UserMessage` |
| `EventRemoteTrace` | `*zt.RemoteTrace` |
| `EventTrace` | `string` |
| 其他 | `nil` |

### 可选回调

| 回调 | 用途 |
|------|------|
| `PathCheck(ztAddr, socket, addr)` | 过滤物理路径，返回 `false` 拒绝该路径 |
| `PathLookup(ztAddr, family)` | 提供对端的地址提示（类似地址簿） |
| `Logger(msg)` | 接收库内部的诊断信息（如缓冲区溢出警告） |

## API 速查

### 节点生命周期

```go
node, err := zt.NewNode(callbacks, config) // config 传 nil 使用默认配置
node.Close()                                // 关闭节点，可安全调用多次
major, minor, rev := zt.Version()          // 查询 ZeroTier 核心版本
```

> 注意：`Close()` 之后调用任何节点方法会 panic。

### 网络

```go
node.Join(nwid)              // 加入网络
node.Leave(nwid)             // 离开网络
node.Networks()              // 列出已加入的所有网络
node.NetworkConfig(nwid)     // 获取某个网络的配置（不在则返回 nil）
```

### 数据包处理

```go
// 从物理网络收到数据包时调用（通常在 UDP read goroutine 里）
node.ProcessWirePacket(localSocket, remoteAddr, data, &nextDeadline)

// 从虚拟网络（TAP）收到以太帧时调用
node.ProcessVirtualNetworkFrame(nwid, srcMAC, dstMAC, etherType, vlanID, data, &nextDeadline)

// 周期性调用，处理心跳、路径探测等后台任务
node.ProcessBackgroundTasks(&nextDeadline)
```

`nextDeadline` 是下次必须调用 `ProcessBackgroundTasks` 的截止时间（Unix 毫秒），传 `nil` 表示不关心。

### 对端与路径

```go
node.Peers()                                        // 列出已知对端
node.Orbit(moonWorldID, moonSeed)                   // 加入 Moon（自建根服务器）
node.Deorbit(moonWorldID)                           // 离开 Moon
node.SendUserMessage(dest, typeID, data)            // 发送 P2P 消息，失败返回 ErrUserMessageRejected
node.AddLocalInterfaceAddress(addr)                 // 注册本地地址，失败返回 ErrLocalInterfaceAddressRejected
node.ClearLocalInterfaceAddresses()                 // 清除所有已注册的本地地址
node.SetPhysicalPathConfiguration(addr, id, mtu)   // 配置物理路径（可信路径 ID、MTU）
```

### 组播

```go
node.MulticastSubscribe(nwid, group, adi)    // 订阅以太网组播组
node.MulticastUnsubscribe(nwid, group, adi)  // 取消订阅
```

## 交叉编译

```bash
# Linux ARM64
make linux-arm64

# Android ARM64（需要 NDK）
NDK=/path/to/ndk make android-arm64

# 指定其他 ZeroTier 源码路径
make lib ZEROTIER_SRC=/path/to/ZeroTierOne
```

## 许可证

本仓库的 Go 绑定代码使用 **MIT** 协议。

`ZeroTierOne/` 子模块保留其原有上游协议，不受 MIT 协议约束。具体见 `ZeroTierOne/LICENSE.txt` 及 `ZeroTierOne/ext/` 下各自的许可文件。
