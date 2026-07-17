package zt

import (
	"net"
	"net/netip"
	"strconv"
	"testing"
)

func TestNormalizeAddrPortUnmapsIPv4InIPv6(t *testing.T) {
	mapped := netip.MustParseAddrPort("[::ffff:192.0.2.1]:9993")

	if got, want := normalizeAddrPort(mapped), netip.MustParseAddrPort("192.0.2.1:9993"); got != want {
		t.Fatalf("normalizeAddrPort() = %v, want %v", got, want)
	}
}

func TestPrefixFromAddrBitsRejectsInvalidIPv4PrefixLength(t *testing.T) {
	if _, ok := prefixFromAddrBits(netip.MustParseAddr("192.0.2.0"), 33); ok {
		t.Fatal("prefixFromAddrBits() accepted invalid IPv4 prefix length")
	}
}

func TestPrefixFromAddrBitsRejectsInvalidIPv6PrefixLength(t *testing.T) {
	if _, ok := prefixFromAddrBits(netip.MustParseAddr("2001:db8::"), 129); ok {
		t.Fatal("prefixFromAddrBits() accepted invalid IPv6 prefix length")
	}
}

func TestPrefixFromAddrBitsReturnsValidPrefix(t *testing.T) {
	if got, ok := prefixFromAddrBits(netip.MustParseAddr("2001:db8::"), 64); !ok || got != netip.MustParsePrefix("2001:db8::/64") {
		t.Fatalf("prefixFromAddrBits() = (%v, %v), want (%v, true)", got, ok, netip.MustParsePrefix("2001:db8::/64"))
	}
}

func TestSockaddrRoundTripPreservesIPv6Scope(t *testing.T) {
	want := netip.MustParseAddrPort("[fe80::1%42]:9993")
	sa, err := addrPortToSockaddr(want)
	if err != nil {
		t.Fatalf("addrPortToSockaddr() error = %v", err)
	}
	got, ok := sockaddrToAddrPort(&sa)
	if !ok || got != want {
		t.Fatalf("sockaddrToAddrPort() = (%v, %v), want (%v, true)", got, ok, want)
	}
}

func TestSockaddrRoundTripResolvesNamedIPv6Zone(t *testing.T) {
	interfaces, err := net.Interfaces()
	if err != nil {
		t.Fatalf("net.Interfaces() error = %v", err)
	}
	var iface net.Interface
	found := false
	for _, candidate := range interfaces {
		if candidate.Index > 0 && candidate.Name != "" {
			iface = candidate
			found = true
			break
		}
	}
	if !found {
		t.Skip("no network interface with a valid index")
	}

	addr := netip.MustParseAddr("fe80::1").WithZone(iface.Name)
	want := netip.AddrPortFrom(addr, 9993)
	sa, err := addrPortToSockaddr(want)
	if err != nil {
		t.Fatalf("addrPortToSockaddr() error = %v", err)
	}
	got, ok := sockaddrToAddrPort(&sa)
	if !ok {
		t.Fatal("sockaddrToAddrPort() rejected a valid IPv6 sockaddr")
	}
	if got.Addr().Zone() != strconv.Itoa(iface.Index) {
		t.Fatalf("round-trip zone = %q, want interface index %d", got.Addr().Zone(), iface.Index)
	}
	if got.Addr().WithZone("") != addr.WithZone("") || got.Port() != want.Port() {
		t.Fatalf("round-trip address = %v, want %v", got, want)
	}
}

func TestProcessWirePacketRejectsUnknownIPv6Zone(t *testing.T) {
	node := newTestNode(t)
	defer node.Close()
	addr := netip.AddrPortFrom(
		netip.MustParseAddr("fe80::1").WithZone("go-zt-invalid-zone-that-cannot-exist"),
		9993,
	)

	if err := node.ProcessWirePacket(0, addr, nil, nil); err == nil {
		t.Fatal("ProcessWirePacket() accepted an unknown IPv6 zone")
	}
}
