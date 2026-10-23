package zt

import (
	"net"
	"net/netip"
	"strconv"
	"testing"
)

func TestSockaddrRoundTripUnmapsIPv4InIPv6(t *testing.T) {
	mapped := netip.MustParseAddrPort("[::ffff:192.0.2.1]:9993")
	sa, err := addrPortToSockaddr(mapped)
	if err != nil {
		t.Fatalf("addrPortToSockaddr() error = %v", err)
	}
	got, ok := sockaddrToAddrPort(&sa)
	if want := netip.MustParseAddrPort("192.0.2.1:9993"); !ok || got != want {
		t.Fatalf("sockaddrToAddrPort() = (%v, %v), want (%v, true)", got, ok, want)
	}
}

func TestSockaddrToPrefix(t *testing.T) {
	tests := []struct {
		name    string
		address string
		want    string
	}{
		{"IPv4", "192.0.2.1:24", "192.0.2.1/24"},
		{"IPv6", "[2001:db8::1]:64", "2001:db8::1/64"},
		{"invalid IPv4 prefix", "192.0.2.1:33", ""},
		{"invalid IPv6 prefix", "[2001:db8::1]:129", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sa, err := addrPortToSockaddr(netip.MustParseAddrPort(tt.address))
			if err != nil {
				t.Fatalf("addrPortToSockaddr() error = %v", err)
			}
			got, ok := sockaddrToPrefix(&sa)
			if ok != (tt.want != "") || (ok && got.String() != tt.want) {
				t.Fatalf("sockaddrToPrefix() = (%v, %v), want %q", got, ok, tt.want)
			}
		})
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
