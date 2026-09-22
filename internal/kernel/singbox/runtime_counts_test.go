package singbox

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func startEchoServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				_, _ = io.Copy(conn, conn)
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func httpProxyEcho(t *testing.T, proxyPort, echoPort int, payload []byte) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), 3*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	auth := base64.StdEncoding.EncodeToString([]byte("u-1:p1"))
	request := fmt.Sprintf("CONNECT 127.0.0.1:%d HTTP/1.1\r\nHost: 127.0.0.1:%d\r\nProxy-Authorization: Basic %s\r\n\r\n",
		echoPort, echoPort, auth)
	if _, err := conn.Write([]byte(request)); err != nil {
		t.Fatalf("connect write: %v", err)
	}
	reader := bufio.NewReader(conn)
	status, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("connect status: %v", err)
	}
	if status != "HTTP/1.1 200 Connection established\r\n" {
		t.Fatalf("connect rejected: %q", status)
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("connect headers: %v", err)
		}
		if line == "\r\n" {
			break
		}
	}
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("payload write: %v", err)
	}
	echo := make([]byte, len(payload))
	if _, err := io.ReadFull(reader, echo); err != nil {
		t.Fatalf("echo read: %v", err)
	}
	if string(echo) != string(payload) {
		t.Fatalf("echo mismatch %q", echo)
	}
}

func TestRuntimeCountsRealTrafficPerUser(t *testing.T) {
	echoPort := startEchoServer(t)
	proxyPort := freePort(t)

	config := fmt.Sprintf(`{
  "log": {"disabled": true},
  "inbounds": [{
    "type": "http",
    "tag": "http-7",
    "listen": "127.0.0.1",
    "listen_port": %d,
    "users": [{"username": "u-1", "password": "p1"}]
  }],
  "outbounds": [{"type": "direct", "tag": "direct"}]
}`, proxyPort)

	runtime := NewRuntime()
	if err := runtime.Start([]byte(config), []UserRef{{
		ID:    1,
		Nodes: []NodeRef{{ID: 7, Protocol: "http", Port: proxyPort}},
	}}); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		if err := runtime.Stop(); err != nil {
			t.Fatalf("stop: %v", err)
		}
	}()

	payload := []byte("hello-embedded-traffic")
	httpProxyEcho(t, proxyPort, echoPort, payload)

	pair := Pair{UserID: 1, NodeID: 7}
	want := int64(len(payload))
	deadline := time.Now().Add(2 * time.Second)
	for {
		traffic := runtime.Snapshot().Traffic[pair]
		if traffic.Upload == want && traffic.Download == want {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("tracker did not count payload %d/%d, got %+v", want, want, traffic)
		}
		time.Sleep(20 * time.Millisecond)
	}

	snapshot := runtime.Snapshot()
	device := findDevice(snapshot.Devices, pair)
	if !hasIP(device, "127.0.0.1") {
		t.Fatalf("expected source ip to be tracked, got %+v", snapshot.Devices)
	}
}
