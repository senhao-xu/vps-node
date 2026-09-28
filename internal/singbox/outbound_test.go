package singbox_test

import (
	"encoding/json"
	"testing"

	kernelsingbox "vps-node/internal/kernel/singbox"
	"vps-node/internal/singbox"
)

// validateOutbound proves the produced mapping is accepted by the embedded
// sing-box option parser.
func validateOutbound(t *testing.T, outbound map[string]any) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"outbounds": []any{outbound},
		"route":     map[string]any{"final": "direct"},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := kernelsingbox.Validate(body); err != nil {
		t.Fatalf("embedded sing-box rejected outbound %v: %v", outbound, err)
	}
}

func TestProxyToOutboundShadowsocks(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-ss", "type": "ss", "server": "ss.example.com", "port": 8388,
		"cipher": "aes-256-gcm", "password": "secret",
	})
	if err != nil {
		t.Fatalf("convert ss: %v", err)
	}
	if outbound["type"] != "shadowsocks" || outbound["server"] != "ss.example.com" ||
		outbound["server_port"] != 8388 || outbound["method"] != "aes-256-gcm" || outbound["password"] != "secret" {
		t.Fatalf("unexpected ss outbound: %+v", outbound)
	}
	if _, has := outbound["tag"]; has {
		t.Fatalf("converter must not set a tag: %+v", outbound)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundVLESSReality(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-vless", "type": "vless", "server": "vless.example.com", "port": 443,
		"uuid": "11111111-2222-3333-4444-555555555555", "flow": "xtls-rprx-vision",
		"tls": true, "servername": "reality.example.com", "network": "ws",
		"reality-opts": map[string]any{"public-key": "pbk", "short-id": "abcd1234"},
		"ws-opts":      map[string]any{"path": "/ws", "headers": map[string]any{"Host": "h.example.com"}},
	})
	if err != nil {
		t.Fatalf("convert vless: %v", err)
	}
	if outbound["type"] != "vless" || outbound["uuid"] != "11111111-2222-3333-4444-555555555555" || outbound["flow"] != "xtls-rprx-vision" {
		t.Fatalf("unexpected vless outbound: %+v", outbound)
	}
	tls := outbound["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "reality.example.com" {
		t.Fatalf("unexpected vless tls: %+v", tls)
	}
	reality := tls["reality"].(map[string]any)
	if reality["enabled"] != true || reality["public_key"] != "pbk" || reality["short_id"] != "abcd1234" {
		t.Fatalf("unexpected reality block: %+v", reality)
	}
	transport := outbound["transport"].(map[string]any)
	if transport["type"] != "ws" || transport["path"] != "/ws" {
		t.Fatalf("unexpected transport: %+v", transport)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundVLESSPlainTLS(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-vless", "type": "vless", "server": "vless.example.com", "port": 8443,
		"uuid": "uuid-1", "tls": true, "sni": "sni.example.com", "skip-cert-verify": true,
	})
	if err != nil {
		t.Fatalf("convert vless: %v", err)
	}
	if _, has := outbound["flow"]; has {
		t.Fatalf("a flowless vless proxy must not carry flow: %+v", outbound)
	}
	tls := outbound["tls"].(map[string]any)
	if tls["server_name"] != "sni.example.com" || tls["insecure"] != true {
		t.Fatalf("unexpected vless tls: %+v", tls)
	}
	if _, has := tls["reality"]; has {
		t.Fatalf("no reality-opts means no reality block: %+v", tls)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundTrojan(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-trojan", "type": "trojan", "server": "trojan.example.com", "port": 443,
		"password": "pw", "sni": "trojan.example.com", "skip-cert-verify": true, "network": "grpc",
		"grpc-opts": map[string]any{"grpc-service-name": "gs"},
	})
	if err != nil {
		t.Fatalf("convert trojan: %v", err)
	}
	if outbound["type"] != "trojan" || outbound["password"] != "pw" {
		t.Fatalf("unexpected trojan outbound: %+v", outbound)
	}
	tls := outbound["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "trojan.example.com" || tls["insecure"] != true {
		t.Fatalf("unexpected trojan tls: %+v", tls)
	}
	transport := outbound["transport"].(map[string]any)
	if transport["type"] != "grpc" || transport["service_name"] != "gs" {
		t.Fatalf("unexpected trojan transport: %+v", transport)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundVMess(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-vmess", "type": "vmess", "server": "vmess.example.com", "port": 443,
		"uuid": "uuid-2", "alterId": 4, "cipher": "auto", "tls": true, "servername": "vmess.example.com",
		"network": "ws", "ws-opts": map[string]any{"path": "/path"},
	})
	if err != nil {
		t.Fatalf("convert vmess: %v", err)
	}
	if outbound["type"] != "vmess" || outbound["uuid"] != "uuid-2" || outbound["alter_id"] != 4 || outbound["security"] != "auto" {
		t.Fatalf("unexpected vmess outbound: %+v", outbound)
	}
	if outbound["tls"].(map[string]any)["server_name"] != "vmess.example.com" {
		t.Fatalf("unexpected vmess tls: %+v", outbound["tls"])
	}
	if outbound["transport"].(map[string]any)["type"] != "ws" {
		t.Fatalf("unexpected vmess transport: %+v", outbound["transport"])
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundHysteria2(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-hy2", "type": "hysteria2", "server": "hy2.example.com", "port": 8443,
		"password": "pw", "sni": "hy2.example.com", "skip-cert-verify": true,
		"obfs": "salamander", "obfs-password": "op",
	})
	if err != nil {
		t.Fatalf("convert hysteria2: %v", err)
	}
	if outbound["type"] != "hysteria2" || outbound["password"] != "pw" {
		t.Fatalf("unexpected hysteria2 outbound: %+v", outbound)
	}
	tls := outbound["tls"].(map[string]any)
	if tls["server_name"] != "hy2.example.com" || tls["insecure"] != true {
		t.Fatalf("unexpected hysteria2 tls: %+v", tls)
	}
	obfs := outbound["obfs"].(map[string]any)
	if obfs["type"] != "salamander" || obfs["password"] != "op" {
		t.Fatalf("unexpected hysteria2 obfs: %+v", obfs)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundAnyTLS(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-anytls", "type": "anytls", "server": "any.example.com", "port": 443,
		"password": "pw", "sni": "any.example.com",
	})
	if err != nil {
		t.Fatalf("convert anytls: %v", err)
	}
	if outbound["type"] != "anytls" || outbound["password"] != "pw" {
		t.Fatalf("unexpected anytls outbound: %+v", outbound)
	}
	tls := outbound["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "any.example.com" {
		t.Fatalf("unexpected anytls tls: %+v", tls)
	}
	if _, has := tls["insecure"]; has {
		t.Fatalf("anytls without skip-cert-verify must verify certificates: %+v", tls)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundSocks5(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-socks", "type": "socks5", "server": "socks.example.com", "port": 1080,
		"username": "u", "password": "p",
	})
	if err != nil {
		t.Fatalf("convert socks5: %v", err)
	}
	if outbound["type"] != "socks" || outbound["server"] != "socks.example.com" || outbound["server_port"] != 1080 ||
		outbound["username"] != "u" || outbound["password"] != "p" {
		t.Fatalf("unexpected socks outbound: %+v", outbound)
	}
	validateOutbound(t, outbound)
}

