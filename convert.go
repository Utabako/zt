package zt

/*
#include <stdbool.h>
#include <string.h>
#include <sys/socket.h>
#include <netinet/in.h>
#include "ZeroTierOne.h"

// Helper to access ss_family portably (Android bionic uses anonymous union/struct)
static inline sa_family_t zt_ss_family(const struct sockaddr_storage *sa) { return sa->ss_family; }
static inline void zt_set_ss_family(struct sockaddr_storage *sa, sa_family_t f) { sa->ss_family = f; }
*/
import "C"

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"unsafe"
)

// sockaddrToAddrPort converts a C sockaddr_storage to Go netip.AddrPort.
func sockaddrToAddrPort(sa *C.struct_sockaddr_storage) (netip.AddrPort, bool) {
	if sa == nil {
		return netip.AddrPort{}, false
	}
	switch C.zt_ss_family(sa) {
	case C.AF_INET:
		sin := (*C.struct_sockaddr_in)(unsafe.Pointer(sa))
		var ip [4]byte
		copy(ip[:], (*[4]byte)(unsafe.Pointer(&sin.sin_addr))[:])
		var portBytes [2]byte
		copy(portBytes[:], (*[2]byte)(unsafe.Pointer(&sin.sin_port))[:])
		port := binary.BigEndian.Uint16(portBytes[:])
		return netip.AddrPortFrom(netip.AddrFrom4(ip), port), true
	case C.AF_INET6:
		sin6 := (*C.struct_sockaddr_in6)(unsafe.Pointer(sa))
		var ip [16]byte
		copy(ip[:], (*[16]byte)(unsafe.Pointer(&sin6.sin6_addr))[:])
		var portBytes [2]byte
		copy(portBytes[:], (*[2]byte)(unsafe.Pointer(&sin6.sin6_port))[:])
		port := binary.BigEndian.Uint16(portBytes[:])
		addr := netip.AddrFrom16(ip)
		if zone := zoneForScopeID(uint32(sin6.sin6_scope_id)); zone != "" {
			addr = addr.WithZone(zone)
		}
		return netip.AddrPortFrom(addr, port), true
	default:
		return netip.AddrPort{}, false
	}
}

// addrPortToSockaddr converts a Go netip.AddrPort to a C sockaddr_storage.
func addrPortToSockaddr(ap netip.AddrPort) (sa C.struct_sockaddr_storage, err error) {
	if !ap.IsValid() {
		return sa, fmt.Errorf("zt: invalid address %v", ap)
	}

	C.memset(unsafe.Pointer(&sa), 0, C.sizeof_struct_sockaddr_storage)
	ap = normalizeAddrPort(ap)
	addr := ap.Addr()
	if addr.Is4() {
		sin := (*C.struct_sockaddr_in)(unsafe.Pointer(&sa))
		sin.sin_family = C.AF_INET
		ip4 := addr.As4()
		copy((*[4]byte)(unsafe.Pointer(&sin.sin_addr))[:], ip4[:])
		var portBytes [2]byte
		binary.BigEndian.PutUint16(portBytes[:], ap.Port())
		copy((*[2]byte)(unsafe.Pointer(&sin.sin_port))[:], portBytes[:])
		return sa, nil
	}

	scopeID, err := scopeIDForZone(addr.Zone())
	if err != nil {
		return sa, err
	}
	sin6 := (*C.struct_sockaddr_in6)(unsafe.Pointer(&sa))
	sin6.sin6_family = C.AF_INET6
	sin6.sin6_scope_id = C.uint32_t(scopeID)
	ip6 := addr.WithZone("").As16()
	copy((*[16]byte)(unsafe.Pointer(&sin6.sin6_addr))[:], ip6[:])
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], ap.Port())
	copy((*[2]byte)(unsafe.Pointer(&sin6.sin6_port))[:], portBytes[:])
	return sa, nil
}

func scopeIDForZone(zone string) (uint32, error) {
	if zone == "" {
		return 0, nil
	}
	if scopeID, err := strconv.ParseUint(zone, 10, 32); err == nil {
		return uint32(scopeID), nil
	}
	iface, err := net.InterfaceByName(zone)
	if err != nil {
		return 0, fmt.Errorf("zt: resolve IPv6 zone %q: %w", zone, err)
	}
	if iface.Index <= 0 {
		return 0, fmt.Errorf("zt: IPv6 zone %q has invalid interface index %d", zone, iface.Index)
	}
	return uint32(iface.Index), nil
}

func zoneForScopeID(scopeID uint32) string {
	if scopeID == 0 {
		return ""
	}
	return strconv.FormatUint(uint64(scopeID), 10)
}

