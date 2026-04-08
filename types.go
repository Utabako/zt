package zt

import (
	"errors"
	"net/netip"
)

// NodeConfig contains node-wide configuration options.
type NodeConfig struct {
	// EnableEncryptedHello enables encrypted HELLO packets (increases CPU usage slightly).
	EnableEncryptedHello bool
	// LowBandwidthMode reduces keepalive and path sensing traffic.
	LowBandwidthMode bool
}

var (
	ErrLocalInterfaceAddressRejected = errors.New("local interface address rejected")
	ErrUserMessageRejected           = errors.New("user message rejected")
)

// --- Constants ---

const (
	DefaultPort    = 9993
	MinMTU         = 1280
	MaxMTU         = 10000
	DefaultPhysMTU = 1432

	MaxNetworkRoutes          = 128
	MaxAssignedAddresses      = 32
	MaxPeerNetworkPaths       = 64
	MaxMulticastSubscriptions = 4096
	MaxNetworkShortNameLen    = 127
	MaxDNSServers             = 4

	PathLinkQualityMax = 0xff
)

// --- Enums ---

// ResultCode represents the result of a ZeroTier API call.
type ResultCode int

const (
	ResultOK                        ResultCode = 0
	ResultOKIgnored                 ResultCode = 1
	ResultFatalErrorOutOfMemory     ResultCode = 100
	ResultFatalErrorDataStoreFailed ResultCode = 101
	ResultFatalErrorInternal        ResultCode = 102
	ResultErrorNetworkNotFound      ResultCode = 1000
	ResultErrorUnsupportedOperation ResultCode = 1001
	ResultErrorBadParameter         ResultCode = 1002
)

// IsFatal returns true if the result code indicates a fatal error.
func (r ResultCode) IsFatal() bool {
	return int(r) >= 100 && int(r) < 1000
}

func (r ResultCode) Error() string {
	switch r {
	case ResultOK:
		return "ok"
	case ResultOKIgnored:
		return "ok (ignored)"
	case ResultFatalErrorOutOfMemory:
		return "fatal: out of memory"
	case ResultFatalErrorDataStoreFailed:
		return "fatal: data store failed"
	case ResultFatalErrorInternal:
		return "fatal: internal error"
	case ResultErrorNetworkNotFound:
		return "error: network not found"
	case ResultErrorUnsupportedOperation:
		return "error: unsupported operation"
	case ResultErrorBadParameter:
		return "error: bad parameter"
	default:
		return "unknown error"
	}
}

// Event represents a ZeroTier event type.
type Event int

const (
	EventUp                          Event = 0
	EventOffline                     Event = 1
	EventOnline                      Event = 2
	EventDown                        Event = 3
	EventFatalErrorIdentityCollision Event = 4
	EventTrace                       Event = 5
	EventUserMessage                 Event = 6
	EventRemoteTrace                 Event = 7
)

func (e Event) String() string {
	switch e {
	case EventUp:
		return "UP"
	case EventOffline:
		return "OFFLINE"
	case EventOnline:
		return "ONLINE"
	case EventDown:
		return "DOWN"
	case EventFatalErrorIdentityCollision:
		return "FATAL_ERROR_IDENTITY_COLLISION"
	case EventTrace:
		return "TRACE"
	case EventUserMessage:
		return "USER_MESSAGE"
	case EventRemoteTrace:
		return "REMOTE_TRACE"
	default:
		return "UNKNOWN"
	}
}

// VirtualNetworkStatus represents the status of a virtual network.
type VirtualNetworkStatus int

const (
	NetworkStatusRequestingConfiguration VirtualNetworkStatus = 0
	NetworkStatusOK                      VirtualNetworkStatus = 1
	NetworkStatusAccessDenied            VirtualNetworkStatus = 2
	NetworkStatusNotFound                VirtualNetworkStatus = 3
	NetworkStatusPortError               VirtualNetworkStatus = 4
	NetworkStatusClientTooOld            VirtualNetworkStatus = 5
	NetworkStatusAuthenticationRequired  VirtualNetworkStatus = 6
)

func (s VirtualNetworkStatus) String() string {
	switch s {
	case NetworkStatusRequestingConfiguration:
		return "REQUESTING_CONFIGURATION"
	case NetworkStatusOK:
		return "OK"
	case NetworkStatusAccessDenied:
		return "ACCESS_DENIED"
	case NetworkStatusNotFound:
		return "NOT_FOUND"
	case NetworkStatusPortError:
		return "PORT_ERROR"
	case NetworkStatusClientTooOld:
		return "CLIENT_TOO_OLD"
	case NetworkStatusAuthenticationRequired:
		return "AUTHENTICATION_REQUIRED"
	default:
		return "UNKNOWN"
	}
}

// VirtualNetworkType represents the type of a virtual network.
type VirtualNetworkType int

const (
	NetworkTypePrivate VirtualNetworkType = 0
	NetworkTypePublic  VirtualNetworkType = 1
)

func (t VirtualNetworkType) String() string {
	switch t {
	case NetworkTypePrivate:
		return "PRIVATE"
	case NetworkTypePublic:
		return "PUBLIC"
	default:
		return "UNKNOWN"
	}
}

// VirtualNetworkConfigOperation represents a network config update type.
type VirtualNetworkConfigOperation int

