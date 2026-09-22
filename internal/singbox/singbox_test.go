package singbox_test

import (
	"encoding/json"
	"testing"

	"vps-node/internal/singbox"
)

var testAppKey = make([]byte, 32)

func init() {
	for i := range testAppKey {
		testAppKey[i] = byte(i)
	}
}

func TestDeriveSSPasswordDeterministic(t *testing.T) {
	a1, err := singbox.DeriveSSPassword(testAppKey, 7, "uuid-a", singbox.SSMethod2022Aes128Gcm)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	a2, _ := singbox.DeriveSSPassword(testAppKey, 7, "uuid-a", singbox.SSMethod2022Aes128Gcm)
	if a1 != a2 {
		t.Fatalf("same inputs must produce same password: %s != %s", a1, a2)
	}
	if len(a1) != 24 {
		t.Fatalf("aes-128 password must be base64 of 16 bytes (24 chars), got %d", len(a1))
	}

	b, _ := singbox.DeriveSSPassword(testAppKey, 8, "uuid-a", singbox.SSMethod2022Aes128Gcm)
	if b == a1 {
		t.Fatal("different node must derive different password")
	}
	c, _ := singbox.DeriveSSPassword(testAppKey, 7, "uuid-b", singbox.SSMethod2022Aes128Gcm)
	if c == a1 {
		t.Fatal("different user must derive different password")
	}

	d, _ := singbox.DeriveSSPassword(testAppKey, 7, "uuid-a", singbox.SSMethod2022Aes256Gcm)
	if len(d) != 44 {
		t.Fatalf("aes-256 password must be base64 of 32 bytes (44 chars), got %d", len(d))
	}
	if _, err := singbox.DeriveSSPassword(testAppKey, 7, "uuid-a", "aes-128-gcm"); err == nil {
		t.Fatal("legacy method must be rejected by ss-cred-v1 contract")
	}
}

