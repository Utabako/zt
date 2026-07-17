package zt

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewNodeRejectsMissingRequiredCallbacks(t *testing.T) {
	tests := []struct {
		name  string
		clear func(*NodeCallbacks)
	}{
		{name: "StatePut", clear: func(callbacks *NodeCallbacks) { callbacks.StatePut = nil }},
		{name: "StateGet", clear: func(callbacks *NodeCallbacks) { callbacks.StateGet = nil }},
		{name: "WirePacketSend", clear: func(callbacks *NodeCallbacks) { callbacks.WirePacketSend = nil }},
		{name: "VirtualNetworkFrame", clear: func(callbacks *NodeCallbacks) { callbacks.VirtualNetworkFrame = nil }},
		{name: "VirtualNetworkConfig", clear: func(callbacks *NodeCallbacks) { callbacks.VirtualNetworkConfig = nil }},
		{name: "Event", clear: func(callbacks *NodeCallbacks) { callbacks.Event = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			callbacks := testNodeCallbacks()
			test.clear(&callbacks)

			node, err := NewNode(callbacks, nil)
			if node != nil {
				node.Close()
				t.Fatal("NewNode() returned a node with a missing required callback")
			}
			if err == nil || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("NewNode() error = %v, want missing callback %s", err, test.name)
			}
		})
	}
}

func TestNewNodeReportsAllMissingCallbacks(t *testing.T) {
	_, err := NewNode(NodeCallbacks{}, nil)
	const want = "zt: missing required callbacks: StatePut, StateGet, WirePacketSend, VirtualNetworkFrame, VirtualNetworkConfig, Event"
	if err == nil || err.Error() != want {
		t.Fatalf("NewNode() error = %v, want %q", err, want)
	}
}

func TestStatePutDistinguishesDeleteAndEmptyObject(t *testing.T) {
	var values [][]byte
	callbacks := testNodeCallbacks()
	callbacks.StatePut = func(_ StateObjectType, _ [2]uint64, data []byte) {
		values = append(values, data)
	}
	node := &Node{callbacks: callbacks}

	node.statePutCallback(StateObjectIdentitySecret, [2]uint64{}, nil, -1)
	node.statePutCallback(StateObjectIdentitySecret, [2]uint64{}, nil, 0)

	if len(values) != 2 {
		t.Fatalf("StatePut calls = %d, want 2", len(values))
	}
	if values[0] != nil {
		t.Fatalf("delete payload = %#v, want nil", values[0])
	}
	if values[1] == nil || len(values[1]) != 0 {
		t.Fatalf("empty object payload = %#v, want non-nil empty slice", values[1])
	}
}

func TestStateGetReturnsZeroForPresentEmptyObject(t *testing.T) {
	callbacks := testNodeCallbacks()
	callbacks.StateGet = func(StateObjectType, [2]uint64) ([]byte, bool) {
		return []byte{}, true
	}
	node := &Node{callbacks: callbacks}

	if got := node.stateGetCallback(StateObjectIdentitySecret, [2]uint64{}, nil, 0); got != 0 {
		t.Fatalf("stateGetCallback() = %d, want 0", got)
	}
}

func TestCallbackPanicDoesNotCrossCgo(t *testing.T) {
	const childEnvironment = "GO_ZT_CALLBACK_PANIC_CHILD"
	if os.Getenv(childEnvironment) == "1" {
		callbackPanicChild(t)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCallbackPanicDoesNotCrossCgo$")
	cmd.Env = append(os.Environ(), childEnvironment+"=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("callback panic child timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("callback panic child failed: %v\n%s", err, output)
	}
}

func callbackPanicChild(t *testing.T) {
	logged := false
	callbacks := testNodeCallbacks()
	callbacks.VirtualNetworkConfig = func(uint64, VirtualNetworkConfigOperation, *VirtualNetworkConfig) int {
		panic("callback exploded")
	}
	callbacks.Logger = func(message string) {
		if strings.Contains(message, "panic in VirtualNetworkConfig callback: callback exploded") {
			logged = true
		}
	}

	node, err := NewNode(callbacks, nil)
	if err != nil {
		t.Fatalf("NewNode() error = %v", err)
	}
	_ = node.Join(0x8056c2e21c000001)
	node.Close()
	if !logged {
		t.Fatal("callback panic was not reported to Logger")
	}
}

func TestWarnRecoversLoggerPanic(t *testing.T) {
	node := &Node{callbacks: NodeCallbacks{Logger: func(string) { panic("logger exploded") }}}
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("warn() propagated Logger panic: %v", recovered)
		}
	}()
	node.warn("warning")
}