// sockaddrToPrefix converts a C sockaddr_storage to a Go netip.Prefix.
// ZeroTier stores the prefix length in the port field.
func sockaddrToPrefix(sa *C.struct_sockaddr_storage) (netip.Prefix, bool) {
	if sa == nil {
		return netip.Prefix{}, false
	}
	switch C.zt_ss_family(sa) {
	case C.AF_INET:
		sin := (*C.struct_sockaddr_in)(unsafe.Pointer(sa))
		var ip [4]byte
		copy(ip[:], (*[4]byte)(unsafe.Pointer(&sin.sin_addr))[:])
		var portBytes [2]byte
		copy(portBytes[:], (*[2]byte)(unsafe.Pointer(&sin.sin_port))[:])
		bits := int(binary.BigEndian.Uint16(portBytes[:]))
		return prefixFromAddrBits(netip.AddrFrom4(ip), bits)
	case C.AF_INET6:
		sin6 := (*C.struct_sockaddr_in6)(unsafe.Pointer(sa))
		var ip [16]byte
		copy(ip[:], (*[16]byte)(unsafe.Pointer(&sin6.sin6_addr))[:])
		var portBytes [2]byte
		copy(portBytes[:], (*[2]byte)(unsafe.Pointer(&sin6.sin6_port))[:])
		bits := int(binary.BigEndian.Uint16(portBytes[:]))
		return prefixFromAddrBits(netip.AddrFrom16(ip), bits)
	default:
		return netip.Prefix{}, false
	}
}

// sockaddrToAddr extracts only the IP address from a sockaddr_storage.
func sockaddrToAddr(sa *C.struct_sockaddr_storage) (netip.Addr, bool) {
	if sa == nil {
		return netip.Addr{}, false
	}
	family := C.zt_ss_family(sa)
	if family == 0 {
		return netip.Addr{}, false
	}
	switch family {
	case C.AF_INET:
		sin := (*C.struct_sockaddr_in)(unsafe.Pointer(sa))
		var ip [4]byte
		copy(ip[:], (*[4]byte)(unsafe.Pointer(&sin.sin_addr))[:])
		return netip.AddrFrom4(ip), true
	case C.AF_INET6:
		sin6 := (*C.struct_sockaddr_in6)(unsafe.Pointer(sa))
		var ip [16]byte
		copy(ip[:], (*[16]byte)(unsafe.Pointer(&sin6.sin6_addr))[:])
		return netip.AddrFrom16(ip), true
	default:
		return netip.Addr{}, false
	}
}

func normalizeAddrPort(ap netip.AddrPort) netip.AddrPort {
	return netip.AddrPortFrom(ap.Addr().Unmap(), ap.Port())
}

func prefixFromAddrBits(addr netip.Addr, bits int) (netip.Prefix, bool) {
	p := netip.PrefixFrom(addr, bits)
	return p, p.IsValid()
}

// convertNodeStatus converts a C ZT_NodeStatus to a Go NodeStatus.
func convertNodeStatus(cs *C.ZT_NodeStatus) NodeStatus {
	return NodeStatus{
		Address:        uint64(cs.address),
		PublicIdentity: C.GoString(cs.publicIdentity),
		SecretIdentity: C.GoString(cs.secretIdentity),
		Online:         cs.online != 0,
	}
}

// convertPeerPhysicalPath converts a C ZT_PeerPhysicalPath to Go.
func convertPeerPhysicalPath(cp *C.ZT_PeerPhysicalPath) PeerPhysicalPath {
	addr, _ := sockaddrToAddrPort(&cp.address)
	return PeerPhysicalPath{
		Address:           addr,
		LastSend:          uint64(cp.lastSend),
		LastReceive:       uint64(cp.lastReceive),
		TrustedPathID:     uint64(cp.trustedPathId),
		LatencyMean:       float32(cp.latencyMean),
		LatencyMax:        float32(cp.latencyMax),
		LatencyVariance:   float32(cp.latencyVariance),
		PacketLossRatio:   float32(cp.packetLossRatio),
		PacketErrorRatio:  float32(cp.packetErrorRatio),
		AssignedFlowCount: uint16(cp.assignedFlowCount),
		Scope:             uint8(cp.scope),
		RelativeQuality:   float32(cp.relativeQuality),
		InterfaceName:     C.GoString(&cp.ifname[0]),
		LocalSocket:       uint64(cp.localSocket),
		LocalPort:         uint16(cp.localPort),
		Expired:           cp.expired != 0,
		Bonded:            cp.bonded != 0,
		Eligible:          cp.eligible != 0,
		LinkSpeed:         uint32(cp.linkSpeed),
		Preferred:         cp.preferred != 0,
	}
}