func TestRenderShadowsocksInbound(t *testing.T) {
	users := []singbox.User{{ID: 1, UUID: "uuid-1"}, {ID: 2, UUID: "uuid-2"}}
	node := singbox.Node{
		ID:       11,
		Name:     "hk-ss",
		Protocol: singbox.ProtocolShadowsocks,
		Port:     8388,
		Settings: map[string]any{"method": singbox.SSMethod2022Aes128Gcm},
		Secret:   map[string]any{"password": "server-secret"},
		Users:    users,
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{Port: 29090, Secret: "clash-secret"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	inbounds := config["inbounds"].([]map[string]any)
	if len(inbounds) != 1 {
		t.Fatalf("expected 1 inbound, got %d", len(inbounds))
	}
	inbound := inbounds[0]
	if inbound["type"] != "shadowsocks" || inbound["tag"] != "shadowsocks-11" ||
		inbound["listen_port"] != 8388 || inbound["method"] != singbox.SSMethod2022Aes128Gcm {
		t.Fatalf("unexpected inbound: %+v", inbound)
	}
	if inbound["password"] != "server-secret" {
		t.Fatalf("server password must come from node secret, got %v", inbound["password"])
	}
	rendered := inbound["users"].([]map[string]any)
	if len(rendered) != 2 {
		t.Fatalf("expected 2 users, got %d", len(rendered))
	}
	for i, u := range users {
		want, _ := singbox.DeriveSSPassword(testAppKey, 11, u.UUID, singbox.SSMethod2022Aes128Gcm)
		if rendered[i]["password"] != want {
			t.Fatalf("user %d password mismatch: %v != %s", u.ID, rendered[i]["password"], want)
		}
	}
}

func TestRenderShadowsocksDerivedServerPassword(t *testing.T) {
	node := singbox.Node{
		ID:       12,
		Protocol: singbox.ProtocolShadowsocks,
		Port:     8389,
		Settings: map[string]any{"method": singbox.SSMethod2022Chacha20},
		Users:    []singbox.User{{ID: 1, UUID: "uuid-1"}},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{Port: 29090, Secret: "s"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	inbound := config["inbounds"].([]map[string]any)[0]
	want, _ := singbox.DeriveSSServerPassword(testAppKey, 12, singbox.SSMethod2022Chacha20)
	if inbound["password"] != want {
		t.Fatalf("expected derived server password %s, got %v", want, inbound["password"])
	}
}

func TestRenderShadowsocksUnsupportedMethodFails(t *testing.T) {
	node := singbox.Node{
		ID:       13,
		Protocol: singbox.ProtocolShadowsocks,
		Port:     8390,
		Settings: map[string]any{},
	}
	if _, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{}); err == nil {
		t.Fatal("expected error for missing method")
	}
}

func TestRenderVLESSReality(t *testing.T) {
	node := singbox.Node{
		ID:       21,
		Protocol: singbox.ProtocolVLESS,
		Port:     443,
		Settings: map[string]any{"short_id": "abcd1234", "server_names": []any{"example.com"}},
		Secret:   map[string]any{"private_key": "privkey-xyz"},
		Users: []singbox.User{
			{ID: 1, UUID: "uuid-1"},
			{ID: 2, UUID: "uuid-2"},
		},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{Port: 29090, Secret: "s"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	inbound := config["inbounds"].([]map[string]any)[0]
	if inbound["type"] != "vless" || inbound["listen_port"] != 443 {
		t.Fatalf("unexpected inbound: %+v", inbound)
	}
	users := inbound["users"].([]map[string]any)
	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}
	if users[0]["name"] != "u-1" || users[0]["uuid"] != "uuid-1" || users[0]["flow"] != "xtls-rprx-vision" {
		t.Fatalf("vless users must carry u-<id> name for traffic attribution, got %+v", users[0])
	}
	tlsMap := inbound["tls"].(map[string]any)
	if tlsMap["enabled"] != true || tlsMap["server_name"] != "example.com" {
		t.Fatalf("unexpected tls: %+v", tlsMap)
	}
	reality := tlsMap["reality"].(map[string]any)
	if reality["private_key"] != "privkey-xyz" {
		t.Fatalf("reality private_key must be the decrypted secret, got %v", reality["private_key"])
	}
	shortIDs := reality["short_id"].([]string)
	if len(shortIDs) != 1 || shortIDs[0] != "abcd1234" {
		t.Fatalf("unexpected short_id: %v", shortIDs)
	}
	handshake := reality["handshake"].(map[string]any)
	if handshake["server"] != "example.com" {
		t.Fatalf("unexpected handshake: %+v", handshake)
	}
}

func TestRenderHysteria2Obfs(t *testing.T) {
	node := singbox.Node{
		ID:       32,
		Protocol: singbox.ProtocolHysteria2,
		Port:     8443,
		Settings: map[string]any{"obfs_password": "obfs-secret", "hop_ports": "30000-40000"},
		Users:    []singbox.User{{ID: 1, UUID: "uuid-1"}},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	inbound := config["inbounds"].([]map[string]any)[0]
	obfs, ok := inbound["obfs"].(map[string]any)
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "obfs-secret" {
		t.Fatalf("expected salamander obfs, got %+v", inbound["obfs"])
	}
	for _, key := range []string{"hop_ports", "mport", "ports"} {
		if _, has := inbound[key]; has {
			t.Fatalf("hop_ports must not leak into the server inbound: %+v", inbound)
		}
	}

	// Without obfs_password the inbound must not carry the obfs key.
	node.Settings = map[string]any{}
	config, err = singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if _, has := config["inbounds"].([]map[string]any)[0]["obfs"]; has {
		t.Fatal("obfs must be omitted when obfs_password is empty")
	}
}

func TestRenderVLESSMissingMaterialFails(t *testing.T) {
	noKey := singbox.Node{ID: 1, Protocol: singbox.ProtocolVLESS, Port: 443,
		Settings: map[string]any{"server_names": []any{"a.com"}}}
	if _, err := singbox.Render(testAppKey, []singbox.Node{noKey}, singbox.ClashAPI{}); err == nil {
		t.Fatal("expected error for missing private_key")
	}
	noNames := singbox.Node{ID: 1, Protocol: singbox.ProtocolVLESS, Port: 443,
		Secret: map[string]any{"private_key": "k"}}
	if _, err := singbox.Render(testAppKey, []singbox.Node{noNames}, singbox.ClashAPI{}); err == nil {
		t.Fatal("expected error for missing server_names")
	}
}

func TestRenderHysteria2(t *testing.T) {
	node := singbox.Node{
		ID:       31,
		Protocol: singbox.ProtocolHysteria2,
		Port:     8443,
		Settings: map[string]any{"up_mbps": float64(100), "down_mbps": float64(200)},
		Users:    []singbox.User{{ID: 1, UUID: "uuid-1"}},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{Port: 29090, Secret: "s"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	inbound := config["inbounds"].([]map[string]any)[0]
	if inbound["type"] != "hysteria2" || inbound["listen_port"] != 8443 {
		t.Fatalf("unexpected inbound: %+v", inbound)
	}
	if inbound["up_mbps"] != int64(100) || inbound["down_mbps"] != int64(200) {
		t.Fatalf("unexpected bandwidth: %+v", inbound)
	}
	users := inbound["users"].([]map[string]any)
	if len(users) != 1 || users[0]["name"] != "u-1" || users[0]["password"] != "uuid-1" {
		t.Fatalf("hysteria2 users must use uuid as password with u-<id> name, got %+v", users)
	}
	tlsMap := inbound["tls"].(map[string]any)
	if tlsMap["enabled"] != true {
		t.Fatalf("hysteria2 tls must be enabled, got %+v", tlsMap)
	}
}

func TestRenderAnyTLS(t *testing.T) {
	node := singbox.Node{
		ID:       51,
		Name:     "hk-anytls",
		Protocol: singbox.ProtocolAnyTLS,
		Port:     9443,
		Settings: map[string]any{"server_name": "anytls.example.com"},
		Secret: map[string]any{
			"certificate": "-----BEGIN CERTIFICATE-----\nline1\nline2\n-----END CERTIFICATE-----",
			"private_key": "-----BEGIN PRIVATE KEY-----\nkey1\n-----END PRIVATE KEY-----",
		},
		Users: []singbox.User{{ID: 7, UUID: "uuid-7"}},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{node}, singbox.ClashAPI{Port: 29090, Secret: "s"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	inbound := config["inbounds"].([]map[string]any)[0]
	if inbound["type"] != "anytls" || inbound["tag"] != "anytls-51" || inbound["listen_port"] != 9443 {
		t.Fatalf("unexpected inbound: %+v", inbound)
	}
	users := inbound["users"].([]map[string]any)
	if len(users) != 1 || users[0]["name"] != "u-7" || users[0]["password"] != "uuid-7" {
		t.Fatalf("anytls users must use uuid as password with u-<id> name, got %+v", users)
	}
	tlsMap := inbound["tls"].(map[string]any)
	if tlsMap["enabled"] != true || tlsMap["server_name"] != "anytls.example.com" {
		t.Fatalf("unexpected tls: %+v", tlsMap)
	}
	certLines := tlsMap["certificate"].([]string)
	if len(certLines) != 4 || certLines[1] != "line1" {
		t.Fatalf("certificate must be split into lines, got %v", certLines)
	}
	keyLines := tlsMap["key"].([]string)
	if len(keyLines) != 3 || keyLines[1] != "key1" {
		t.Fatalf("private key must be split into lines, got %v", keyLines)
	}
	for _, absent := range []string{"up_mbps", "down_mbps", "obfs", "padding_scheme"} {
		if _, has := inbound[absent]; has {
			t.Fatalf("anytls inbound must not contain %q: %+v", absent, inbound)
		}
	}
}

func TestRenderMultipleNodesShapeAndClashAPI(t *testing.T) {
	nodes := []singbox.Node{
		{ID: 41, Protocol: singbox.ProtocolHysteria2, Port: 10001,
			Users: []singbox.User{{ID: 1, UUID: "uuid-1"}}},
		{ID: 42, Protocol: singbox.ProtocolShadowsocks, Port: 10002,
			Settings: map[string]any{"method": singbox.SSMethod2022Aes256Gcm},
			Users:    []singbox.User{{ID: 2, UUID: "uuid-2"}}},
	}
	config, err := singbox.Render(testAppKey, nodes, singbox.ClashAPI{Port: 29090, Secret: "clash-secret"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if singbox.ContractVersion == "" {
		t.Fatal("renderer contract version must be set")
	}
	inbounds := config["inbounds"].([]map[string]any)
	if len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds, got %d", len(inbounds))
	}
	if inbounds[0]["users"].([]map[string]any)[0]["password"] != "uuid-1" {
		t.Fatal("cross-node user leakage in inbounds")
	}
	experimental := config["experimental"].(map[string]any)
	clash := experimental["clash_api"].(map[string]any)
	if clash["external_controller"] != "127.0.0.1:29090" || clash["secret"] != "clash-secret" {
		t.Fatalf("unexpected clash_api: %+v", clash)
	}
	if config["route"].(map[string]any)["final"] != "direct" {
		t.Fatalf("unexpected route: %+v", config["route"])
	}

	if _, err := singbox.Render(testAppKey, []singbox.Node{{ID: 1, Protocol: "snell", Port: 1}}, singbox.ClashAPI{}); err == nil {
		t.Fatal("unknown protocol must fail")
	}

	raw, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(raw, &round); err != nil {
		t.Fatalf("config must be JSON serializable: %v", err)
	}
}
