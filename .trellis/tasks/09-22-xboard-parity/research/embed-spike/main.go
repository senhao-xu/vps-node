package main

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sync/atomic"
	"time"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/include"
	"github.com/sagernet/sing-box/option"
	singJSON "github.com/sagernet/sing/common/json"
	N "github.com/sagernet/sing/common/network"
	"github.com/sagernet/sing/service"
)

type countingConn struct {
	net.Conn
	up   *atomic.Int64
	down *atomic.Int64
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 {
		c.up.Add(int64(n))
	}
	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 {
		c.down.Add(int64(n))
	}
	return n, err
}

type spikeTracker struct {
	up    atomic.Int64
	down  atomic.Int64
	calls atomic.Int64
	user  atomic.Value
}

func (t *spikeTracker) RoutedConnection(_ context.Context, conn net.Conn, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) net.Conn {
	t.calls.Add(1)
	t.user.Store(metadata.User)
	log.Printf("[tracker] TCP user=%q inbound=%q inboundType=%q src=%s", metadata.User, metadata.Inbound, metadata.InboundType, metadata.Source.AddrString())
	return &countingConn{Conn: conn, up: &t.up, down: &t.down}
}

func (t *spikeTracker) RoutedPacketConnection(_ context.Context, conn N.PacketConn, metadata adapter.InboundContext, _ adapter.Rule, _ adapter.Outbound) N.PacketConn {
	log.Printf("[tracker] UDP user=%q inbound=%q", metadata.User, metadata.Inbound)
	return conn
}

var _ adapter.ConnectionTracker = (*spikeTracker)(nil)

func main() {
	// local echo server as the proxied destination
	echoLn, err := net.Listen("tcp", "127.0.0.1:18099")
	if err != nil {
		log.Fatal(err)
	}
	defer echoLn.Close()
	go func() {
		for {
			c, err := echoLn.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 4096)
				for {
					n, err := c.Read(buf)
					if n > 0 {
						_, _ = c.Write(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}(c)
		}
	}()

	cfg := `{
  "log": {"level": "warn"},
  "inbounds": [{
    "type": "socks",
    "tag": "socks-in",
    "listen": "127.0.0.1",
    "listen_port": 18388,
    "users": [{"username": "u1", "password": "p1"}]
  }],
  "outbounds": [{"type": "direct", "tag": "direct"}]
}`

	ctx := include.Context(context.Background())
	opts, err := singJSON.UnmarshalExtendedContext[option.Options](ctx, []byte(cfg))
	if err != nil {
		log.Fatalf("parse options: %v", err)
	}
	inst, err := box.New(box.Options{Context: ctx, Options: opts})
	if err != nil {
		log.Fatalf("box.New: %v", err)
	}
	tracker := &spikeTracker{}
	router := service.FromContext[adapter.Router](ctx)
	if router == nil {
		log.Fatal("router not found in context")
	}
	router.AppendTracker(tracker)
	if err := inst.Start(); err != nil {
		log.Fatalf("Start: %v", err)
	}
	defer inst.Close()
	log.Println("[spike] sing-box embedded instance started")

	time.Sleep(200 * time.Millisecond)
	if err := driveSOCKS5(); err != nil {
		log.Fatalf("traffic: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	log.Printf("[spike] tracker calls=%d user=%v", tracker.calls.Load(), tracker.user.Load())
}

// driveSOCKS5 performs a username/password SOCKS5 handshake and echoes a payload.
func driveSOCKS5() error {
	c, err := net.DialTimeout("tcp", "127.0.0.1:18388", 3*time.Second)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))

	if _, err := c.Write([]byte{0x05, 0x01, 0x02}); err != nil {
		return err
	}
	resp := make([]byte, 2)
	if _, err := c.Read(resp); err != nil {
		return fmt.Errorf("method negotiate: %w", err)
	}
	if resp[0] != 0x05 || resp[1] != 0x02 {
		return fmt.Errorf("unexpected method response %v", resp)
	}
	auth := append([]byte{0x01, 0x02}, []byte("u1")...)
	auth = append(auth, 0x02)
	auth = append(auth, []byte("p1")...)
	if _, err := c.Write(auth); err != nil {
		return err
	}
	if _, err := c.Read(resp); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if resp[1] != 0x00 {
		return fmt.Errorf("auth rejected %v", resp)
	}
	req := []byte{0x05, 0x01, 0x00, 0x01, 127, 0, 0, 1}
	port := make([]byte, 2)
	binary.BigEndian.PutUint16(port, 18099)
	req = append(req, port...)
	if _, err := c.Write(req); err != nil {
		return err
	}
	reply := make([]byte, 10)
	if _, err := c.Read(reply); err != nil {
		return fmt.Errorf("connect reply: %w", err)
	}
	if reply[1] != 0x00 {
		return fmt.Errorf("connect failed %v", reply)
	}
	payload := []byte("hello-spike-payload")
	if _, err := c.Write(payload); err != nil {
		return err
	}
	echo := make([]byte, len(payload))
	if _, err := c.Read(echo); err != nil {
		return fmt.Errorf("echo: %w", err)
	}
	if string(echo) != string(payload) {
		return fmt.Errorf("echo mismatch %q", echo)
	}
	return nil
}
