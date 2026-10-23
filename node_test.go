package zt

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"testing"
)

func testNodeCallbacks() NodeCallbacks {
	return NodeCallbacks{
		StatePut: func(StateObjectType, [2]uint64, []byte) {},
		StateGet: func(StateObjectType, [2]uint64) ([]byte, bool) {
			return nil, false
		},
		WirePacketSend: func(int64, netip.AddrPort, []byte, uint) error {
			return nil
		},
		VirtualNetworkFrame:  func(uint64, uint64, uint64, uint, uint, []byte) {},
		VirtualNetworkConfig: func(uint64, VirtualNetworkConfigOperation, *VirtualNetworkConfig) int { return 0 },
		Event:                func(Event, any) {},
	}
}

func newTestNode(t *testing.T) *Node {
	t.Helper()

	node, err := NewNode(testNodeCallbacks(), nil)
	if err != nil {
		t.Fatalf("NewNode() error = %v", err)
	}
	return node
}

func TestNetworkConfigPreservesNullTargetRoute(t *testing.T) {
	const nwid = uint64(0x8056c2e21c000001)
	// Serialized routes: 192.0.2.0/24 on-link, then a null target via
	// 192.0.2.1 with metric 7. The dictionary escapes each NUL as \0.
	const routes = "\x04\xc0\x00\x02\x00\x00\x18\x00\x00\x00\x00\x00" +
		"\x00\x04\xc0\x00\x02\x01\x00\x00\x00\x00\x00\x07"
	var address uint64
	var callbackConfig *VirtualNetworkConfig
	callbacks := testNodeCallbacks()
	callbacks.StateGet = func(kind StateObjectType, id [2]uint64) ([]byte, bool) {
		if kind != StateObjectNetworkConfig {
			return nil, false
		}
		config := fmt.Sprintf("v=6\nnwid=%016x\nid=%010x\nRT=%s\n",
			id[0], address, strings.ReplaceAll(routes, "\x00", "\\0"))
		return []byte(config), true
	}
	callbacks.VirtualNetworkConfig = func(_ uint64, _ VirtualNetworkConfigOperation, config *VirtualNetworkConfig) int {
		callbackConfig = config
		return 0
	}
	node, err := NewNode(callbacks, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(node.Close)
	address = node.Address()
	if err := node.Join(nwid); err != nil {
		t.Fatal(err)
	}
	networks := node.Networks()
	if len(networks) != 1 {
		t.Fatalf("Networks() returned %d networks, want 1", len(networks))
	}
	want := []Route{
		{Target: netip.MustParsePrefix("192.0.2.0/24")},
		{Target: netip.MustParsePrefix("0.0.0.0/0"), Via: netip.MustParseAddr("192.0.2.1"), Metric: 7},
	}
	for name, config := range map[string]*VirtualNetworkConfig{
		"callback":      callbackConfig,
		"NetworkConfig": node.NetworkConfig(nwid),
		"Networks":      &networks[0],
	} {
		t.Run(name, func(t *testing.T) {
			if config == nil || config.Status != NetworkStatusOK {
				t.Fatal("persisted network configuration was not accepted")
			}
			if !slices.Equal(config.Routes, want) {
				t.Fatalf("Routes = %+v, want %+v", config.Routes, want)
			}
		})
	}
}

func TestNodeCloseIsIdempotent(t *testing.T) {
	node := newTestNode(t)

	for i := 0; i < 2; i++ {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Close() panicked on call %d: %v", i+1, r)
				}
			}()
			node.Close()
		}()
	}
}

func TestProcessWirePacketAllowsEmptyPayload(t *testing.T) {
	node := newTestNode(t)
	defer node.Close()

	addr := netip.MustParseAddrPort("127.0.0.1:9993")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ProcessWirePacket() panicked with empty payload: %v", r)
		}
	}()

	_ = node.ProcessWirePacket(0, addr, nil, nil)
}

func TestProcessVirtualNetworkFrameAllowsEmptyPayload(t *testing.T) {
	node := newTestNode(t)
	defer node.Close()

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("ProcessVirtualNetworkFrame() panicked with empty payload: %v", r)
		}
	}()

	_ = node.ProcessVirtualNetworkFrame(0, 0, 0, 0x0800, 0, nil, nil)
}

var _ func(*Node, netip.AddrPort) error = (*Node).AddLocalInterfaceAddress
var _ func(*Node, uint64, uint64, []byte) error = (*Node).SendUserMessage

func TestBoolResultToError(t *testing.T) {
	if err := boolResultToError(true, ErrUserMessageRejected); err != nil {
		t.Fatalf("boolResultToError(true, ...) error = %v, want nil", err)
	}
	if err := boolResultToError(false, ErrUserMessageRejected); err != ErrUserMessageRejected {
		t.Fatalf("boolResultToError(false, ...) error = %v, want %v", err, ErrUserMessageRejected)
	}
}

func TestNodeMethodPanicsAfterClose(t *testing.T) {
	node := newTestNode(t)
	node.Close()

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Address() did not panic after Close()")
		}
		if got, want := r, "zt: use of closed Node"; got != want {
			t.Fatalf("panic = %v, want %q", got, want)
		}
	}()

	_ = node.Address()
}

func TestWarnUsesOptionalLogger(t *testing.T) {
	var got string
	node := &Node{callbacks: NodeCallbacks{Logger: func(msg string) {
		got = msg
	}}}

	node.warn("hello")

	if got != "hello" {
		t.Fatalf("warn() logged %q, want %q", got, "hello")
	}
}

func TestWarnAllowsNilLogger(t *testing.T) {
	node := &Node{}
	node.warn("ignored")
}
