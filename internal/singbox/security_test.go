package singbox_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	kernelsingbox "vps-node/internal/kernel/singbox"
	"vps-node/internal/singbox"
)

func TestRenderOmitsUnauthorizedHTTPAndSOCKSAndTheirRoutes(t *testing.T) {
	nodes := []singbox.Node{
		{ID: 1, Protocol: singbox.ProtocolHTTP, Port: 10001},
		{ID: 2, Protocol: singbox.ProtocolSocks, Port: 10002},
		{ID: 3, Protocol: singbox.ProtocolHTTP, Port: 10003, Users: []singbox.User{{ID: 1, UUID: "p1"}}},
		{ID: 4, Protocol: singbox.ProtocolSocks, Port: 10004, Relays: []singbox.Relay{{EntryServerID: 8}}},
		{ID: 5, Protocol: singbox.ProtocolShadowsocks, Port: 10005, Settings: map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm}},
	}
	var chains []singbox.ChainExit
	for _, node := range nodes[:4] {
		chains = append(chains, singbox.ChainExit{EntryNodeID: node.ID, Outbound: map[string]any{"type": "direct"}})
	}
	config, err := singbox.Render(testAppKey, nodes, chains...)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	inbounds := config["inbounds"].([]map[string]any)
	if len(inbounds) != 3 {
		t.Fatalf("expected authorized HTTP, relay-only SOCKS and unchanged SS, got %+v", inbounds)
	}
	for i, tag := range []string{"http-3", "socks-4", "shadowsocks-5"} {
		if inbounds[i]["tag"] != tag {
			t.Fatalf("inbound %d tag = %v, want %s", i, inbounds[i]["tag"], tag)
		}
	}
	outbounds := config["outbounds"].([]map[string]any)
	if len(outbounds) != 3 || outbounds[1]["tag"] != "chain-3" || outbounds[2]["tag"] != "chain-4" {
		t.Fatalf("skipped inbounds must have no chain outbounds, got %+v", outbounds)
	}
	rules := config["route"].(map[string]any)["rules"].([]map[string]any)
	if len(rules) != 2 || rules[0]["inbound"].([]string)[0] != "http-3" || rules[1]["inbound"].([]string)[0] != "socks-4" {
		t.Fatalf("skipped inbounds must have no route rules, got %+v", rules)
	}
}

func TestRenderRejectsManagedShadowsocksChaChaWithoutChangingNode(t *testing.T) {
	for _, credentials := range []string{"empty", "user", "relay"} {
		t.Run(credentials, func(t *testing.T) {
			node := singbox.Node{ID: 13, Protocol: singbox.ProtocolShadowsocks, Port: 8390, Settings: map[string]any{"cipher": singbox.SSMethod2022Chacha20}, Secret: map[string]any{"password": "original-key"}}
			if credentials == "user" {
				node.Users = []singbox.User{{ID: 1, UUID: "uuid-1"}}
			} else if credentials == "relay" {
				node.Relays = []singbox.Relay{{EntryServerID: 3}}
			}
			_, err := singbox.Render(testAppKey, []singbox.Node{node})
			if !errors.Is(err, singbox.ErrUnrenderable) || !strings.Contains(err.Error(), "node 13") || !strings.Contains(err.Error(), singbox.SSMethod2022Chacha20) {
				t.Fatalf("unsupported managed cipher must fail clearly, got %v", err)
			}
			if node.Settings["cipher"] != singbox.SSMethod2022Chacha20 || node.Secret["password"] != "original-key" {
				t.Fatal("rendering must not replace the stored cipher or server credential")
			}
		})
	}
}

func TestSupportedManagedShadowsocksMethodsStartMultiUserRuntime(t *testing.T) {
	for _, cipher := range []string{singbox.SSMethod2022Aes128Gcm, singbox.SSMethod2022Aes256Gcm} {
		t.Run(cipher, func(t *testing.T) {
			node := singbox.Node{ID: 11, Protocol: singbox.ProtocolShadowsocks, Settings: map[string]any{"cipher": cipher}, Users: []singbox.User{{ID: 1, UUID: "uuid-1"}, {ID: 2, UUID: "uuid-2"}}, Relays: []singbox.Relay{{EntryServerID: 3}}}
			config, err := singbox.Render(testAppKey, []singbox.Node{node})
			if err != nil {
				t.Fatalf("render supported cipher: %v", err)
			}
			config["log"] = map[string]any{"disabled": true}
			body, err := json.Marshal(config)
			if err != nil {
				t.Fatal(err)
			}
			runtime := kernelsingbox.NewRuntime()
			if err := runtime.Start(body, nil); err != nil {
				t.Fatalf("embedded multi-user runtime rejected %s: %v", cipher, err)
			}
			if err := runtime.Stop(); err != nil {
				t.Fatalf("stop: %v", err)
			}
		})
	}
}

func TestExternalShadowsocksChaChaOutboundStillStarts(t *testing.T) {
	password, err := singbox.DeriveSSPassword(testAppKey, 7, "external", singbox.SSMethod2022Chacha20)
	if err != nil {
		t.Fatalf("external credential derivation: %v", err)
	}
	outbound, err := singbox.ProxyToOutbound(map[string]any{"type": "ss", "server": "127.0.0.1", "port": 8388, "cipher": singbox.SSMethod2022Chacha20, "password": password})
	if err != nil {
		t.Fatalf("convert external ChaCha: %v", err)
	}
	body, err := json.Marshal(map[string]any{"log": map[string]any{"disabled": true}, "outbounds": []any{outbound}})
	if err != nil {
		t.Fatal(err)
	}
	runtime := kernelsingbox.NewRuntime()
	if err := runtime.Start(body, nil); err != nil {
		t.Fatalf("external ChaCha client must remain supported: %v", err)
	}
	if err := runtime.Stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
}
