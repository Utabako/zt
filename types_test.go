package zt

import "testing"

func TestVirtualNetworkTypeString(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{name: "private", got: NetworkTypePrivate.String(), want: "PRIVATE"},
		{name: "public", got: NetworkTypePublic.String(), want: "PUBLIC"},
		{name: "unknown", got: VirtualNetworkType(99).String(), want: "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("String() = %q, want %q", tt.got, tt.want)
			}
		})
	}
}
