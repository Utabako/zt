package zt

/*
#include <stdbool.h>
#include <string.h>
#include "ZeroTierOne.h"
*/
import "C"

import (
	"fmt"
	"net/netip"
	"runtime/cgo"
	"runtime/debug"
	"strings"
	"unsafe"
)

// NodeCallbacks defines all callback functions for a ZeroTier node.
// NewNode rejects a missing required callback. The core may invoke callbacks on
// background threads; except for Node.Address, they must not call Node methods.
// Panics are contained at the CGo boundary and reported through Logger when set.
type NodeCallbacks struct {
	// StatePut stores or deletes a state object.
	// Nil data deletes the object; a non-nil empty slice stores an empty object.
	// Required.
	StatePut func(objType StateObjectType, id [2]uint64, data []byte)

	// StateGet retrieves a state object.
	// Returns data and true if found, including an empty slice for an empty
	// object, or nil and false if not found. Required.
	StateGet func(objType StateObjectType, id [2]uint64) ([]byte, bool)

	// WirePacketSend sends a ZeroTier packet over the physical wire.
	// Required.
	WirePacketSend func(localSocket int64, addr netip.AddrPort, data []byte, ttl uint) error

	// VirtualNetworkFrame delivers an Ethernet frame from a virtual network.
	// Required.
	VirtualNetworkFrame func(nwid uint64, sourceMac, destMac uint64, etherType, vlanID uint, data []byte)

	// VirtualNetworkConfig is called when virtual network configuration changes.
	// Must return 0 on success or an OS-dependent error code on failure.
	// Required.
	VirtualNetworkConfig func(nwid uint64, op VirtualNetworkConfigOperation, config *VirtualNetworkConfig) int

	// Event is called when significant events occur.
	// Required.
	Event func(event Event, data any)

	// PathCheck checks whether a physical path should be used.
	// Return true to allow the path. Optional.
	PathCheck func(ztAddress uint64, localSocket int64, addr netip.AddrPort) bool

	// PathLookup returns a physical address hint for reaching a ZeroTier address.
	// Return nil if no hint is available. Optional.
	PathLookup func(ztAddress uint64, family int) *netip.AddrPort

	// Logger receives internal warnings and recovered callback panics. Optional.
	// If nil, internal warnings are dropped.
	Logger func(msg string)
}

func validateNodeCallbacks(callbacks NodeCallbacks) error {
	missing := make([]string, 0, 6)
	if callbacks.StatePut == nil {
		missing = append(missing, "StatePut")
	}
	if callbacks.StateGet == nil {
		missing = append(missing, "StateGet")
	}
	if callbacks.WirePacketSend == nil {
		missing = append(missing, "WirePacketSend")
	}
	if callbacks.VirtualNetworkFrame == nil {
		missing = append(missing, "VirtualNetworkFrame")
	}
	if callbacks.VirtualNetworkConfig == nil {
		missing = append(missing, "VirtualNetworkConfig")
	}
	if callbacks.Event == nil {
		missing = append(missing, "Event")
	}
	if len(missing) != 0 {
		return fmt.Errorf("zt: missing required callbacks: %s", strings.Join(missing, ", "))
	}
	return nil
}

func callbackNode(handle C.uintptr_t) *Node {
	return cgo.Handle(uintptr(handle)).Value().(*Node)
}

func recoverCallbackPanic(node **Node, name string) {
	recovered := recover()
	if recovered == nil || *node == nil {
		return
	}
	(*node).warn(fmt.Sprintf("zt: panic in %s callback: %v\n%s", name, recovered, debug.Stack()))
}

func (n *Node) statePutCallback(objType StateObjectType, id [2]uint64, data unsafe.Pointer, dataLen int) {
	var copied []byte
	switch {
	case dataLen == -1:
	case dataLen == 0:
		copied = []byte{}
	case dataLen > 0 && data != nil:
		copied = C.GoBytes(data, C.int(dataLen))
	default:
		n.warn(fmt.Sprintf("zt: invalid StatePut payload length=%d data=%p", dataLen, data))
		return
	}
	n.callbacks.StatePut(objType, id, copied)
}

func (n *Node) stateGetCallback(objType StateObjectType, id [2]uint64, buf unsafe.Pointer, bufLen uint) int {
	data, found := n.callbacks.StateGet(objType, id)
	if !found {
		return -1
	}
	if len(data) == 0 {
		return 0
	}
	if uint(len(data)) > bufLen {
		n.warn(fmt.Sprintf("zt: state object truncated in StateGet callback: type=%v id=%016x/%016x len=%d bufLen=%d", objType, id[0], id[1], len(data), bufLen))
		return -1
	}
	if buf == nil {
		n.warn(fmt.Sprintf("zt: nil StateGet buffer: type=%v id=%016x/%016x len=%d", objType, id[0], id[1], len(data)))
		return -1
	}
	C.memcpy(buf, unsafe.Pointer(&data[0]), C.size_t(len(data)))
	return len(data)
}

//export goStatePutCallback
func goStatePutCallback(handle C.uintptr_t, objType C.int, id0 C.uint64_t, id1 C.uint64_t, data unsafe.Pointer, dataLen C.int) {
	var n *Node
	defer recoverCallbackPanic(&n, "StatePut")
	n = callbackNode(handle)
	n.statePutCallback(StateObjectType(objType), [2]uint64{uint64(id0), uint64(id1)}, data, int(dataLen))
}

