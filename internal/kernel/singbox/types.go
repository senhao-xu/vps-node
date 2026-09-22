package singbox

import "time"

type NodeRef struct {
	ID       int64
	Protocol string
	Port     int
}

type UserRef struct {
	ID          int64
	DeviceLimit int64
	Nodes       []NodeRef
}

type Pair struct {
	UserID int64
	NodeID int64
}

type Traffic struct {
	Upload   int64
	Download int64
}

type Device struct {
	UserID int64
	NodeID int64
	IPs    []string
	Online int
}

type Snapshot struct {
	Traffic map[Pair]Traffic
	Devices []Device
}

type Visit struct {
	UserID   int64
	NodeID   int64
	DestHost string
	DestPort int
	Network  string
	ClientIP string
	At       time.Time
}
