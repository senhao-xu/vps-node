package singbox

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/proxy"

	renderer "vps-node/internal/singbox"
)

var runtimeTestAppKey = make([]byte, 32)

func renderRuntimeConfig(t *testing.T, nodes []renderer.Node) []byte {
	t.Helper()
	config, err := renderer.Render(runtimeTestAppKey, nodes)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	config["log"] = map[string]any{"disabled": true}
	body, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return body
}

func assertAnonymousProxyDenied(t *testing.T, protocol string, proxyPort, echoPort int) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), time.Second)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if protocol == renderer.ProtocolSocks {
		if _, err := conn.Write([]byte{5, 1, 0}); err != nil {
			t.Fatalf("write anonymous greeting: %v", err)
		}
		var reply [2]byte
		if _, err := io.ReadFull(conn, reply[:]); err != nil {
			t.Fatalf("read anonymous greeting: %v", err)
		}
		if reply != [2]byte{5, 255} {
			t.Fatalf("SOCKS must reject anonymous authentication, got %v", reply)
		}
		return
	}
	request := fmt.Sprintf("CONNECT 127.0.0.1:%d HTTP/1.1\r\nHost: 127.0.0.1:%d\r\n\r\n", echoPort, echoPort)
	if _, err := io.WriteString(conn, request); err != nil {
		t.Fatalf("write anonymous CONNECT: %v", err)
	}
	status, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read anonymous CONNECT status: %v", err)
	}
	if !strings.Contains(status, " 407 ") {
		t.Fatalf("HTTP must require proxy authentication, got %q", status)
	}
}

func assertProxyPortClosed(t *testing.T, port int) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
	if err == nil {
		_ = conn.Close()
		t.Fatal("proxy without users or relays must have no listener")
	}
}

