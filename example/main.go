package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/Utabako/zt"
)

type stateStore struct {
	dir string
}

func (s *stateStore) path(objType zt.StateObjectType, id [2]uint64) string {
	name := fmt.Sprintf("%s_%016x_%016x", objType, id[0], id[1])
	return filepath.Join(s.dir, name)
}

func (s *stateStore) Put(objType zt.StateObjectType, id [2]uint64, data []byte) {
	if data == nil {
		os.Remove(s.path(objType, id))
		return
	}
	os.MkdirAll(s.dir, 0700)
	os.WriteFile(s.path(objType, id), data, 0600)
}

func (s *stateStore) Get(objType zt.StateObjectType, id [2]uint64) ([]byte, bool) {
	data, err := os.ReadFile(s.path(objType, id))
	if err != nil {
		return nil, false
	}
	return data, true
}

func main() {
	major, minor, rev := zt.Version()
	fmt.Printf("ZeroTier Core v%d.%d.%d\n", major, minor, rev)

	homeDir, _ := os.UserHomeDir()
	store := &stateStore{dir: filepath.Join(homeDir, ".go-zt-example")}

	udpConn, err := net.ListenPacket("udp", ":0")
	if err != nil {
		log.Fatalf("Failed to listen UDP: %v", err)
	}
	defer udpConn.Close()
	fmt.Printf("Listening on %s\n", udpConn.LocalAddr())

	node, err := zt.NewNode(zt.NodeCallbacks{
		StatePut: store.Put,
		StateGet: store.Get,
		WirePacketSend: func(localSocket int64, addr netip.AddrPort, data []byte, ttl uint) error {
			target := net.UDPAddrFromAddrPort(addr)
			_, writeErr := udpConn.WriteTo(data, target)
			return writeErr
		},
		VirtualNetworkFrame: func(nwid uint64, srcMAC, dstMAC uint64, etherType, vlanID uint, data []byte) {
			fmt.Printf("  Frame on %016x: %012x -> %012x type=0x%04x len=%d\n",
				nwid, srcMAC, dstMAC, etherType, len(data))
			if len(data) > 64 {
				fmt.Printf("    %s\n", hex.Dump(data[:64]))
			}
		},
		VirtualNetworkConfig: func(nwid uint64, op zt.VirtualNetworkConfigOperation, config *zt.VirtualNetworkConfig) int {
			fmt.Printf("  Network %016x %s: status=%s mtu=%d\n",
				nwid, op, config.Status, config.MTU)
			for _, addr := range config.AssignedAddresses {
				fmt.Printf("    Address: %s\n", addr)
			}
			for _, route := range config.Routes {
				if route.Via.IsValid() {
					fmt.Printf("    Route: %s via %s\n", route.Target, route.Via)
				} else {
					fmt.Printf("    Route: %s (direct)\n", route.Target)
				}
			}
			return 0
		},
		Event: func(event zt.Event, data any) {
			fmt.Printf("  Event: %s\n", event)
		},
		PathCheck: func(ztAddress uint64, localSocket int64, addr netip.AddrPort) bool {
			return true
		},
	}, nil)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}
	defer node.Close()

	go func() {
		buf := make([]byte, 16384)
		for {
			n, addr, err := udpConn.ReadFrom(buf)
			if err != nil {
				return
			}
			udpAddr, ok := addr.(*net.UDPAddr)
			if !ok {
				continue
			}
			ap := udpAddr.AddrPort()
			_ = node.ProcessWirePacket(0, ap, buf[:n], nil)
		}
	}()

	fmt.Printf("Node address: %010x\n", node.Address())

	networkID := uint64(0x8056c2e21c000001)
	if len(os.Args) > 1 {
		fmt.Sscanf(os.Args[1], "%x", &networkID)
	}

	fmt.Printf("Joining network %016x...\n", networkID)
	if err := node.Join(networkID); err != nil {
		log.Fatalf("Failed to join: %v", err)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	statusTicker := time.NewTicker(10 * time.Second)
	defer statusTicker.Stop()

	fmt.Println("Running... Press Ctrl+C to exit")

	for {
		select {
		case <-sig:
			fmt.Println("\nShutting down...")
			node.Leave(networkID)
			return
		case <-ticker.C:
			node.ProcessBackgroundTasks(nil)
		case <-statusTicker.C:
			status := node.Status()
			fmt.Printf("  Status: online=%v address=%010x\n", status.Online, status.Address)
			peers := node.Peers()
			fmt.Printf("  Peers: %d\n", len(peers))
			for _, p := range peers {
				fmt.Printf("    %010x role=%s latency=%dms paths=%d\n",
					p.Address, p.Role, p.Latency, len(p.Paths))
			}
		}
	}
}
