package singbox

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

func testTracker() *ConnTracker {
	return NewConnTracker([]UserRef{{
		ID:    1,
		Nodes: []NodeRef{{ID: 7, Protocol: "vless", Port: 443}},
	}})
}

func testMetadata(user, tag, ip string) adapter.InboundContext {
	meta := adapter.InboundContext{User: user, Inbound: tag}
	if ip != "" {
		meta.Source = M.Socksaddr{Addr: netip.MustParseAddr(ip), Port: 4321}
	}
	return meta
}

func findDevice(devices []Device, pair Pair) *Device {
	for i := range devices {
		if devices[i].UserID == pair.UserID && devices[i].NodeID == pair.NodeID {
			return &devices[i]
		}
	}
	return nil
}

func hasIP(device *Device, ip string) bool {
	if device == nil {
		return false
	}
	for _, candidate := range device.IPs {
		if candidate == ip {
			return true
		}
	}
	return false
}

func TestTrackerCountsTCPBytesAndDevices(t *testing.T) {
	tracker := testTracker()
	client, server := net.Pipe()
	defer server.Close()

	wrapped := tracker.RoutedConnection(context.Background(), client, testMetadata("u-1", "vless-7", "203.0.113.9"), nil, nil)
	tracked, ok := wrapped.(*trackedConn)
	if !ok {
		t.Fatal("known user/node connection must be tracked")
	}

	payload := []byte("hello")

	serverRead := make(chan error, 1)
	go func() {
		buffer := make([]byte, len(payload))
		_, err := io.ReadFull(server, buffer)
		serverRead <- err
	}()
	if _, err := tracked.Write(payload); err != nil {
		t.Fatalf("tracked write: %v", err)
	}
	if err := <-serverRead; err != nil {
		t.Fatalf("server read: %v", err)
	}

	serverWrite := make(chan error, 1)
	go func() {
		_, err := server.Write(payload)
		serverWrite <- err
	}()
	inbound := make([]byte, len(payload))
	if _, err := io.ReadFull(tracked, inbound); err != nil {
		t.Fatalf("tracked read: %v", err)
	}
	if err := <-serverWrite; err != nil {
		t.Fatalf("server write: %v", err)
	}

	snapshot := tracker.Snapshot()
	traffic := snapshot.Traffic[Pair{UserID: 1, NodeID: 7}]
	if traffic.Upload != int64(len(payload)) {
		t.Fatalf("read from inbound must be upload, got %d", traffic.Upload)
	}
	if traffic.Download != int64(len(payload)) {
		t.Fatalf("write to inbound must be download, got %d", traffic.Download)
	}
	device := findDevice(snapshot.Devices, Pair{UserID: 1, NodeID: 7})
	if !hasIP(device, "203.0.113.9") {
		t.Fatalf("source ip must be tracked, got %+v", snapshot.Devices)
	}
	if device.Online != 1 {
		t.Fatalf("expected 1 online connection, got %d", device.Online)
	}

	if err := tracked.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	after := tracker.Snapshot()
	if len(after.Devices) != 0 {
		t.Fatalf("closed connection must leave no live state, got %+v", after.Devices)
	}
	if after.Traffic[Pair{UserID: 1, NodeID: 7}].Upload != int64(len(payload)) {
		t.Fatalf("cumulative traffic must survive close, got %+v", after.Traffic)
	}
}

func TestTrackerAggregatesDevicesByIP(t *testing.T) {
	tracker := testTracker()
	firstClient, firstServer := net.Pipe()
	defer firstServer.Close()
	secondClient, secondServer := net.Pipe()
	defer secondServer.Close()

	first := tracker.RoutedConnection(context.Background(), firstClient, testMetadata("u-1", "vless-7", "198.51.100.4"), nil, nil).(*trackedConn)
	second := tracker.RoutedConnection(context.Background(), secondClient, testMetadata("u-1", "vless-7", "198.51.100.4"), nil, nil).(*trackedConn)

	snapshot := tracker.Snapshot()
	if len(snapshot.Devices) != 1 {
		t.Fatalf("connections sharing user/node/ip must aggregate into one device, got %+v", snapshot.Devices)
	}
	if snapshot.Devices[0].Online != 2 || len(snapshot.Devices[0].IPs) != 1 {
		t.Fatalf("expected 2 online connections on one ip, got %+v", snapshot.Devices[0])
	}

	if err := first.Close(); err != nil {
		t.Fatalf("close first: %v", err)
	}
	snapshot = tracker.Snapshot()
	if len(snapshot.Devices) != 1 || snapshot.Devices[0].Online != 1 {
		t.Fatalf("unexpected state after one close: %+v", snapshot.Devices)
	}
	_ = second
}

