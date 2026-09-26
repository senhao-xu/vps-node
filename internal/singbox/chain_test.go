package singbox_test

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"vps-node/internal/singbox"
)

func testRealityKey(t *testing.T) (privateB64, publicB64 string) {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("x25519: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes()),
		base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
}

func TestRelayCredentialDeterministicAndIsolated(t *testing.T) {
	p1, err := singbox.DeriveRelaySSPassword(testAppKey, 7, 3, singbox.SSMethod2022Aes128Gcm)
	if err != nil {
		t.Fatalf("derive relay ss password: %v", err)
	}
	p2, _ := singbox.DeriveRelaySSPassword(testAppKey, 7, 3, singbox.SSMethod2022Aes128Gcm)
	if p1 != p2 {
		t.Fatal("same inputs must derive the same relay password")
	}
	other, _ := singbox.DeriveRelaySSPassword(testAppKey, 7, 4, singbox.SSMethod2022Aes128Gcm)
	if other == p1 {
		t.Fatal("different entry servers must derive different relay passwords")
	}
	otherNode, _ := singbox.DeriveRelaySSPassword(testAppKey, 8, 3, singbox.SSMethod2022Aes128Gcm)
	if otherNode == p1 {
		t.Fatal("different exit nodes must derive different relay passwords")
	}

	u1 := singbox.DeriveRelayUUID(testAppKey, 7, 3)
	if u1 != singbox.DeriveRelayUUID(testAppKey, 7, 3) {
		t.Fatal("relay uuid must be deterministic")
	}
	if u1 == singbox.DeriveRelayUUID(testAppKey, 7, 4) {
		t.Fatal("relay uuid must differ per entry server")
	}
	pw := singbox.DeriveRelayPassword(testAppKey, 7, 3)
	if pw != singbox.DeriveRelayPassword(testAppKey, 7, 3) || len(pw) != 32 {
		t.Fatalf("relay password must be a deterministic 32-char hex, got %q", pw)
	}
}

func TestRenderChainShadowsocks(t *testing.T) {
	entry := singbox.Node{
		ID: 1, Protocol: singbox.ProtocolShadowsocks, Port: 10001,
		Settings: map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm},
		Users:    []singbox.User{{ID: 1, UUID: "uuid-1"}},
	}
	exit := singbox.Node{
		ID: 2, Protocol: singbox.ProtocolShadowsocks, Port: 20002,
		Settings: map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm},
		Secret:   map[string]any{"password": "exit-server-key"},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{entry}, singbox.ChainExit{
		EntryNodeID: 1, EntryServerID: 3, Exit: exit, DialAddress: "exit.example.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	outbounds := config["outbounds"].([]map[string]any)
	if len(outbounds) != 2 {
		t.Fatalf("expected direct + chain outbound, got %d", len(outbounds))
	}
	chain := outbounds[1]
	if chain["type"] != "shadowsocks" || chain["tag"] != "chain-1" ||
		chain["server"] != "exit.example.com" || chain["server_port"] != 20002 ||
		chain["method"] != singbox.SSMethod2022Aes128Gcm {
		t.Fatalf("unexpected chain outbound: %+v", chain)
	}
	relayKey, _ := singbox.DeriveRelaySSPassword(testAppKey, 2, 3, singbox.SSMethod2022Aes128Gcm)
	want := singbox.CombineSSClientPassword(singbox.SSMethod2022Aes128Gcm, "exit-server-key", relayKey)
	if chain["password"] != want {
		t.Fatalf("chain password must be server:relay combined, got %v want %v", chain["password"], want)
	}

	route := config["route"].(map[string]any)
	if route["final"] != "direct" {
		t.Fatalf("route final must stay direct: %+v", route)
	}
	rules := route["rules"].([]map[string]any)
	if len(rules) != 1 {
		t.Fatalf("expected one chain route rule, got %v", rules)
	}
	inboundTags := rules[0]["inbound"].([]string)
	if len(inboundTags) != 1 || inboundTags[0] != "shadowsocks-1" || rules[0]["outbound"] != "chain-1" {
		t.Fatalf("unexpected route rule: %+v", rules[0])
	}
}

func TestRenderChainVLESS(t *testing.T) {
	privateKey, publicKey := testRealityKey(t)
	entry := singbox.Node{
		ID: 1, Protocol: singbox.ProtocolVLESS, Port: 443,
		Settings: map[string]any{"reality_settings": map[string]any{"server_name": "a.com"}},
		Secret:   map[string]any{"private_key": privateKey},
	}
	exit := singbox.Node{
		ID: 2, Protocol: singbox.ProtocolVLESS, Port: 443,
		Settings: map[string]any{"reality_settings": map[string]any{"server_name": "exit.com", "short_id": "abcd1234"}},
		Secret:   map[string]any{"private_key": privateKey},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{entry}, singbox.ChainExit{
		EntryNodeID: 1, EntryServerID: 3, Exit: exit, DialAddress: "exit.example.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	chain := config["outbounds"].([]map[string]any)[1]
	if chain["type"] != "vless" || chain["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected vless outbound: %+v", chain)
	}
	if chain["uuid"] != singbox.DeriveRelayUUID(testAppKey, 2, 3) {
		t.Fatalf("relay uuid mismatch: %+v", chain)
	}
	tlsMap := chain["tls"].(map[string]any)
	if tlsMap["server_name"] != "exit.com" {
		t.Fatalf("tls server_name must come from the exit node, got %+v", tlsMap)
	}
	reality := tlsMap["reality"].(map[string]any)
	if reality["public_key"] != publicKey {
		t.Fatalf("public key must be derived from the exit private key: %v != %s", reality["public_key"], publicKey)
	}
	if reality["short_id"] != "abcd1234" {
		t.Fatalf("short id mismatch: %+v", reality)
	}
}

func TestRenderChainHysteria2AndAnyTLS(t *testing.T) {
	entry := singbox.Node{
		ID: 1, Protocol: singbox.ProtocolShadowsocks, Port: 10001,
		Settings: map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm},
	}
	hy2Exit := singbox.Node{
		ID: 2, Protocol: singbox.ProtocolHysteria2, Port: 8443,
		Settings: map[string]any{
			"tls":  map[string]any{"server_name": "hy2.example.com"},
			"obfs": map[string]any{"open": true, "type": "salamander", "password": "obfs-secret"},
		},
		Secret: map[string]any{"certificate": "-----BEGIN CERTIFICATE-----\nc1\n-----END CERTIFICATE-----"},
	}
	anyExit := singbox.Node{
		ID: 3, Protocol: singbox.ProtocolAnyTLS, Port: 9443,
		Settings: map[string]any{"tls": map[string]any{"server_name": "any.example.com"}},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{entry},
		singbox.ChainExit{EntryNodeID: 1, EntryServerID: 3, Exit: hy2Exit, DialAddress: "hy2.exit.com"},
	)
	if err != nil {
		t.Fatalf("render hy2 chain: %v", err)
	}
	chain := config["outbounds"].([]map[string]any)[1]
	if chain["type"] != "hysteria2" || chain["password"] != singbox.DeriveRelayPassword(testAppKey, 2, 3) {
		t.Fatalf("unexpected hy2 chain outbound: %+v", chain)
	}
	tlsMap := chain["tls"].(map[string]any)
	if tlsMap["server_name"] != "hy2.example.com" {
		t.Fatalf("hy2 chain tls mismatch: %+v", tlsMap)
	}
	if cert, ok := tlsMap["certificate"].([]string); !ok || len(cert) != 3 {
		t.Fatalf("exit certificate must be pinned for the relay client, got %+v", tlsMap)
	}
	obfs := chain["obfs"].(map[string]any)
	if obfs["type"] != "salamander" || obfs["password"] != "obfs-secret" {
		t.Fatalf("hy2 chain must carry obfs, got %+v", chain)
	}
	for _, key := range []string{"hop_ports", "mport", "ports"} {
		if _, has := chain[key]; has {
			t.Fatalf("port hopping must not enter chain outbounds: %+v", chain)
		}
	}

	config, err = singbox.Render(testAppKey, []singbox.Node{entry},
		singbox.ChainExit{EntryNodeID: 1, EntryServerID: 3, Exit: anyExit, DialAddress: "any.exit.com"},
	)
	if err != nil {
		t.Fatalf("render anytls chain: %v", err)
	}
	chain = config["outbounds"].([]map[string]any)[1]
	if chain["type"] != "anytls" || chain["password"] != singbox.DeriveRelayPassword(testAppKey, 3, 3) {
		t.Fatalf("unexpected anytls chain outbound: %+v", chain)
	}
	tlsMap = chain["tls"].(map[string]any)
	if tlsMap["insecure"] != true {
		t.Fatalf("anytls chain without a stored certificate must fall back to insecure, got %+v", tlsMap)
	}
}

func TestRenderRelayUsersInjected(t *testing.T) {
	// Exit-side: the inbound carries one relay pseudo user per entry server,
	// and the credential matches what the entry-side outbound derives.
	for _, protocol := range []string{singbox.ProtocolShadowsocks, singbox.ProtocolVLESS, singbox.ProtocolHysteria2, singbox.ProtocolAnyTLS} {
		node := singbox.Node{ID: 9, Protocol: protocol, Port: 9000, Relays: []singbox.Relay{{EntryServerID: 3}}}
		switch protocol {
		case singbox.ProtocolShadowsocks:
			node.Settings = map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm}
		case singbox.ProtocolVLESS:
			privateKey, _ := testRealityKey(t)
			node.Settings = map[string]any{"reality_settings": map[string]any{"server_name": "a.com"}}
			node.Secret = map[string]any{"private_key": privateKey}
		case singbox.ProtocolHysteria2, singbox.ProtocolAnyTLS:
			node.Settings = map[string]any{"tls": map[string]any{"server_name": "a.com"}}
		}
		config, err := singbox.Render(testAppKey, []singbox.Node{node})
		if err != nil {
			t.Fatalf("render %s with relay: %v", protocol, err)
		}
		users := config["inbounds"].([]map[string]any)[0]["users"].([]map[string]any)
		if len(users) != 1 || users[0]["name"] != singbox.RelayUserName(3) {
			t.Fatalf("%s must carry the relay pseudo user, got %+v", protocol, users)
		}
		switch protocol {
		case singbox.ProtocolShadowsocks:
			want, _ := singbox.DeriveRelaySSPassword(testAppKey, 9, 3, singbox.SSMethod2022Aes128Gcm)
			if users[0]["password"] != want {
				t.Fatalf("ss relay password mismatch: %v", users[0])
			}
		case singbox.ProtocolVLESS:
			if users[0]["uuid"] != singbox.DeriveRelayUUID(testAppKey, 9, 3) {
				t.Fatalf("vless relay uuid mismatch: %v", users[0])
			}
		default:
			if users[0]["password"] != singbox.DeriveRelayPassword(testAppKey, 9, 3) {
				t.Fatalf("%s relay password mismatch: %v", protocol, users[0])
			}
		}
	}
}

func TestRenderChainUnknownEntrySkipped(t *testing.T) {
	entry := singbox.Node{
		ID: 1, Protocol: singbox.ProtocolShadowsocks, Port: 10001,
		Settings: map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm},
	}
	config, err := singbox.Render(testAppKey, []singbox.Node{entry}, singbox.ChainExit{
		EntryNodeID: 99, EntryServerID: 3,
		Exit: singbox.Node{ID: 2, Protocol: singbox.ProtocolShadowsocks, Port: 20002,
			Settings: map[string]any{"cipher": singbox.SSMethod2022Aes128Gcm}},
		DialAddress: "exit.example.com",
	})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	// The outbound is rendered but no route rule references it; traffic stays direct.
	if _, has := config["route"].(map[string]any)["rules"]; has {
		t.Fatal("no route rule must be rendered for an unknown entry node")
	}
}
