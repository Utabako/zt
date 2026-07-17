package zt

import (
	"net/netip"
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
