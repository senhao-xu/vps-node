package subscription

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func testNode(t *testing.T, protocol string) Node {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Node{ID: 1, Name: "node one", Protocol: protocol, Address: "2001:db8::1", Port: 443,
		Settings: map[string]any{"method": "2022-blake3-aes-128-gcm", "server_names": []any{"example.com"}, "server_name": "example.com", "short_id": "0123abcd"},
		Secret:   map[string]any{"private_key": base64.RawURLEncoding.EncodeToString(key.Bytes())}}
}

func TestRenderGeneralIncludesAllProtocolsWithoutPrivateKey(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	nodes := []Node{testNode(t, "shadowsocks"), testNode(t, "vless"), testNode(t, "hysteria2"), testNode(t, "anytls")}
	nodes[1].ID, nodes[2].ID, nodes[3].ID = 2, 3, 4
	encoded, err := RenderGeneral(key, "user-uuid", nodes)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, prefix := range []string{"ss://", "vless://", "hysteria2://", "anytls://"} {
		if !strings.Contains(text, prefix) {
			t.Fatalf("missing %s in %q", prefix, text)
		}
	}
	if strings.Contains(text, "private_key") {
		t.Fatal("private key leaked")
	}
}

func TestRenderAnyTLSLinkAndProxy(t *testing.T) {
	key := make([]byte, 32)
	node := testNode(t, "anytls")

	encoded, err := RenderGeneral(key, "user-uuid", []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	link := string(raw)
	if !strings.HasPrefix(link, "anytls://user-uuid@[2001:db8::1]:443?sni=example.com#") {
		t.Fatalf("unexpected anytls link: %q", link)
	}

	template := "proxy-groups:\n  - {name: anytls, type: select, proxies: [__ANYTLS_PROXIES__]}\n"
	out, err := RenderClash(key, "user-uuid", template, []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxy := config["proxies"].([]any)[0].(map[string]any)
	if proxy["type"] != "anytls" || proxy["password"] != "user-uuid" || proxy["sni"] != "example.com" ||
		proxy["skip-cert-verify"] != false || proxy["udp"] != true {
		t.Fatalf("unexpected anytls clash proxy: %+v", proxy)
	}
	groups := config["proxy-groups"].([]any)
	expanded := groups[0].(map[string]any)["proxies"].([]any)
	if len(expanded) != 1 || expanded[0].(string) != "node one" {
		t.Fatalf("__ANYTLS_PROXIES__ must expand to the anytls node, got %v", expanded)
	}

	broken := testNode(t, "anytls")
	broken.Settings["server_name"] = ""
	if _, err := RenderGeneral(key, "user-uuid", []Node{broken}); err == nil {
		t.Fatal("anytls node without server_name must fail to render")
	}
}

func TestRenderClashPlaceholdersAndRestrictions(t *testing.T) {
	nodes := []Node{testNode(t, "shadowsocks"), testNode(t, "vless"), testNode(t, "hysteria2")}
	nodes[0].Name, nodes[1].Name, nodes[2].Name = "ss-node", "vless-node", "hy2-node"
	nodes[1].ID, nodes[2].ID = 2, 3
	template := `proxy-groups:
  - {name: all, type: select, proxies: [__ALL_PROXIES__]}
  - {name: ss, type: select, proxies: [__SHADOWSOCKS_PROXIES__]}
  - {name: vless, type: select, proxies: [__VLESS_PROXIES__]}
  - {name: hy2, type: select, proxies: [__HYSTERIA2_PROXIES__]}
  - {name: static, type: select, proxies: [DIRECT]}
`
	out, err := RenderClash(make([]byte, 32), "uuid", template, nodes)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	groups := config["proxy-groups"].([]any)
	want := map[string][]string{"all": {"ss-node", "vless-node", "hy2-node"}, "ss": {"ss-node"}, "vless": {"vless-node"}, "hy2": {"hy2-node"}, "static": {"DIRECT"}}
	for _, raw := range groups {
		group := raw.(map[string]any)
		items := group["proxies"].([]any)
		got := make([]string, len(items))
		for i := range items {
			got[i] = items[i].(string)
		}
		if strings.Join(got, ",") != strings.Join(want[group["name"].(string)], ",") {
			t.Fatalf("group %v got %v", group["name"], got)
		}
	}
	if err := ValidateTemplate("proxy-groups:\n  - name: x\n    type: select\n    proxies: [__ALL_PROXIES__]\nproxies: []\n"); err == nil {
		t.Fatal("accepted static proxies")
	}
	if err := ValidateTemplate("proxy-groups:\n  - name: x\n    type: select\n    proxies: [STATIC]\n"); err == nil {
		t.Fatal("accepted dangling target")
	}
	for _, field := range []string{"url", "path", "header", "external-controller", "listeners", "authentication", "script", "proxy-providers"} {
		template := "dns:\n  nested:\n    " + field + ": bad\nproxy-groups:\n  - {name: x, type: select, proxies: [DIRECT]}\n"
		if field == "external-controller" || field == "listeners" || field == "authentication" || field == "script" || field == "proxy-providers" {
			template = field + ": bad\nproxy-groups:\n  - {name: x, type: select, proxies: [DIRECT]}\n"
		}
		if err := ValidateTemplate(template); err == nil {
			t.Fatalf("accepted forbidden field %s", field)
		}
	}
	if strings.Contains(string(out), "private_key") || strings.Contains(string(out), nodes[1].Secret["private_key"].(string)) {
		t.Fatal("private key leaked")
	}
}

func TestRenderHysteria2ObfsAndHopPorts(t *testing.T) {
	key := make([]byte, 32)
	node := testNode(t, "hysteria2")
	node.Settings["obfs_password"] = "obfs-secret"
	node.Settings["hop_ports"] = "30000-40000"

	encoded, err := RenderGeneral(key, "user-uuid", []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	link := string(raw)
	for _, want := range []string{"obfs=salamander", "obfs-password=obfs-secret", "mport=30000-40000"} {
		if !strings.Contains(link, want) {
			t.Fatalf("hy2 link must contain %q: %s", want, link)
		}
	}

	template := "proxy-groups:\n  - {name: all, type: select, proxies: [__ALL_PROXIES__]}\n"
	out, err := RenderClash(key, "user-uuid", template, []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxy := config["proxies"].([]any)[0].(map[string]any)
	if proxy["obfs"] != "salamander" || proxy["obfs-password"] != "obfs-secret" || proxy["ports"] != "30000-40000" {
		t.Fatalf("unexpected hy2 clash proxy: %+v", proxy)
	}

	// Legacy nodes without the new keys render exactly as before.
	legacy := testNode(t, "hysteria2")
	encoded, err = RenderGeneral(key, "user-uuid", []Node{legacy})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = base64.StdEncoding.DecodeString(encoded)
	link = string(raw)
	for _, absent := range []string{"obfs", "mport"} {
		if strings.Contains(link, absent) {
			t.Fatalf("legacy hy2 link must not contain %q: %s", absent, link)
		}
	}
	out, err = RenderClash(key, "user-uuid", template, []Node{legacy})
	if err != nil {
		t.Fatal(err)
	}
	config = map[string]any{}
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxy = config["proxies"].([]any)[0].(map[string]any)
	for _, absent := range []string{"obfs", "obfs-password", "ports"} {
		if _, has := proxy[absent]; has {
			t.Fatalf("legacy hy2 clash proxy must not contain %q: %+v", absent, proxy)
		}
	}
}

func TestRenderFilteredSkipsBrokenNodes(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	good := testNode(t, "vless")
	broken := testNode(t, "vless")
	broken.ID = 7
	broken.Name = "broken"
	broken.Secret["private_key"] = "aaaa"

	var skipped []int64
	text := RenderGeneralLinks(key, "uuid", []Node{broken, good}, func(n Node, err error) {
		skipped = append(skipped, n.ID)
	})
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "vless://") || strings.Contains(string(raw), "broken") {
		t.Fatalf("expected only the healthy node in %q", string(raw))
	}
	if len(skipped) != 1 || skipped[0] != 7 {
		t.Fatalf("expected node 7 to be skipped, got %v", skipped)
	}

	template := "proxy-groups:\n  - {name: all, type: select, proxies: [__ALL_PROXIES__]}\n"
	out, err := RenderClashFiltered(key, "uuid", template, []Node{broken, good}, func(n Node, err error) {
		skipped = append(skipped, n.ID)
	})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxies := config["proxies"].([]any)
	if len(proxies) != 1 || proxies[0].(map[string]any)["name"] != "node one" {
		t.Fatalf("expected only the healthy proxy, got %v", proxies)
	}
	if len(skipped) != 2 {
		t.Fatalf("expected node 7 skipped twice in total, got %v", skipped)
	}
}
