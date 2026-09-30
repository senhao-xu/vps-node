package singbox

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/sagernet/sing/common/buf"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/protocol/socks"
	"github.com/sagernet/sing/protocol/socks/socks5"
)

type handshakeLocalConn struct {
	net.Conn
}

func (c handshakeLocalConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 1234}
}

func handshakePipe(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	first, second := net.Pipe()
	t.Cleanup(func() { _ = first.Close(); _ = second.Close() })
	_ = first.SetDeadline(time.Now().Add(2 * time.Second))
	_ = second.SetDeadline(time.Now().Add(2 * time.Second))
	return handshakeLocalConn{first}, second
}

func checkTrackedSOCKSHandshake(t *testing.T, tracker *ConnTracker, peer net.Conn, fail bool, report func() error) {
	t.Helper()
	reported := make(chan error, 1)
	go func() { reported <- report() }()
	response, err := socks5.ReadResponse(bufio.NewReader(peer))
	if err != nil {
		t.Fatalf("wrapped connection did not forward the SOCKS handshake: %v", err)
	}
	want := socks5.ReplyCodeSuccess
	if fail {
		want = socks5.ReplyCodeConnectionRefused
	}
	if response.ReplyCode != want {
		t.Fatalf("SOCKS reply=%d want=%d", response.ReplyCode, want)
	}
	err = <-reported
	if fail {
		if !errors.Is(err, syscall.ECONNREFUSED) {
			t.Fatalf("original handshake error lost: %v", err)
		}
	} else if err != nil {
		t.Fatalf("report successful handshake: %v", err)
	}
	if traffic := tracker.Snapshot().Traffic[Pair{UserID: 1, NodeID: 7}]; traffic != (Traffic{}) {
		t.Fatalf("handshake bytes must not count as payload, got %+v", traffic)
	}
	if fail && len(tracker.Snapshot().Devices) != 0 {
		t.Fatal("failed handshake must close the tracked connection")
	}
}

func TestTrackedConnForwardsSOCKSHandshakeAndCountsOnlyPayload(t *testing.T) {
	for _, outcome := range []string{"success", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			tracker := NewConnTracker([]UserRef{{ID: 1, Nodes: []NodeRef{{ID: 7, Protocol: "socks"}}}})
			server, peer := handshakePipe(t)
			wrapped := tracker.RoutedConnection(context.Background(), socks.NewLazyConn(server, socks5.Version), testMetadata("u-1", "socks-7", "127.0.0.1"), nil, nil)
			t.Cleanup(func() { _ = wrapped.Close() })
			fail := outcome == "failure"
			checkTrackedSOCKSHandshake(t, tracker, peer, fail, func() error {
				if fail {
					return N.CloseOnHandshakeFailure(wrapped, nil, syscall.ECONNREFUSED)
				}
				return N.ReportConnHandshakeSuccess(wrapped, server)
			})
			if fail {
				return
			}

			peerRead := make(chan error, 1)
			go func() {
				data := make([]byte, 4)
				_, err := io.ReadFull(peer, data)
				if err == nil && string(data) != "down" {
					err = errors.New("incorrect download payload")
				}
				peerRead <- err
			}()
			if _, err := wrapped.Write([]byte("down")); err != nil {
				t.Fatal(err)
			}
			if err := <-peerRead; err != nil {
				t.Fatal(err)
			}
			peerWrite := make(chan error, 1)
			go func() { _, err := peer.Write([]byte("up")); peerWrite <- err }()
			data := make([]byte, 2)
			if _, err := io.ReadFull(wrapped, data); err != nil || string(data) != "up" {
				t.Fatalf("upload payload=%q: %v", data, err)
			}
			if err := <-peerWrite; err != nil {
				t.Fatal(err)
			}
			if traffic := tracker.Snapshot().Traffic[Pair{UserID: 1, NodeID: 7}]; traffic != (Traffic{Upload: 2, Download: 4}) {
				t.Fatalf("TCP payload must be counted exactly once, got %+v", traffic)
			}
		})
	}
}

func TestTrackedPacketConnForwardsSOCKSHandshakeAndCountsOnlyPayload(t *testing.T) {
	for _, outcome := range []string{"success", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			tracker := NewConnTracker([]UserRef{{ID: 1, Nodes: []NodeRef{{ID: 7, Protocol: "socks"}}}})
			server, peer := handshakePipe(t)
			control, controlPeer := handshakePipe(t)
			wrapped := tracker.RoutedPacketConnection(context.Background(), socks.NewLazyAssociatePacketConn(server, control), testMetadata("u-1", "socks-7", "127.0.0.1"), nil, nil)
			t.Cleanup(func() { _ = wrapped.Close() })
			fail := outcome == "failure"
			checkTrackedSOCKSHandshake(t, tracker, controlPeer, fail, func() error {
				if fail {
					return N.CloseOnHandshakeFailure(wrapped, nil, syscall.ECONNREFUSED)
				}
				return N.ReportPacketConnHandshakeSuccess(wrapped, nil)
			})
			if fail {
				return
			}

			destination := M.Socksaddr{Addr: netip.MustParseAddr("127.0.0.1"), Port: 9000}
			packetPeer := socks.NewAssociatePacketConn(peer, destination, controlPeer)
			peerWrite := make(chan error, 1)
			go func() { _, err := packetPeer.WriteTo([]byte("up"), destination.UDPAddr()); peerWrite <- err }()
			incoming := buf.New()
			defer incoming.Release()
			gotDestination, err := wrapped.ReadPacket(incoming)
			if err != nil || gotDestination != destination || string(incoming.Bytes()) != "up" {
				t.Fatalf("upload packet=%q destination=%v: %v", incoming.Bytes(), gotDestination, err)
			}
			if err := <-peerWrite; err != nil {
				t.Fatal(err)
			}
			peerRead := make(chan error, 1)
			go func() {
				data := buf.New()
				defer data.Release()
				_, err := packetPeer.ReadPacket(data)
				if err == nil && string(data.Bytes()) != "down" {
					err = errors.New("incorrect packet download payload")
				}
				peerRead <- err
			}()
			outgoing := buf.New()
			outgoing.Resize(64, 0)
			_, _ = outgoing.Write([]byte("down"))
			if err := wrapped.WritePacket(outgoing, destination); err != nil {
				t.Fatal(err)
			}
			if err := <-peerRead; err != nil {
				t.Fatal(err)
			}
			if traffic := tracker.Snapshot().Traffic[Pair{UserID: 1, NodeID: 7}]; traffic != (Traffic{Upload: 2, Download: 4}) {
				t.Fatalf("packet payload must exclude handshakes and framing, got %+v", traffic)
			}
		})
	}
}
