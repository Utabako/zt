package zt

import (
	"context"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
	"time"
)

func TestNodeLifecycleHandlesCallbackReentryAndClose(t *testing.T) {
	const childEnvironment = "GO_ZT_LIFECYCLE_CHILD"
	if os.Getenv(childEnvironment) == "1" {
		nodeLifecycleChild(t)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestNodeLifecycleHandlesCallbackReentryAndClose$")
	cmd.Env = append(os.Environ(), childEnvironment+"=1")
	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("node lifecycle child timed out: %v\n%s", ctx.Err(), output)
	}
	if err != nil {
		t.Fatalf("node lifecycle child failed: %v\n%s", err, output)
	}
}

func nodeLifecycleChild(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	addressResult := make(chan uint64, 1)
	var downCount atomic.Int32
	var node *Node

	callbacks := testNodeCallbacks()
	callbacks.VirtualNetworkConfig = func(_ uint64, op VirtualNetworkConfigOperation, _ *VirtualNetworkConfig) int {
		if op == VirtualNetworkConfigOperationUp {
			addressResult <- node.Address()
			close(entered)
			<-release
		}
		return 0
	}
	callbacks.Event = func(event Event, _ any) {
		if event == EventDown {
			downCount.Add(1)
			panic("down callback exploded")
		}
	}

	var err error
	node, err = NewNode(callbacks, nil)
	if err != nil {
		t.Fatalf("NewNode() error = %v", err)
	}
	wantAddress := node.Address()

	joinDone := make(chan error, 1)
	go func() {
		joinDone <- node.Join(0x8056c2e21c000001)
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("VirtualNetworkConfig callback was not entered")
	}
	if got := <-addressResult; got != wantAddress {
		t.Fatalf("Address() from callback = %x, want %x", got, wantAddress)
	}

	closeDone := make(chan struct{}, 3)
	for i := 0; i < 3; i++ {
		go func() {
			node.Close()
			closeDone <- struct{}{}
		}()
	}
	select {
	case <-closeDone:
		t.Fatal("Close() returned while a C call was active")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case err := <-joinDone:
		if err != nil {
			t.Fatalf("Join() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Join() did not return after callback release")
	}
	for i := 0; i < 3; i++ {
		select {
		case <-closeDone:
		case <-time.After(5 * time.Second):
			t.Fatal("Close() did not finish")
		}
	}
	if got := downCount.Load(); got != 1 {
		t.Fatalf("EventDown count = %d, want 1", got)
	}
}