func TestTrackerEnforcesDeviceLimit(t *testing.T) {
	tracker := NewConnTracker([]UserRef{{
		ID:          1,
		DeviceLimit: 1,
		Nodes:       []NodeRef{{ID: 7, Protocol: "vless", Port: 443}},
	}})

	first, firstServer := net.Pipe()
	defer firstServer.Close()
	if _, ok := tracker.RoutedConnection(context.Background(), first, testMetadata("u-1", "vless-7", "10.0.0.1"), nil, nil).(*trackedConn); !ok {
		t.Fatal("first device must be admitted")
	}

	sameIP, sameServer := net.Pipe()
	defer sameServer.Close()
	if _, ok := tracker.RoutedConnection(context.Background(), sameIP, testMetadata("u-1", "vless-7", "10.0.0.1"), nil, nil).(*trackedConn); !ok {
		t.Fatal("existing device ip must not be counted twice against the limit")
	}

	secondIP, secondServer := net.Pipe()
	defer secondServer.Close()
	if _, ok := tracker.RoutedConnection(context.Background(), secondIP, testMetadata("u-1", "vless-7", "10.0.0.2"), nil, nil).(*trackedConn); ok {
		t.Fatal("new device ip over the limit must be denied")
	}
}

func TestTrackerSkipsUnknownAttribution(t *testing.T) {
	tracker := testTracker()

	unknownUser, closeUser := net.Pipe()
	defer closeUser.Close()
	if _, ok := tracker.RoutedConnection(context.Background(), unknownUser, testMetadata("u-99", "vless-7", "1.2.3.4"), nil, nil).(*trackedConn); ok {
		t.Fatal("unknown user must not be tracked")
	}

	unknownNode, closeNode := net.Pipe()
	defer closeNode.Close()
	if _, ok := tracker.RoutedConnection(context.Background(), unknownNode, testMetadata("u-1", "vless-8", "1.2.3.4"), nil, nil).(*trackedConn); ok {
		t.Fatal("unknown node must not be tracked")
	}

	snapshot := tracker.Snapshot()
	if len(snapshot.Traffic) != 0 || len(snapshot.Devices) != 0 {
		t.Fatalf("unattributed connections must not be reported, got %+v", snapshot)
	}
}

func TestTrackerCountsPacketBytes(t *testing.T) {
	tracker := testTracker()
	fake := &fakePacketConn{next: []byte("up")}
	wrapped := tracker.RoutedPacketConnection(context.Background(), fake, testMetadata("u-1", "vless-7", "203.0.113.9"), nil, nil)
	packet, ok := wrapped.(*trackedPacketConn)
	if !ok {
		t.Fatal("known user/node packet connection must be tracked")
	}

	buffer := buf.New()
	if _, err := packet.ReadPacket(buffer); err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if string(buffer.Bytes()) != "up" {
		t.Fatalf("unexpected packet payload %q", buffer.Bytes())
	}
	buffer.Release()

	if err := packet.WritePacket(buf.As([]byte("down")), M.Socksaddr{}); err != nil {
		t.Fatalf("write packet: %v", err)
	}

	snapshot := tracker.Snapshot()
	traffic := snapshot.Traffic[Pair{UserID: 1, NodeID: 7}]
	if traffic.Upload != 2 || traffic.Download != 4 {
		t.Fatalf("unexpected packet traffic %+v", traffic)
	}
	if err := packet.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if after := tracker.Snapshot(); len(after.Devices) != 0 {
		t.Fatalf("closed packet connection must be released, got %+v", after.Devices)
	}
}

func TestTrackerRecordsVisitDestination(t *testing.T) {
	tracker := testTracker()
	client, server := net.Pipe()
	defer server.Close()

	meta := testMetadata("u-1", "vless-7", "203.0.113.9")
	meta.Destination = M.Socksaddr{Fqdn: "  Example.COM  ", Port: 8443}
	meta.Network = "tcp"
	wrapped := tracker.RoutedConnection(context.Background(), client, meta, nil, nil)
	if _, ok := wrapped.(*trackedConn); !ok {
		t.Fatal("known connection must be tracked")
	}

	visits := tracker.drainVisits()
	if len(visits) != 1 {
		t.Fatalf("expected 1 visit, got %d", len(visits))
	}
	v := visits[0]
	if v.UserID != 1 || v.NodeID != 7 {
		t.Fatalf("unexpected attribution %+v", v)
	}
	if v.DestHost != "example.com" || v.DestPort != 8443 || v.Network != "tcp" || v.ClientIP != "203.0.113.9" {
		t.Fatalf("unexpected visit %+v", v)
	}
	if tracker.drainVisits() != nil {
		t.Fatal("drain must clear the queue")
	}
}