const (
	VirtualNetworkConfigOperationUp           VirtualNetworkConfigOperation = 1
	VirtualNetworkConfigOperationConfigUpdate VirtualNetworkConfigOperation = 2
	VirtualNetworkConfigOperationDown         VirtualNetworkConfigOperation = 3
	VirtualNetworkConfigOperationDestroy      VirtualNetworkConfigOperation = 4
)

func (op VirtualNetworkConfigOperation) String() string {
	switch op {
	case VirtualNetworkConfigOperationUp:
		return "UP"
	case VirtualNetworkConfigOperationConfigUpdate:
		return "CONFIG_UPDATE"
	case VirtualNetworkConfigOperationDown:
		return "DOWN"
	case VirtualNetworkConfigOperationDestroy:
		return "DESTROY"
	default:
		return "UNKNOWN"
	}
}

// PeerRole represents the role of a peer in the trust hierarchy.
type PeerRole int

const (
	PeerRoleLeaf   PeerRole = 0
	PeerRoleMoon   PeerRole = 1
	PeerRolePlanet PeerRole = 2
)

func (r PeerRole) String() string {
	switch r {
	case PeerRoleLeaf:
		return "LEAF"
	case PeerRoleMoon:
		return "MOON"
	case PeerRolePlanet:
		return "PLANET"
	default:
		return "UNKNOWN"
	}
}

// StateObjectType represents the type of a persisted state object.
type StateObjectType int

const (
	StateObjectNull           StateObjectType = 0
	StateObjectIdentityPublic StateObjectType = 1
	StateObjectIdentitySecret StateObjectType = 2
	StateObjectPlanet         StateObjectType = 3
	StateObjectMoon           StateObjectType = 4
	StateObjectPeer           StateObjectType = 5
	StateObjectNetworkConfig  StateObjectType = 6
)

func (t StateObjectType) String() string {
	switch t {
	case StateObjectNull:
		return "NULL"
	case StateObjectIdentityPublic:
		return "IDENTITY_PUBLIC"
	case StateObjectIdentitySecret:
		return "IDENTITY_SECRET"
	case StateObjectPlanet:
		return "PLANET"
	case StateObjectMoon:
		return "MOON"
	case StateObjectPeer:
		return "PEER"
	case StateObjectNetworkConfig:
		return "NETWORK_CONFIG"
	default:
		return "UNKNOWN"
	}
}

// --- Struct Types ---

// NodeStatus represents the current status of a ZeroTier node.
type NodeStatus struct {
	Address        uint64
	PublicIdentity string
	SecretIdentity string
	Online         bool
}

// Route represents a route pushed on a virtual network.
type Route struct {
	Target netip.Prefix
	Via    netip.Addr // Zero value if LAN-local (no gateway)
	Flags  uint16
	Metric uint16
}

// VirtualNetworkDNS represents DNS configuration for a virtual network.
type VirtualNetworkDNS struct {
	Domain  string
	Servers []netip.Addr
}

// MulticastGroup represents an Ethernet multicast group.
type MulticastGroup struct {
	MAC uint64
	ADI uint32
}

// VirtualNetworkConfig represents the configuration of a virtual network.
type VirtualNetworkConfig struct {
	NetworkID              uint64
	MAC                    uint64
	Name                   string
	Status                 VirtualNetworkStatus
	Type                   VirtualNetworkType
	MTU                    uint
	DHCP                   bool
	Bridge                 bool
	BroadcastEnabled       bool
	PortError              int
	NetconfRevision        uint64
	AssignedAddresses      []netip.Prefix
	Routes                 []Route
	MulticastSubscriptions []MulticastGroup
	DNS                    VirtualNetworkDNS

	// SSO fields
	SSOEnabled               bool
	SSOVersion               uint64
	AuthenticationURL        string
	AuthenticationExpiryTime uint64
	IssuerURL                string
	CentralAuthURL           string
	SSONonce                 string
	SSOState                 string
	SSOClientID              string
	SSOProvider              string
}

// PeerPhysicalPath represents a physical network path to a peer.
type PeerPhysicalPath struct {
	Address           netip.AddrPort
	LastSend          uint64
	LastReceive       uint64
	TrustedPathID     uint64
	LatencyMean       float32
	LatencyMax        float32
	LatencyVariance   float32
	PacketLossRatio   float32
	PacketErrorRatio  float32
	AssignedFlowCount uint16
	Scope             uint8
	RelativeQuality   float32
	InterfaceName     string
	LocalSocket       uint64
	LocalPort         uint16
	Expired           bool
	Bonded            bool
	Eligible          bool
	LinkSpeed         uint32
	Preferred         bool
}

// Peer represents information about a known ZeroTier peer.
type Peer struct {
	Address        uint64
	VersionMajor   int
	VersionMinor   int
	VersionRev     int
	Latency        int
	Role           PeerRole
	IsBonded       bool
	BondingPolicy  int
	NumAliveLinks  int
	NumTotalLinks  int
	CustomBondName string
	Paths          []PeerPhysicalPath
}

// UserMessage represents a VERB_USER_MESSAGE payload.
type UserMessage struct {
	Origin uint64
	TypeID uint64
	Data   []byte
}

// RemoteTrace represents a VERB_REMOTE_TRACE payload.
type RemoteTrace struct {
	Origin uint64
	Data   string
}
