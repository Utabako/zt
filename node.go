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

// Node represents a ZeroTier node instance.
type Node struct {
	cNode     unsafe.Pointer // *C.ZT_Node (typedef void)
	handle    cgo.Handle
	callbacks NodeCallbacks
	mu        sync.Mutex
}

// NewNode creates a new ZeroTier node with the given callbacks and optional config.
// If config is nil, default configuration is used.
func NewNode(callbacks NodeCallbacks, config *NodeConfig) (*Node, error) {
	n := &Node{callbacks: callbacks}
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
	rc := C.ZT_Node_new(
		&cNode,
		&cConfig,
		unsafe.Pointer(uintptr(n.handle)),
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

// Close shuts down and frees the ZeroTier node.
func (n *Node) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.cNode != nil {
		C.ZT_Node_delete(n.cNode)
		n.cNode = nil
	}
	if n.handle != 0 {
		n.handle.Delete()
		n.handle = 0
	}
}

func (n *Node) mustCNode() unsafe.Pointer {
	if n.cNode == nil {
		panic("zt: use of closed Node")
	}
	return n.cNode
}

// Address returns this node's 40-bit ZeroTier address.
func (n *Node) Address() uint64 {
	n.mu.Lock()
	defer n.mu.Unlock()
	return uint64(C.ZT_Node_address(n.mustCNode()))
}

// Status returns the current status of this node.
func (n *Node) Status() NodeStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	var cs C.ZT_NodeStatus
	C.ZT_Node_status(n.mustCNode(), &cs)
	return convertNodeStatus(&cs)
}

// ProcessWirePacket processes a packet received from the physical wire.
func (n *Node) ProcessWirePacket(localSocket int64, remoteAddress netip.AddrPort, data []byte, nextDeadline *int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	var sa C.struct_sockaddr_storage
	addrPortToSockaddr(remoteAddress, &sa)

	now := C.int64_t(time.Now().UnixMilli())
	var deadline C.int64_t

	rc := C.ZT_Node_processWirePacket(
		n.mustCNode(),
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
	n.mu.Lock()
	defer n.mu.Unlock()

	now := C.int64_t(time.Now().UnixMilli())
	var deadline C.int64_t

	rc := C.ZT_Node_processVirtualNetworkFrame(
		n.mustCNode(),
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
	n.mu.Lock()
	defer n.mu.Unlock()

	now := C.int64_t(time.Now().UnixMilli())
	var deadline C.int64_t

	rc := C.ZT_Node_processBackgroundTasks(n.mustCNode(), nil, now, &deadline)

	if nextDeadline != nil {
		*nextDeadline = int64(deadline)
	}

	return resultToError(rc)
}

// Join joins a virtual network.
func (n *Node) Join(nwid uint64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rc := C.ZT_Node_join(n.mustCNode(), C.uint64_t(nwid), nil, nil)
	return resultToError(rc)
}

// Leave leaves a virtual network.
func (n *Node) Leave(nwid uint64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rc := C.ZT_Node_leave(n.mustCNode(), C.uint64_t(nwid), nil, nil)
	return resultToError(rc)
}

// MulticastSubscribe subscribes to an Ethernet multicast group.
func (n *Node) MulticastSubscribe(nwid uint64, multicastGroup uint64, multicastADI uint32) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rc := C.ZT_Node_multicastSubscribe(n.mustCNode(), nil, C.uint64_t(nwid), C.uint64_t(multicastGroup), C.ulong(multicastADI))
	return resultToError(rc)
}

// MulticastUnsubscribe unsubscribes from an Ethernet multicast group.
func (n *Node) MulticastUnsubscribe(nwid uint64, multicastGroup uint64, multicastADI uint32) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rc := C.ZT_Node_multicastUnsubscribe(n.mustCNode(), C.uint64_t(nwid), C.uint64_t(multicastGroup), C.ulong(multicastADI))
	return resultToError(rc)
}

// Orbit adds or updates a moon (federated root set).
func (n *Node) Orbit(moonWorldID uint64, moonSeed uint64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rc := C.ZT_Node_orbit(n.mustCNode(), nil, C.uint64_t(moonWorldID), C.uint64_t(moonSeed))
	return resultToError(rc)
}

// Deorbit removes a moon.
func (n *Node) Deorbit(moonWorldID uint64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	rc := C.ZT_Node_deorbit(n.mustCNode(), nil, C.uint64_t(moonWorldID))
	return resultToError(rc)
}

// Peers returns a list of all known peers.
func (n *Node) Peers() []Peer {
	n.mu.Lock()
	defer n.mu.Unlock()

	cNode := n.mustCNode()
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
	n.mu.Lock()
	defer n.mu.Unlock()

	cNode := n.mustCNode()
	cc := C.ZT_Node_networkConfig(cNode, C.uint64_t(nwid))
	if cc == nil {
		return nil
	}
	defer C.ZT_Node_freeQueryResult(cNode, unsafe.Pointer(cc))

	return convertVirtualNetworkConfig(cc)
}

// Networks returns a list of all joined networks.
func (n *Node) Networks() []VirtualNetworkConfig {
	n.mu.Lock()
	defer n.mu.Unlock()

	cNode := n.mustCNode()
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
	n.mu.Lock()
	defer n.mu.Unlock()

	var sa C.struct_sockaddr_storage
	addrPortToSockaddr(addr, &sa)
	return boolResultToError(C.ZT_Node_addLocalInterfaceAddress(n.mustCNode(), &sa) != 0, ErrLocalInterfaceAddressRejected)
}

// ClearLocalInterfaceAddresses clears all local interface addresses.
func (n *Node) ClearLocalInterfaceAddresses() {
	n.mu.Lock()
	defer n.mu.Unlock()
	C.ZT_Node_clearLocalInterfaceAddresses(n.mustCNode())
}

// SendUserMessage sends a VERB_USER_MESSAGE to another ZeroTier node.
// It returns ErrUserMessageRejected if the core rejects the message.
func (n *Node) SendUserMessage(dest uint64, typeID uint64, data []byte) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	dataPtr := sliceDataPointer(data)
	return boolResultToError(
		C.ZT_Node_sendUserMessage(n.mustCNode(), nil, C.uint64_t(dest), C.uint64_t(typeID), dataPtr, C.uint(len(data))) != 0,
		ErrUserMessageRejected,
	)
}

// SetPhysicalPathConfiguration sets configuration for a given physical path.
func (n *Node) SetPhysicalPathConfiguration(pathNetwork netip.AddrPort, trustedPathID uint64, mtu int) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	var sa C.struct_sockaddr_storage
	addrPortToSockaddr(pathNetwork, &sa)

	var config C.ZT_PhysicalPathConfiguration
	config.trustedPathId = C.uint64_t(trustedPathID)
	config.mtu = C.int(mtu)

	rc := C.ZT_Node_setPhysicalPathConfiguration(n.mustCNode(), &sa, &config)
	return resultToError(rc)
}

// SetNetconfMaster sets a network configuration master instance for this node.
// This is for nodes with compiled-in support for acting as network controllers.
// Pass nil to disable.
func (n *Node) SetNetconfMaster(instance unsafe.Pointer) {
	n.mu.Lock()
	defer n.mu.Unlock()
	C.ZT_Node_setNetconfMaster(n.mustCNode(), instance)
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