func TestProxyToOutboundHTTP(t *testing.T) {
	outbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-http", "type": "http", "server": "http.example.com", "port": 8080,
		"username": "u", "password": "p",
	})
	if err != nil {
		t.Fatalf("convert plain http: %v", err)
	}
	if outbound["type"] != "http" || outbound["server"] != "http.example.com" || outbound["server_port"] != 8080 ||
		outbound["username"] != "u" || outbound["password"] != "p" {
		t.Fatalf("unexpected http outbound: %+v", outbound)
	}
	if _, has := outbound["tls"]; has {
		t.Fatalf("plain http proxy must not attach tls: %+v", outbound)
	}
	validateOutbound(t, outbound)

	tlsOutbound, err := singbox.ProxyToOutbound(map[string]any{
		"name": "ext-https", "type": "http", "server": "http.example.com", "port": 8443,
		"username": "u", "password": "p", "tls": true, "sni": "http.example.com", "skip-cert-verify": true,
	})
	if err != nil {
		t.Fatalf("convert tls http: %v", err)
	}
	tls := tlsOutbound["tls"].(map[string]any)
	if tls["enabled"] != true || tls["server_name"] != "http.example.com" || tls["insecure"] != true {
		t.Fatalf("unexpected http tls: %+v", tls)
	}
	validateOutbound(t, tlsOutbound)
}

func TestProxyToOutboundRequiresFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		proxy map[string]any
	}{
		{"unknown type", map[string]any{"type": "wireguard", "server": "a.com", "port": 1}},
		{"missing type", map[string]any{"server": "a.com", "port": 1}},
		{"missing server", map[string]any{"type": "ss", "port": 1, "cipher": "aes-128-gcm", "password": "p"}},
		{"bad port", map[string]any{"type": "ss", "server": "a.com", "port": 0, "cipher": "aes-128-gcm", "password": "p"}},
		{"ss missing password", map[string]any{"type": "ss", "server": "a.com", "port": 1, "cipher": "aes-128-gcm"}},
		{"vless missing uuid", map[string]any{"type": "vless", "server": "a.com", "port": 1}},
		{"trojan missing password", map[string]any{"type": "trojan", "server": "a.com", "port": 1}},
		{"vmess missing uuid", map[string]any{"type": "vmess", "server": "a.com", "port": 1}},
		{"hysteria2 missing password", map[string]any{"type": "hysteria2", "server": "a.com", "port": 1}},
		{"anytls missing password", map[string]any{"type": "anytls", "server": "a.com", "port": 1}},
	} {
		if _, err := singbox.ProxyToOutbound(tc.proxy); err == nil {
			t.Fatalf("%s must return an error", tc.name)
		}
	}
}

func TestOutboundSupported(t *testing.T) {
	for _, clashType := range []string{"ss", "vless", "trojan", "vmess", "hysteria2", "anytls", "socks5", "http"} {
		if !singbox.OutboundSupported(clashType) {
			t.Fatalf("%s must be supported", clashType)
		}
	}
	for _, clashType := range []string{"", "snell", "wireguard", "ssh", "socks"} {
		if singbox.OutboundSupported(clashType) {
			t.Fatalf("%s must be unsupported", clashType)
		}
	}
}
