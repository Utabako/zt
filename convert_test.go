package zt

import (
	"net/netip"
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