//export goStateGetCallback
func goStateGetCallback(handle C.uintptr_t, objType C.int, id0 C.uint64_t, id1 C.uint64_t, buf unsafe.Pointer, bufLen C.uint) (result C.int) {
	result = -1
	var n *Node
	defer recoverCallbackPanic(&n, "StateGet")
	n = callbackNode(handle)
	return C.int(n.stateGetCallback(
		StateObjectType(objType),
		[2]uint64{uint64(id0), uint64(id1)},
		buf,
		uint(bufLen),
	))
}

//export goWirePacketSendCallback
func goWirePacketSendCallback(handle C.uintptr_t, localSocket C.int64_t, remoteAddr *C.struct_sockaddr_storage, data unsafe.Pointer, dataLen C.uint, ttl C.uint) (result C.int) {
	result = -1
	var n *Node
	defer recoverCallbackPanic(&n, "WirePacketSend")
	n = callbackNode(handle)
	addr, ok := sockaddrToAddrPort(remoteAddr)
	if !ok {
		n.warn("zt: invalid remote address in WirePacketSend callback")
		return
	}
	buf := C.GoBytes(data, C.int(dataLen))
	if err := n.callbacks.WirePacketSend(int64(localSocket), addr, buf, uint(ttl)); err == nil {
		result = 0
	}
	return
}

//export goVirtualNetworkFrameCallback
func goVirtualNetworkFrameCallback(handle C.uintptr_t, nwid C.uint64_t, sourceMac C.uint64_t, destMac C.uint64_t, etherType C.uint, vlanID C.uint, data unsafe.Pointer, dataLen C.uint) {
	var n *Node
	defer recoverCallbackPanic(&n, "VirtualNetworkFrame")
	n = callbackNode(handle)
	buf := C.GoBytes(data, C.int(dataLen))
	n.callbacks.VirtualNetworkFrame(uint64(nwid), uint64(sourceMac), uint64(destMac), uint(etherType), uint(vlanID), buf)
}

//export goVirtualNetworkConfigCallback
func goVirtualNetworkConfigCallback(handle C.uintptr_t, nwid C.uint64_t, op C.int, config *C.ZT_VirtualNetworkConfig) (result C.int) {
	result = -1
	var n *Node
	defer recoverCallbackPanic(&n, "VirtualNetworkConfig")
	n = callbackNode(handle)
	goConfig := convertVirtualNetworkConfig(config)
	return C.int(n.callbacks.VirtualNetworkConfig(uint64(nwid), VirtualNetworkConfigOperation(op), goConfig))
}

//export goEventCallback
func goEventCallback(handle C.uintptr_t, event C.int, data unsafe.Pointer) {
	var n *Node
	defer recoverCallbackPanic(&n, "Event")
	n = callbackNode(handle)
	ev := Event(event)
	var payload any
	switch ev {
	case EventUserMessage:
		if data != nil {
			cm := (*C.ZT_UserMessage)(data)
			payload = &UserMessage{
				Origin: uint64(cm.origin),
				TypeID: uint64(cm.typeId),
				Data:   C.GoBytes(cm.data, C.int(cm.length)),
			}
		}
	case EventRemoteTrace:
		if data != nil {
			ct := (*C.ZT_RemoteTrace)(data)
			payload = &RemoteTrace{
				Origin: uint64(ct.origin),
				Data:   C.GoString(ct.data),
			}
		}
	case EventTrace:
		if data != nil {
			payload = C.GoString((*C.char)(data))
		}
	}
	n.emitEvent(ev, payload)
}

func (n *Node) emitEvent(event Event, data any) {
	defer recoverCallbackPanic(&n, "Event")
	if event == EventDown {
		n.downOnce.Do(func() {
			n.callbacks.Event(event, data)
		})
		return
	}
	n.callbacks.Event(event, data)
}

//export goPathCheckCallback
func goPathCheckCallback(handle C.uintptr_t, ztAddress C.uint64_t, localSocket C.int64_t, remoteAddr *C.struct_sockaddr_storage) (result C.int) {
	var n *Node
	defer recoverCallbackPanic(&n, "PathCheck")
	n = callbackNode(handle)
	if n.callbacks.PathCheck == nil {
		return 1
	}
	addr, ok := sockaddrToAddrPort(remoteAddr)
	if !ok {
		n.warn("zt: invalid remote address in PathCheck callback")
		return 0
	}
	if n.callbacks.PathCheck(uint64(ztAddress), int64(localSocket), addr) {
		return 1
	}
	return 0
}

//export goPathLookupCallback
func goPathLookupCallback(handle C.uintptr_t, ztAddress C.uint64_t, family C.int, result *C.struct_sockaddr_storage) (found C.int) {
	var n *Node
	defer recoverCallbackPanic(&n, "PathLookup")
	n = callbackNode(handle)
	if n.callbacks.PathLookup == nil {
		return 0
	}
	ap := n.callbacks.PathLookup(uint64(ztAddress), int(family))
	if ap == nil {
		return 0
	}
	sa, err := addrPortToSockaddr(*ap)
	if err != nil {
		n.warn(fmt.Sprintf("zt: invalid address from PathLookup callback: %v", err))
		return 0
	}
	*result = sa
	return 1
}

func (n *Node) warn(msg string) {
	logger := n.callbacks.Logger
	if logger == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	logger(msg)
}
