package zt

/*
#include <stdbool.h>
#include "ZeroTierOne.h"
#include "shim.h"
*/
import "C"

import (
	"net/netip"
	"runtime/cgo"
	"sync"
	"time"
	"unsafe"
)

type nodeState uint8

const (
	nodeStateOpen nodeState = iota
	nodeStateClosing
	nodeStateClosed
)

// Node represents a ZeroTier node instance. Its methods are safe for concurrent use.
type Node struct {
	cNode         unsafe.Pointer // *C.ZT_Node (typedef void)
	handle        cgo.Handle
	callbacks     NodeCallbacks
	lifecycleMu   sync.Mutex
	lifecycleCond *sync.Cond
	activeCalls   int
	state         nodeState
	callMu        sync.Mutex
	downOnce      sync.Once
}

// NewNode creates a new ZeroTier node with the given callbacks and optional config.
// It rejects missing required callbacks before creating the core node.
// If config is nil, default configuration is used.
func NewNode(callbacks NodeCallbacks, config *NodeConfig) (*Node, error) {
	if err := validateNodeCallbacks(callbacks); err != nil {
		return nil, err
	}

	n := &Node{callbacks: callbacks}
	n.lifecycleCond = sync.NewCond(&n.lifecycleMu)
	n.handle = cgo.NewHandle(n)

	var cCallbacks C.struct_ZT_Node_Callbacks
	C.zt_init_callbacks(&cCallbacks)

	var cConfig C.struct_ZT_Node_Config
	if config != nil {
		if config.EnableEncryptedHello {
			cConfig.enableEncryptedHello = 1
		}
		if config.LowBandwidthMode {
			cConfig.lowBandwidthMode = 1
		}
	}

	now := C.int64_t(time.Now().UnixMilli())

	var cNode unsafe.Pointer
	rc := C.zt_node_new(
		&cNode,
		&cConfig,
		C.uintptr_t(n.handle),
		nil,
		&cCallbacks,
		now,
	)

	if err := resultToError(rc); err != nil {
		n.handle.Delete()
		return nil, err
	}

	n.cNode = cNode
	return n, nil
}

// Close waits for in-flight calls, emits EventDown once, and frees the core node.
// It is safe to call Close multiple times. EventDown handlers must not call Node methods.
func (n *Node) Close() {
	n.lifecycleMu.Lock()
	if n.lifecycleCond == nil {
		n.lifecycleCond = sync.NewCond(&n.lifecycleMu)
	}
	switch n.state {
	case nodeStateClosed:
		n.lifecycleMu.Unlock()
		return
	case nodeStateClosing:
		for n.state != nodeStateClosed {
			n.lifecycleCond.Wait()
		}
		n.lifecycleMu.Unlock()
		return
	}

	n.state = nodeStateClosing
	for n.activeCalls != 0 {
		n.lifecycleCond.Wait()
	}
	cNode := n.cNode
	n.cNode = nil
	handle := n.handle
	n.lifecycleMu.Unlock()

	if cNode != nil {
		n.emitEvent(EventDown, nil)
		C.ZT_Node_delete(cNode)
	}
	if handle != 0 {
		handle.Delete()
	}

	n.lifecycleMu.Lock()
	n.handle = 0
	n.state = nodeStateClosed
	n.lifecycleCond.Broadcast()
	n.lifecycleMu.Unlock()
}

func (n *Node) beginCall() unsafe.Pointer {
	n.lifecycleMu.Lock()
	defer n.lifecycleMu.Unlock()
	if n.state != nodeStateOpen || n.cNode == nil {
		panic("zt: use of closed Node")
	}
	n.activeCalls++
	return n.cNode
}

func (n *Node) endCall() {
	n.lifecycleMu.Lock()
	n.activeCalls--
	if n.activeCalls == 0 && n.lifecycleCond != nil {
		n.lifecycleCond.Broadcast()
	}
	n.lifecycleMu.Unlock()
}

func (n *Node) beginSerializedCall() unsafe.Pointer {
	cNode := n.beginCall()
	n.callMu.Lock()
	return cNode
}

func (n *Node) endSerializedCall() {
	n.callMu.Unlock()
	n.endCall()
}

// Address returns this node's 40-bit ZeroTier address.
// Address is the only Node method that may be called from a callback while the node is open.
func (n *Node) Address() uint64 {
	cNode := n.beginCall()
	defer n.endCall()
	return uint64(C.ZT_Node_address(cNode))
}

