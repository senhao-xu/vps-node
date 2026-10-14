package subscription

import (
	"encoding/base64"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const customTestTemplate = "proxy-groups:\n  - {name: all, type: select, proxies: [__ALL_PROXIES__]}\n"

func TestRenderGeneralLinksMergedAppendsCustomVerbatim(t *testing.T) {
	appKey := make([]byte, 32)
	node := testNode(t, "shadowsocks")
	custom := []CustomSource{
		{ID: 1, Name: "ext", Links: []string{"ss://ZXh0ZXJuYWw@ext.example.com:8388#ext", "garbage line"}},
	}
	encoded := RenderGeneralLinksMerged(appKey, User{UUID: "user-uuid"}, []Node{node}, custom, nil)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected managed + 2 custom lines, got %q", raw)
	}
	if lines[1] != "ss://ZXh0ZXJuYWw@ext.example.com:8388#ext" || lines[2] != "garbage line" {
		t.Fatalf("custom lines must pass through verbatim: %q", raw)
	}
}

func TestRenderClashFilteredMergedCustomSources(t *testing.T) {
	appKey := make([]byte, 32)
	node := testNode(t, "shadowsocks") // renders as "node one"
	custom := []CustomSource{
		{ID: 7, Name: "links", Links: []string{
			"ss://aes-128-gcm:secret@ext.example.com:8388#node one",
			"garbage line",
		}},
		{ID: 8, Name: "upstream", Proxies: []map[string]any{
			{"name": "node one 7", "type": "trojan", "server": "up.example.com", "port": 443, "password": "p"},
		}},
	}
	var skipped []string
	onSkipCustom := func(sourceID int64, item string, err error) {
		skipped = append(skipped, item)
	}
	out, err := RenderClashFilteredMerged(appKey, User{UUID: "user-uuid"}, customTestTemplate, []Node{node}, custom, nil, onSkipCustom)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxies := config["proxies"].([]any)
	if len(proxies) != 3 {
		t.Fatalf("expected 3 proxies, got %v", proxies)
	}
	names := []string{}
	for _, p := range proxies {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	// Managed node keeps its name; the colliding custom link gets the
	// source-id suffix; the upstream proxy collides again and gets its own.
	want := []string{"node one", "node one 7", "node one 7 8"}
	for i, name := range want {
		if names[i] != name {
			t.Fatalf("proxy %d name = %q, want %q (all: %v)", i, names[i], name, names)
		}
	}
	customProxy := proxies[1].(map[string]any)
	if customProxy["type"] != "ss" || customProxy["server"] != "ext.example.com" || customProxy["port"] != 8388 {
		t.Fatalf("custom link proxy: %v", customProxy)
	}
	if len(skipped) != 1 || skipped[0] != "garbage line" {
		t.Fatalf("unparseable link must be skipped via onSkipCustom: %v", skipped)
	}
	groups := config["proxy-groups"].([]any)
	group := groups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)
	if len(groupProxies) != 3 {
		t.Fatalf("custom proxies must join __ALL_PROXIES__: %v", groupProxies)
	}
}

func TestRenderClashFilteredWithoutCustomMatchesLegacy(t *testing.T) {
	appKey := make([]byte, 32)
	nodes := []Node{testNode(t, "shadowsocks"), testNode(t, "vless")}
	legacy, err := RenderClashFiltered(appKey, User{UUID: "user-uuid"}, customTestTemplate, nodes, nil)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := RenderClashFilteredMerged(appKey, User{UUID: "user-uuid"}, customTestTemplate, nodes, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacy) != string(merged) {
		t.Fatal("empty custom slice must render byte-identical output")
	}
	generalLegacy := RenderGeneralLinks(appKey, User{UUID: "user-uuid"}, nodes, nil)
	generalMerged := RenderGeneralLinksMerged(appKey, User{UUID: "user-uuid"}, nodes, nil, nil)
	if generalLegacy != generalMerged {
		t.Fatal("empty custom slice must render byte-identical general output")
	}
}

func TestRenderProxiesFragmentFlowStyle(t *testing.T) {
	proxies := []map[string]any{
		{"name": "a", "type": "ss", "server": "x.example.com", "port": 1, "reality-opts": map[string]any{"public-key": "k"}},
		{"name": "b", "type": "vless", "server": "y.example.com", "port": 2},
	}
	out, err := RenderProxiesFragment(proxies)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) != 3 || lines[0] != "proxies:" {
		t.Fatalf("unexpected fragment: %q", out)
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "- {") || !strings.HasSuffix(trimmed, "}") {
			t.Fatalf("each proxy must be a single flow line: %q", out)
		}
	}
	empty, err := RenderProxiesFragment(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "proxies: []\n" {
		t.Fatalf("empty fragment: %q", empty)
	}
}

func TestRenderProxiesFragmentOrdersFields(t *testing.T) {
	proxy := map[string]any{
		"network": "tcp", "client-fingerprint": "safari",
		"reality-opts": map[string]any{"z-extra": true, "short-id": "0963ab", "public-key": "public-key"},
		"servername":   "www.amd.com", "skip-cert-verify": false, "tls": true,
		"encryption": "none", "flow": "xtls-rprx-vision", "udp": true, "cipher": "auto",
		"alterId": 0, "uuid": "uuid", "port": 45561, "server": "2400:c800:109:220::a",
		"type": "vless", "name": "Japan-L", "z-extra": "last", "a-extra": "first",
	}
	out, err := RenderProxiesFragment([]map[string]any{proxy})
	if err != nil {
		t.Fatal(err)
	}
	want := "proxies:\n    - {name: Japan-L, type: vless, server: '2400:c800:109:220::a', port: 45561, uuid: uuid, alterId: 0, cipher: auto, udp: true, flow: xtls-rprx-vision, encryption: none, tls: true, skip-cert-verify: false, servername: www.amd.com, reality-opts: {public-key: public-key, short-id: 0963ab, z-extra: true}, client-fingerprint: safari, network: tcp, a-extra: first, z-extra: last}\n"
	if string(out) != want {
		t.Fatalf("ordered fragment = %q, want %q", out, want)
	}

	var decoded struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(out, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Proxies) != 1 || !reflect.DeepEqual(decoded.Proxies[0], proxy) {
		t.Fatalf("round trip changed proxy: got %#v, want %#v", decoded.Proxies, proxy)
	}
}

func TestRenderProxiesFragmentSkipsMissingPreferredFields(t *testing.T) {
	proxy := map[string]any{
		"network":  "ws",
		"server":   "example.com",
		"name":     "sparse",
		"password": "secret",
	}
	out, err := RenderProxiesFragment([]map[string]any{proxy})
	if err != nil {
		t.Fatal(err)
	}
	want := "proxies:\n    - {name: sparse, server: example.com, network: ws, password: secret}\n"
	if string(out) != want {
		t.Fatalf("sparse fragment = %q, want %q", out, want)
	}
}

func TestUniqueProxyName(t *testing.T) {
	used := map[string]bool{"a": true, "a 9": true}
	if got := uniqueProxyName(used, "b", 9); got != "b" {
		t.Fatalf("fresh name: %q", got)
	}
	if got := uniqueProxyName(used, "a", 9); got != "a 9-2" {
		t.Fatalf("second collision: %q", got)
	}
	if got := uniqueProxyName(used, "a", 9); got != "a 9-3" {
		t.Fatalf("third collision: %q", got)
	}
}