func TestTrackerVisitFallsBackToDestinationAddr(t *testing.T) {
	tracker := testTracker()
	client, server := net.Pipe()
	defer server.Close()

	meta := testMetadata("u-1", "vless-7", "203.0.113.9")
	meta.Destination = M.Socksaddr{Addr: netip.MustParseAddr("::ffff:1.2.3.4"), Port: 53}
	meta.Network = "udp"
	tracker.RoutedConnection(context.Background(), client, meta, nil, nil)

	visits := tracker.drainVisits()
	if len(visits) != 1 || visits[0].DestHost != "1.2.3.4" || visits[0].Network != "udp" {
		t.Fatalf("expected mapped address fallback, got %+v", visits)
	}
}

func TestTrackerSkipsEmptyDestination(t *testing.T) {
	tracker := testTracker()
	client, server := net.Pipe()
	defer server.Close()
	tracker.RoutedConnection(context.Background(), client, testMetadata("u-1", "vless-7", "203.0.113.9"), nil, nil)
	if visits := tracker.drainVisits(); len(visits) != 0 {
		t.Fatalf("empty destination must be skipped, got %+v", visits)
	}
}

func TestTrackerVisitHostTruncated(t *testing.T) {
	tracker := testTracker()
	client, server := net.Pipe()
	defer server.Close()

	meta := testMetadata("u-1", "vless-7", "203.0.113.9")
	meta.Destination = M.Socksaddr{Fqdn: strings.Repeat("a", 300) + ".example.com", Port: 443}
	tracker.RoutedConnection(context.Background(), client, meta, nil, nil)

	visits := tracker.drainVisits()
	if len(visits) != 1 || len(visits[0].DestHost) != maxVisitHost {
		t.Fatalf("host must be truncated to %d, got %+v", maxVisitHost, visits)
	}
}

func TestTrackerVisitQueueDropsOldest(t *testing.T) {
	tracker := testTracker()
	for i := 0; i < maxVisits+5; i++ {
		client, server := net.Pipe()
		meta := testMetadata("u-1", "vless-7", "203.0.113.9")
		meta.Destination = M.Socksaddr{Fqdn: fmt.Sprintf("host-%d.example.com", i), Port: 443}
		tracker.RoutedConnection(context.Background(), client, meta, nil, nil)
		_ = server.Close()
	}

	visits := tracker.drainVisits()
	if len(visits) != maxVisits {
		t.Fatalf("queue must be bounded to %d, got %d", maxVisits, len(visits))
	}
	if visits[0].DestHost != "host-5.example.com" {
		t.Fatalf("oldest visits must be dropped, first is %q", visits[0].DestHost)
	}
	if dropped := tracker.droppedVisits(); dropped != 5 {
		t.Fatalf("expected 5 dropped visits, got %d", dropped)
	}
}

type fakePacketConn struct {
	next []byte
	sent []byte
}

func (f *fakePacketConn) ReadPacket(buffer *buf.Buffer) (M.Socksaddr, error) {
	if f.next == nil {
		return M.Socksaddr{}, io.EOF
	}
	if _, err := buffer.Write(f.next); err != nil {
		return M.Socksaddr{}, err
	}
	f.next = nil
	return M.Socksaddr{}, nil
}

func (f *fakePacketConn) WritePacket(buffer *buf.Buffer, _ M.Socksaddr) error {
	f.sent = append(f.sent, buffer.Bytes()...)
	buffer.Release()
	return nil
}

func (f *fakePacketConn) Close() error                     { return nil }
func (f *fakePacketConn) LocalAddr() net.Addr              { return nil }
func (f *fakePacketConn) SetDeadline(time.Time) error      { return nil }
func (f *fakePacketConn) SetReadDeadline(time.Time) error  { return nil }
func (f *fakePacketConn) SetWriteDeadline(time.Time) error { return nil }

var _ N.PacketConn = (*fakePacketConn)(nil)