// Status returns the current status of this node.
func (n *Node) Status() NodeStatus {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	var cs C.ZT_NodeStatus
	C.ZT_Node_status(cNode, &cs)
	return convertNodeStatus(&cs)
}

// ProcessWirePacket processes a packet received from the physical wire.
func (n *Node) ProcessWirePacket(localSocket int64, remoteAddress netip.AddrPort, data []byte, nextDeadline *int64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()

	sa, err := addrPortToSockaddr(remoteAddress)
	if err != nil {
		return err
	}

	now := C.int64_t(time.Now().UnixMilli())
	var deadline C.int64_t

	rc := C.ZT_Node_processWirePacket(
		cNode,
		nil,
		now,
		C.int64_t(localSocket),
		&sa,
		sliceDataPointer(data),
		C.uint(len(data)),
		&deadline,
	)

	if nextDeadline != nil {
		*nextDeadline = int64(deadline)
	}

	return resultToError(rc)
}

// ProcessVirtualNetworkFrame processes a frame from a virtual network port.
func (n *Node) ProcessVirtualNetworkFrame(nwid uint64, sourceMac, destMac uint64, etherType, vlanID uint, data []byte, nextDeadline *int64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()

	now := C.int64_t(time.Now().UnixMilli())
	var deadline C.int64_t

	rc := C.ZT_Node_processVirtualNetworkFrame(
		cNode,
		nil,
		now,
		C.uint64_t(nwid),
		C.uint64_t(sourceMac),
		C.uint64_t(destMac),
		C.uint(etherType),
		C.uint(vlanID),
		sliceDataPointer(data),
		C.uint(len(data)),
		&deadline,
	)

	if nextDeadline != nil {
		*nextDeadline = int64(deadline)
	}

	return resultToError(rc)
}

// ProcessBackgroundTasks performs periodic background operations.
func (n *Node) ProcessBackgroundTasks(nextDeadline *int64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()

	now := C.int64_t(time.Now().UnixMilli())
	var deadline C.int64_t

	rc := C.ZT_Node_processBackgroundTasks(cNode, nil, now, &deadline)

	if nextDeadline != nil {
		*nextDeadline = int64(deadline)
	}

	return resultToError(rc)
}

// Join joins a virtual network.
func (n *Node) Join(nwid uint64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	rc := C.ZT_Node_join(cNode, C.uint64_t(nwid), nil, nil)
	return resultToError(rc)
}

// Leave leaves a virtual network.
func (n *Node) Leave(nwid uint64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	rc := C.ZT_Node_leave(cNode, C.uint64_t(nwid), nil, nil)
	return resultToError(rc)
}

// MulticastSubscribe subscribes to an Ethernet multicast group.
func (n *Node) MulticastSubscribe(nwid uint64, multicastGroup uint64, multicastADI uint32) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	rc := C.ZT_Node_multicastSubscribe(cNode, nil, C.uint64_t(nwid), C.uint64_t(multicastGroup), C.ulong(multicastADI))
	return resultToError(rc)
}

// MulticastUnsubscribe unsubscribes from an Ethernet multicast group.
func (n *Node) MulticastUnsubscribe(nwid uint64, multicastGroup uint64, multicastADI uint32) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	rc := C.ZT_Node_multicastUnsubscribe(cNode, C.uint64_t(nwid), C.uint64_t(multicastGroup), C.ulong(multicastADI))
	return resultToError(rc)
}

// Orbit adds or updates a moon (federated root set).
func (n *Node) Orbit(moonWorldID uint64, moonSeed uint64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	rc := C.ZT_Node_orbit(cNode, nil, C.uint64_t(moonWorldID), C.uint64_t(moonSeed))
	return resultToError(rc)
}

// Deorbit removes a moon.
func (n *Node) Deorbit(moonWorldID uint64) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	rc := C.ZT_Node_deorbit(cNode, nil, C.uint64_t(moonWorldID))
	return resultToError(rc)
}

// Peers returns a list of all known peers.
func (n *Node) Peers() []Peer {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	cList := C.ZT_Node_peers(cNode)
	if cList == nil {
		return nil
	}
	defer C.ZT_Node_freeQueryResult(cNode, unsafe.Pointer(cList))

	count := int(cList.peerCount)
	if count == 0 {
		return nil
	}

	cPeers := unsafe.Slice(cList.peers, count)
	peers := make([]Peer, count)
	for i := 0; i < count; i++ {
		peers[i] = convertPeer(&cPeers[i])
	}
	return peers
}

