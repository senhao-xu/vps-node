package subscription

import (
	"encoding/base64"
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
	encoded := RenderGeneralLinksMerged(appKey, "user-uuid", []Node{node}, custom, nil)
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
	out, err := RenderClashFilteredMerged(appKey, "user-uuid", customTestTemplate, []Node{node}, custom, nil, onSkipCustom)
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
	legacy, err := RenderClashFiltered(appKey, "user-uuid", customTestTemplate, nodes, nil)
	if err != nil {
		t.Fatal(err)
	}
	merged, err := RenderClashFilteredMerged(appKey, "user-uuid", customTestTemplate, nodes, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(legacy) != string(merged) {
		t.Fatal("empty custom slice must render byte-identical output")
	}
	generalLegacy := RenderGeneralLinks(appKey, "user-uuid", nodes, nil)
	generalMerged := RenderGeneralLinksMerged(appKey, "user-uuid", nodes, nil, nil)
	if generalLegacy != generalMerged {
		t.Fatal("empty custom slice must render byte-identical general output")
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