func waitForRuntimeTraffic(t *testing.T, runtime *Runtime, pair Pair, want Traffic) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := runtime.Snapshot().Traffic[pair]
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("traffic for %+v = %+v, want %+v", pair, got, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func proxyEchoWithAuth(t *testing.T, protocol string, proxyPort, echoPort int, username, password string, payload []byte) {
	t.Helper()
	if protocol == renderer.ProtocolHTTP {
		httpProxyEchoWithAuth(t, proxyPort, echoPort, username, password, payload)
		return
	}
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", proxyPort), &proxy.Auth{User: username, Password: password}, &net.Dialer{Timeout: time.Second})
	if err != nil {
		t.Fatalf("SOCKS dialer: %v", err)
	}
	conn, err := dialer.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", echoPort))
	if err != nil {
		t.Fatalf("authenticated SOCKS CONNECT: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(time.Second))
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("write SOCKS payload: %v", err)
	}
	echo := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, echo); err != nil || string(echo) != string(payload) {
		t.Fatalf("SOCKS echo: %q, %v", echo, err)
	}
}

func TestRuntimeHTTPAndSOCKSFailClosedWithoutUsers(t *testing.T) {
	for _, protocol := range []string{renderer.ProtocolHTTP, renderer.ProtocolSocks} {
		t.Run(protocol, func(t *testing.T) {
			echoPort := startEchoServer(t)
			node := renderer.Node{ID: 7, Protocol: protocol, Port: freePort(t)}
			runtime := NewRuntime()
			t.Cleanup(func() { _ = runtime.Stop() })
			apply := func() {
				t.Helper()
				if err := runtime.Reload(renderRuntimeConfig(t, []renderer.Node{node}), nil); err != nil {
					t.Fatalf("apply: %v", err)
				}
			}

			apply()
			assertAnonymousProxyDenied(t, protocol, node.Port, echoPort)
			assertProxyPortClosed(t, node.Port)

			node.Users = []renderer.User{{ID: 1, UUID: "p1"}}
			apply()
			proxyEchoWithAuth(t, protocol, node.Port, echoPort, "u-1", "p1", []byte("authorized"))
			assertAnonymousProxyDenied(t, protocol, node.Port, echoPort)

			node.Users = nil
			apply()
			assertAnonymousProxyDenied(t, protocol, node.Port, echoPort)
			assertProxyPortClosed(t, node.Port)

			node.Relays = []renderer.Relay{{EntryServerID: 3}}
			apply()
			proxyEchoWithAuth(t, protocol, node.Port, echoPort, renderer.RelayUserName(3), renderer.DeriveRelayPassword(runtimeTestAppKey, node.ID, 3), []byte("relay-only"))
			assertAnonymousProxyDenied(t, protocol, node.Port, echoPort)

			node.Relays = nil
			apply()
			assertAnonymousProxyDenied(t, protocol, node.Port, echoPort)
			assertProxyPortClosed(t, node.Port)
		})
	}
}

func TestRuntimeRestoresLastSuccessfulServiceAfterBindFailure(t *testing.T) {
	echoPort := startEchoServer(t)
	nodes := []renderer.Node{
		{ID: 7, Protocol: renderer.ProtocolHTTP, Port: freePort(t), Users: []renderer.User{{ID: 1, UUID: "p1"}}},
		{ID: 8, Protocol: renderer.ProtocolSocks, Port: freePort(t), Users: []renderer.User{{ID: 1, UUID: "p1"}}},
	}
	users := []UserRef{{ID: 1, Nodes: []NodeRef{
		{ID: 7, Protocol: "http", Port: nodes[0].Port},
		{ID: 8, Protocol: "socks", Port: nodes[1].Port},
	}}}
	body := renderRuntimeConfig(t, nodes)
	runtime := NewRuntime()
	if err := runtime.Start(body, users); err != nil {
		t.Fatalf("start: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Stop() })
	payload := []byte("before-rollback")
	proxyEchoWithAuth(t, "http", nodes[0].Port, echoPort, "u-1", "p1", payload)
	waitForRuntimeTraffic(t, runtime, Pair{UserID: 1, NodeID: 7}, Traffic{Upload: int64(len(payload)), Download: int64(len(payload))})
	for i := range body {
		body[i] = ' '
	}
	users[0].ID = 99
	users[0].Nodes[0].ID = 99
	users[0].Nodes[1].Protocol = "http"
	if runtime.lastUsers[0].ID != 1 || runtime.lastUsers[0].Nodes[0].ID != 7 || runtime.lastUsers[0].Nodes[1].Protocol != "socks" {
		t.Fatal("last successful user references must be independent of the caller")
	}

	occupied, err := net.Listen("tcp", "[::]:0")
	if err != nil {
		t.Fatalf("occupy port: %v", err)
	}
	defer occupied.Close()
	badNode := renderer.Node{ID: 9, Protocol: renderer.ProtocolHTTP, Port: occupied.Addr().(*net.TCPAddr).Port, Users: []renderer.User{{ID: 1, UUID: "p1"}}}
	badConfig := renderRuntimeConfig(t, append(append([]renderer.Node(nil), nodes...), badNode))
	for attempt := 0; attempt < 2; attempt++ {
		err := runtime.Reload(badConfig, nil)
		if err == nil || !strings.Contains(err.Error(), "start sing-box") || !strings.Contains(err.Error(), "address already in use") {
			t.Fatalf("failed apply must report the bind error, got %v", err)
		}
		if strings.Contains(err.Error(), "restore previous") {
			t.Fatalf("previous config should restore: %v", err)
		}
		for _, node := range nodes {
			proxyEchoWithAuth(t, node.Protocol, node.Port, echoPort, "u-1", "p1", payload)
		}
	}
	waitForRuntimeTraffic(t, runtime, Pair{UserID: 1, NodeID: 7}, Traffic{Upload: 3 * int64(len(payload)), Download: 3 * int64(len(payload))})
	waitForRuntimeTraffic(t, runtime, Pair{UserID: 1, NodeID: 8}, Traffic{Upload: 2 * int64(len(payload)), Download: 2 * int64(len(payload))})
	if visits := runtime.DrainVisits(); len(visits) != 5 {
		t.Fatalf("rollback must preserve buffered visits, got %d", len(visits))
	}
	if err := runtime.Reload(renderRuntimeConfig(t, nodes), []UserRef{{ID: 1, Nodes: []NodeRef{{ID: 7, Protocol: "http"}, {ID: 8, Protocol: "socks"}}}}); err != nil {
		t.Fatalf("successful update after rollback: %v", err)
	}
	for _, node := range nodes {
		proxyEchoWithAuth(t, node.Protocol, node.Port, echoPort, "u-1", "p1", payload)
	}
}