// NetworkConfig returns the configuration of a joined network.
func (n *Node) NetworkConfig(nwid uint64) *VirtualNetworkConfig {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	cc := C.ZT_Node_networkConfig(cNode, C.uint64_t(nwid))
	if cc == nil {
		return nil
	}
	defer C.ZT_Node_freeQueryResult(cNode, unsafe.Pointer(cc))

	return convertVirtualNetworkConfig(cc)
}

// Networks returns a list of all joined networks.
func (n *Node) Networks() []VirtualNetworkConfig {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	cList := C.ZT_Node_networks(cNode)
	if cList == nil {
		return nil
	}
	defer C.ZT_Node_freeQueryResult(cNode, unsafe.Pointer(cList))

	count := int(cList.networkCount)
	if count == 0 {
		return nil
	}

	cNets := unsafe.Slice(cList.networks, count)
	nets := make([]VirtualNetworkConfig, count)
	for i := 0; i < count; i++ {
		nets[i] = *convertVirtualNetworkConfig(&cNets[i])
	}
	return nets
}

// AddLocalInterfaceAddress makes ZeroTier aware of a local interface address.
// It returns ErrLocalInterfaceAddressRejected if the core rejects the address.
func (n *Node) AddLocalInterfaceAddress(addr netip.AddrPort) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()

	sa, err := addrPortToSockaddr(addr)
	if err != nil {
		return err
	}
	return boolResultToError(C.ZT_Node_addLocalInterfaceAddress(cNode, &sa) != 0, ErrLocalInterfaceAddressRejected)
}

// ClearLocalInterfaceAddresses clears all local interface addresses.
func (n *Node) ClearLocalInterfaceAddresses() {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	C.ZT_Node_clearLocalInterfaceAddresses(cNode)
}

// SendUserMessage sends a VERB_USER_MESSAGE to another ZeroTier node.
// It returns ErrUserMessageRejected if the core rejects the message.
func (n *Node) SendUserMessage(dest uint64, typeID uint64, data []byte) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()

	dataPtr := sliceDataPointer(data)
	return boolResultToError(
		C.ZT_Node_sendUserMessage(cNode, nil, C.uint64_t(dest), C.uint64_t(typeID), dataPtr, C.uint(len(data))) != 0,
		ErrUserMessageRejected,
	)
}

// SetPhysicalPathConfiguration sets configuration for a given physical path.
func (n *Node) SetPhysicalPathConfiguration(pathNetwork netip.AddrPort, trustedPathID uint64, mtu int) error {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()

	sa, err := addrPortToSockaddr(pathNetwork)
	if err != nil {
		return err
	}

	var config C.ZT_PhysicalPathConfiguration
	config.trustedPathId = C.uint64_t(trustedPathID)
	config.mtu = C.int(mtu)

	rc := C.ZT_Node_setPhysicalPathConfiguration(cNode, &sa, &config)
	return resultToError(rc)
}

// SetNetconfMaster sets a network configuration master instance for this node.
// This is for nodes with compiled-in support for acting as network controllers.
// Pass nil to disable.
func (n *Node) SetNetconfMaster(instance unsafe.Pointer) {
	cNode := n.beginSerializedCall()
	defer n.endSerializedCall()
	C.ZT_Node_setNetconfMaster(cNode, instance)
}

// Version returns the ZeroTier core library version.
func Version() (major, minor, revision int) {
	var cMajor, cMinor, cRev C.int
	C.ZT_version(&cMajor, &cMinor, &cRev)
	return int(cMajor), int(cMinor), int(cRev)
}

func sliceDataPointer(data []byte) unsafe.Pointer {
	if len(data) == 0 {
		return nil
	}
	return unsafe.Pointer(&data[0])
}

func boolResultToError(ok bool, err error) error {
	if ok {
		return nil
	}
	return err
}

// resultToError converts a C ZT_ResultCode to a Go error.
func resultToError(rc C.enum_ZT_ResultCode) error {
	code := ResultCode(rc)
	if code == ResultOK || code == ResultOKIgnored {
		return nil
	}
	return code
}