// convertPeer converts a C ZT_Peer to a Go Peer.
func convertPeer(cp *C.ZT_Peer) Peer {
	pathCount := int(cp.pathCount)
	paths := make([]PeerPhysicalPath, pathCount)
	for i := 0; i < pathCount; i++ {
		paths[i] = convertPeerPhysicalPath(&cp.paths[i])
	}
	return Peer{
		Address:        uint64(cp.address),
		VersionMajor:   int(cp.versionMajor),
		VersionMinor:   int(cp.versionMinor),
		VersionRev:     int(cp.versionRev),
		Latency:        int(cp.latency),
		Role:           PeerRole(cp.role),
		IsBonded:       bool(cp.isBonded),
		BondingPolicy:  int(cp.bondingPolicy),
		NumAliveLinks:  int(cp.numAliveLinks),
		NumTotalLinks:  int(cp.numTotalLinks),
		CustomBondName: C.GoString(&cp.customBondName[0]),
		Paths:          paths,
	}
}

// convertVirtualNetworkConfig converts a C ZT_VirtualNetworkConfig to Go.
func convertVirtualNetworkConfig(cc *C.ZT_VirtualNetworkConfig) *VirtualNetworkConfig {
	config := &VirtualNetworkConfig{
		NetworkID:        uint64(cc.nwid),
		MAC:              uint64(cc.mac),
		Name:             C.GoString(&cc.name[0]),
		Status:           VirtualNetworkStatus(cc.status),
		Type:             VirtualNetworkType(cc._type),
		MTU:              uint(cc.mtu),
		DHCP:             cc.dhcp != 0,
		Bridge:           cc.bridge != 0,
		BroadcastEnabled: cc.broadcastEnabled != 0,
		PortError:        int(cc.portError),
		NetconfRevision:  uint64(cc.netconfRevision),
		SSOEnabled:       bool(cc.ssoEnabled),
		SSOVersion:       uint64(cc.ssoVersion),
	}

	// Assigned addresses
	addrCount := int(cc.assignedAddressCount)
	config.AssignedAddresses = make([]netip.Prefix, 0, addrCount)
	for i := 0; i < addrCount; i++ {
		if prefix, ok := sockaddrToPrefix(&cc.assignedAddresses[i]); ok {
			config.AssignedAddresses = append(config.AssignedAddresses, prefix)
		}
	}

	// Routes
	routeCount := int(cc.routeCount)
	config.Routes = make([]Route, 0, routeCount)
	for i := 0; i < routeCount; i++ {
		cr := &cc.routes[i]
		target, ok := sockaddrToPrefix(&cr.target)
		if !ok {
			continue
		}
		via, _ := sockaddrToAddr(&cr.via)
		config.Routes = append(config.Routes, Route{
			Target: target,
			Via:    via,
			Flags:  uint16(cr.flags),
			Metric: uint16(cr.metric),
		})
	}

	// Multicast subscriptions
	mcastCount := int(cc.multicastSubscriptionCount)
	config.MulticastSubscriptions = make([]MulticastGroup, mcastCount)
	for i := 0; i < mcastCount; i++ {
		config.MulticastSubscriptions[i] = MulticastGroup{
			MAC: uint64(cc.multicastSubscriptions[i].mac),
			ADI: uint32(cc.multicastSubscriptions[i].adi),
		}
	}

	// DNS
	config.DNS.Domain = C.GoString(&cc.dns.domain[0])
	config.DNS.Servers = make([]netip.Addr, 0, MaxDNSServers)
	for i := 0; i < MaxDNSServers; i++ {
		if addr, ok := sockaddrToAddr(&cc.dns.server_addr[i]); ok {
			config.DNS.Servers = append(config.DNS.Servers, addr)
		}
	}

	// SSO
	if config.SSOEnabled {
		config.AuthenticationURL = C.GoString(&cc.authenticationURL[0])
		config.AuthenticationExpiryTime = uint64(cc.authenticationExpiryTime)
		config.IssuerURL = C.GoString(&cc.issuerURL[0])
		config.CentralAuthURL = C.GoString(&cc.centralAuthURL[0])
		config.SSONonce = C.GoString(&cc.ssoNonce[0])
		config.SSOState = C.GoString(&cc.ssoState[0])
		config.SSOClientID = C.GoString(&cc.ssoClientID[0])
		config.SSOProvider = C.GoString(&cc.ssoProvider[0])
	}

	return config
}
