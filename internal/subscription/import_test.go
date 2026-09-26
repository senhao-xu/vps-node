package subscription

import (
	"encoding/base64"
	"fmt"
	"testing"
)

func TestParseShareURI(t *testing.T) {
	ssUserinfo := base64.RawURLEncoding.EncodeToString([]byte("aes-128-gcm:pass word"))
	ssLegacy := base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:secret@1.2.3.4:8388"))
	vmessDoc := base64.StdEncoding.EncodeToString([]byte(
		`{"ps":"vmess 节点","add":"vm.example.com","port":"443","id":"uuid-1","aid":"0","scy":"zero","net":"ws","host":"cdn.example.com","path":"/ws","tls":"tls","sni":"sni.example.com"}`))

	cases := []struct {
		name string
		raw  string
		want map[string]any
	}{
		{
			name: "ss sip002 base64 userinfo",
			raw:  "ss://" + ssUserinfo + "@hk.example.com:443#香港-01",
			want: map[string]any{"name": "香港-01", "type": "ss", "server": "hk.example.com", "port": 443, "cipher": "aes-128-gcm", "password": "pass word", "udp": true},
		},
		{
			name: "ss plain userinfo",
			raw:  "ss://aes-256-gcm:secret@1.2.3.4:8388#plain",
			want: map[string]any{"name": "plain", "type": "ss", "server": "1.2.3.4", "port": 8388, "cipher": "aes-256-gcm", "password": "secret", "udp": true},
		},
		{
			name: "ss legacy fully encoded",
			raw:  "ss://" + ssLegacy + "#legacy",
			want: map[string]any{"name": "legacy", "type": "ss", "server": "1.2.3.4", "port": 8388, "cipher": "chacha20-ietf-poly1305", "password": "secret", "udp": true},
		},
		{
			name: "vless reality",
			raw:  "vless://uuid-9@vl.example.com:443?security=reality&sni=www.example.com&pbk=PUBKEY&sid=0123abcd&flow=xtls-rprx-vision&type=tcp&fp=chrome#vl-节点",
			want: map[string]any{"name": "vl-节点", "type": "vless", "server": "vl.example.com", "port": 443, "uuid": "uuid-9", "network": "tcp", "tls": true, "servername": "www.example.com", "flow": "xtls-rprx-vision", "client-fingerprint": "chrome", "reality-opts": map[string]any{"public-key": "PUBKEY", "short-id": "0123abcd"}, "udp": true},
		},
		{
			name: "vless plain tcp",
			raw:  "vless://uuid-1@10.0.0.1:80?security=none&type=tcp#vless-plain",
			want: map[string]any{"name": "vless-plain", "type": "vless", "server": "10.0.0.1", "port": 80, "uuid": "uuid-1", "network": "tcp", "tls": false, "udp": true},
		},
		{
			name: "hysteria2 with obfs",
			raw:  "hysteria2://passw0rd@hy.example.com:8443?sni=hy.example.com&obfs=salamander&obfs-password=obfs-pass&mport=30000-31000#hy2",
			want: map[string]any{"name": "hy2", "type": "hysteria2", "server": "hy.example.com", "port": 8443, "password": "passw0rd", "sni": "hy.example.com", "obfs": "salamander", "obfs-password": "obfs-pass", "ports": "30000-31000"},
		},
		{
			name: "hy2 alias with insecure",
			raw:  "hy2://passw0rd@hy.example.com:443?sni=hy.example.com&insecure=1",
			want: map[string]any{"name": "hy.example.com:443", "type": "hysteria2", "server": "hy.example.com", "port": 443, "password": "passw0rd", "sni": "hy.example.com", "skip-cert-verify": true},
		},
		{
			name: "anytls",
			raw:  "anytls://secret-pass@at.example.com:443?sni=at.example.com#any",
			want: map[string]any{"name": "any", "type": "anytls", "server": "at.example.com", "port": 443, "password": "secret-pass", "sni": "at.example.com", "udp": true},
		},
		{
			name: "trojan",
			raw:  "trojan://p%40ss@tj.example.com:443?sni=tj.example.com&allowInsecure=1#tr",
			want: map[string]any{"name": "tr", "type": "trojan", "server": "tj.example.com", "port": 443, "password": "p@ss", "sni": "tj.example.com", "skip-cert-verify": true, "network": "tcp", "udp": true},
		},
		{
			name: "vmess websocket tls",
			raw:  "vmess://" + vmessDoc,
			want: map[string]any{"name": "vmess 节点", "type": "vmess", "server": "vm.example.com", "port": 443, "uuid": "uuid-1", "alterId": 0, "cipher": "zero", "network": "ws", "tls": true, "servername": "sni.example.com", "udp": true, "ws-opts": map[string]any{"path": "/ws", "headers": map[string]any{"Host": "cdn.example.com"}}},
		},
		{
			name: "ss ipv6 host",
			raw:  "ss://aes-128-gcm:secret@[2001:db8::1]:443#v6",
			want: map[string]any{"name": "v6", "type": "ss", "server": "2001:db8::1", "port": 443, "cipher": "aes-128-gcm", "password": "secret", "udp": true},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseShareURI(tc.raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			for key, want := range tc.want {
				if fmt.Sprint(got[key]) != fmt.Sprint(want) {
					t.Fatalf("field %s = %v, want %v (proxy %v)", key, got[key], want, got)
				}
			}
			for key := range got {
				if _, ok := tc.want[key]; !ok {
					t.Fatalf("unexpected field %s = %v", key, got[key])
				}
			}
		})
	}
}

func TestParseShareURIRejectsMalformed(t *testing.T) {
	for _, raw := range []string{
		"",
		"   ",
		"no-scheme-at-all",
		"http://example.com",
		"ss://",
		"ss://%%%",
		"ss://aes-128-gcm@1.2.3.4:8388",     // no password separator
		"ss://aes-128-gcm:secret@1.2.3.4",   // missing port
		"ss://aes-128-gcm:secret@1.2.3.4:0", // bad port
		"ss://aes-128-gcm:secret@:8388",     // missing host
		"vless://@host:443#x",               // missing uuid
		"vless://uuid@host:notaport",        // bad port
		"hysteria2://@hy.example.com:443",   // missing password
		"anytls://at.example.com:443",       // missing password
		"trojan://tj.example.com:443",       // missing password
		"vmess://not-base64!!!",
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"add":"","id":""}`)),
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"add":"a","id":"b","port":0}`)),
		"vmess://" + base64.StdEncoding.EncodeToString([]byte(`{"add":"a","id":"b","port":443,"net":"kcp"}`)),
	} {
		t.Run(raw, func(t *testing.T) {
			if _, err := ParseShareURI(raw); err == nil {
				t.Fatalf("expected error for %q", raw)
			}
		})
	}
}
