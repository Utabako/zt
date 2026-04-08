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
	"unsafe"
)

// NodeCallbacks defines all callback functions for a ZeroTier node.
// Required callbacks must be set before creating a node.
type NodeCallbacks struct {
	// StatePut stores or deletes a state object.
	// When data is nil, the object should be deleted.
	// Required.
	StatePut func(objType StateObjectType, id [2]uint64, data []byte)

	// StateGet retrieves a state object.
	// Returns the data and true if found, or nil and false if not found.
	// Required.
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

	// Logger receives internal warnings. Optional.
	// If nil, internal warnings are dropped.
	Logger func(msg string)
}

// --- CGo callback exports ---

//export goStatePutCallback
func goStatePutCallback(uptr unsafe.Pointer, objType C.int, id0 C.uint64_t, id1 C.uint64_t, data unsafe.Pointer, dataLen C.int) {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.StatePut == nil {
		return
	}
	var buf []byte
	if dataLen > 0 && data != nil {
		buf = C.GoBytes(data, dataLen)
	}
	n.callbacks.StatePut(StateObjectType(objType), [2]uint64{uint64(id0), uint64(id1)}, buf)
}

//export goStateGetCallback
func goStateGetCallback(uptr unsafe.Pointer, objType C.int, id0 C.uint64_t, id1 C.uint64_t, buf unsafe.Pointer, bufLen C.uint) C.int {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.StateGet == nil {
		return -1
	}
	objTypeGo := StateObjectType(objType)
	data, ok := n.callbacks.StateGet(objTypeGo, [2]uint64{uint64(id0), uint64(id1)})
	if !ok || len(data) == 0 {
		return -1
	}
	if uint(len(data)) > uint(bufLen) {
		n.warn(fmt.Sprintf("zt: state object truncated in StateGet callback: type=%v id=%016x/%016x len=%d bufLen=%d", objTypeGo, uint64(id0), uint64(id1), len(data), bufLen))
		return -1
	}
	C.memcpy(buf, unsafe.Pointer(&data[0]), C.size_t(len(data)))
	return C.int(len(data))
}

//export goWirePacketSendCallback
func goWirePacketSendCallback(uptr unsafe.Pointer, localSocket C.int64_t, remoteAddr *C.struct_sockaddr_storage, data unsafe.Pointer, dataLen C.uint, ttl C.uint) C.int {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.WirePacketSend == nil {
		return -1
	}
	addr, _ := sockaddrToAddrPort(remoteAddr)
	buf := C.GoBytes(data, C.int(dataLen))
	err := n.callbacks.WirePacketSend(int64(localSocket), addr, buf, uint(ttl))
	if err != nil {
		return -1
	}
	return 0
}

//export goVirtualNetworkFrameCallback
func goVirtualNetworkFrameCallback(uptr unsafe.Pointer, nwid C.uint64_t, sourceMac C.uint64_t, destMac C.uint64_t, etherType C.uint, vlanID C.uint, data unsafe.Pointer, dataLen C.uint) {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.VirtualNetworkFrame == nil {
		return
	}
	buf := C.GoBytes(data, C.int(dataLen))
	n.callbacks.VirtualNetworkFrame(uint64(nwid), uint64(sourceMac), uint64(destMac), uint(etherType), uint(vlanID), buf)
}

//export goVirtualNetworkConfigCallback
func goVirtualNetworkConfigCallback(uptr unsafe.Pointer, nwid C.uint64_t, op C.int, config *C.ZT_VirtualNetworkConfig) C.int {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.VirtualNetworkConfig == nil {
		return 0
	}
	goConfig := convertVirtualNetworkConfig(config)
	return C.int(n.callbacks.VirtualNetworkConfig(uint64(nwid), VirtualNetworkConfigOperation(op), goConfig))
}

//export goEventCallback
func goEventCallback(uptr unsafe.Pointer, event C.int, data unsafe.Pointer) {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.Event == nil {
		return
	}
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
	n.callbacks.Event(ev, payload)
}

//export goPathCheckCallback
func goPathCheckCallback(uptr unsafe.Pointer, ztAddress C.uint64_t, localSocket C.int64_t, remoteAddr *C.struct_sockaddr_storage) C.int {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.PathCheck == nil {
		return 1
	}
	addr, _ := sockaddrToAddrPort(remoteAddr)
	if n.callbacks.PathCheck(uint64(ztAddress), int64(localSocket), addr) {
		return 1
	}
	return 0
}

//export goPathLookupCallback
func goPathLookupCallback(uptr unsafe.Pointer, ztAddress C.uint64_t, family C.int, result *C.struct_sockaddr_storage) C.int {
	h := cgo.Handle(uintptr(uptr))
	n := h.Value().(*Node)
	if n.callbacks.PathLookup == nil {
		return 0
	}
	ap := n.callbacks.PathLookup(uint64(ztAddress), int(family))
	if ap == nil {
		return 0
	}
	addrPortToSockaddr(*ap, result)
	return 1
}

func (n *Node) warn(msg string) {
	if n.callbacks.Logger != nil {
		n.callbacks.Logger(msg)
	}
}
