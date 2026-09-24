package subscription

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"vps-node/internal/singbox"
)

func testNode(t *testing.T, protocol string) Node {
	t.Helper()
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return Node{ID: 1, Name: "node one", Protocol: protocol, Address: "2001:db8::1", Port: 443,
		Settings: map[string]any{
			"cipher":           "2022-blake3-aes-128-gcm",
			"reality_settings": map[string]any{"server_name": "example.com", "short_id": "0123abcd"},
			"tls":              map[string]any{"server_name": "example.com"},
		},
		Secret: map[string]any{"private_key": base64.RawURLEncoding.EncodeToString(key.Bytes())}}
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

func renderSS(t *testing.T, appKey []byte, node Node) (string, map[string]any) {
	t.Helper()
	encoded, err := RenderGeneral(appKey, "user-uuid", []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	link := string(raw)
	if !strings.HasPrefix(link, "ss://") {
		t.Fatalf("unexpected ss link: %q", link)
	}
	rest := strings.TrimPrefix(link, "ss://")
	at := strings.Index(rest, "@")
	if at < 0 {
		t.Fatalf("ss link has no credential separator: %q", link)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(rest[:at])
	if err != nil {
		t.Fatal(err)
	}

	template := "proxy-groups:\n  - {name: all, type: select, proxies: [__ALL_PROXIES__]}\n"
	out, err := RenderClash(appKey, "user-uuid", template, []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxy := config["proxies"].([]any)[0].(map[string]any)
	return string(decoded), proxy
}

func TestRenderShadowsocks2022UsesCombinedClientPassword(t *testing.T) {
	appKey := make([]byte, 32)
	cipher := "2022-blake3-aes-128-gcm"
	node := testNode(t, "shadowsocks")

	userKey, err := singbox.DeriveSSPassword(appKey, node.ID, "user-uuid", cipher)
	if err != nil {
		t.Fatal(err)
	}
	serverKey, err := singbox.DeriveSSServerPassword(appKey, node.ID, cipher)
	if err != nil {
		t.Fatal(err)
	}
	want := serverKey + ":" + userKey

	credential, proxy := renderSS(t, appKey, node)
	if credential != cipher+":"+want {
		t.Fatalf("ss URI credential = %q, want %q", credential, cipher+":"+want)
	}
	if proxy["password"] != want {
		t.Fatalf("clash ss password = %v, want %q", proxy["password"], want)
	}
}

func TestRenderShadowsocks2022UsesCustomServerPassword(t *testing.T) {
	appKey := make([]byte, 32)
	cipher := "2022-blake3-chacha20-poly1305"
	node := testNode(t, "shadowsocks")
	node.Settings["cipher"] = cipher
	node.Secret = map[string]any{"password": "custom-server-key"}

	userKey, err := singbox.DeriveSSPassword(appKey, node.ID, "user-uuid", cipher)
	if err != nil {
		t.Fatal(err)
	}
	want := "custom-server-key:" + userKey

	credential, proxy := renderSS(t, appKey, node)
	if credential != cipher+":"+want {
		t.Fatalf("ss URI credential = %q, want %q", credential, cipher+":"+want)
	}
	if proxy["cipher"] != cipher || proxy["password"] != want {
		t.Fatalf("unexpected clash ss proxy: %+v", proxy)
	}
}

func TestCombineSSClientPasswordNon2022UsesUserKeyOnly(t *testing.T) {
	for _, cipher := range []string{"aes-256-gcm", "chacha20-poly1305"} {
		if singbox.IsSS2022(cipher) {
			t.Fatalf("%q must not be treated as a 2022 cipher", cipher)
		}
		if got := combineSSClientPassword(cipher, "server-key", "user-key"); got != "user-key" {
			t.Fatalf("non-2022 cipher %q password = %q, want user key only", cipher, got)
		}
	}
	if !singbox.IsSS2022("2022-blake3-aes-128-gcm") {
		t.Fatal("2022 method must be detected as SS2022")
	}
	if got := combineSSClientPassword("2022-blake3-aes-128-gcm", "server-key", "user-key"); got != "server-key:user-key" {
		t.Fatalf("2022 cipher password = %q, want server:user", got)
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
	broken.Settings["tls"] = map[string]any{"server_name": ""}
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
	templateWithProxies := "proxies:\n  - {name: stale, type: ss, server: example.com, port: 1, cipher: none, password: x}\nproxy-groups:\n  - {name: x, type: select, proxies: [__ALL_PROXIES__]}\n"
	if err := ValidateTemplate(templateWithProxies); err != nil {
		t.Fatalf("rejected ignored proxies field: %v", err)
	}
	outWithProxies, err := RenderClash(make([]byte, 32), "uuid", templateWithProxies, nodes)
	if err != nil {
		t.Fatal(err)
	}
	var configWithProxies map[string]any
	if err := yaml.Unmarshal(outWithProxies, &configWithProxies); err != nil {
		t.Fatal(err)
	}
	if renderedProxies := configWithProxies["proxies"].([]any); len(renderedProxies) != 3 {
		t.Fatalf("expected generated proxies to replace template proxies, got %v", renderedProxies)
	}
	if err := ValidateTemplate("proxy-groups:\n  - name: x\n    type: select\n    proxies: [STATIC]\n"); err == nil {
		t.Fatal("accepted dangling target")
	}
	if err := ValidateTemplate("bind-address: 127.0.0.1\nallow-lan: true\nproxy-groups:\n  - {name: x, type: select, proxies: [DIRECT]}\n"); err != nil {
		t.Fatalf("rejected bind-address: %v", err)
	}
	for _, field := range []string{"url", "path", "header"} {
		template := "dns:\n  nested:\n    " + field + ": bad\nproxy-groups:\n  - {name: x, type: select, proxies: [DIRECT]}\n"
		if err := ValidateTemplate(template); err == nil {
			t.Fatalf("accepted forbidden nested field %s", field)
		}
	}
	tunTemplate := "tun:\n  enable: true\n  stack: gvisor\nproxy-groups:\n  - {name: x, type: select, proxies: [DIRECT]}\n"
	keptTun, err := RenderClash(make([]byte, 32), "uuid", tunTemplate, nodes)
	if err != nil {
		t.Fatalf("rejected tun: %v", err)
	}
	if !strings.Contains(string(keptTun), "tun:") {
		t.Fatal("tun config dropped from output")
	}
	for _, field := range []string{"proxy-providers", "listeners", "external-controller", "external-controller-tls", "secret", "authentication", "skip-auth-prefixes", "script"} {
		template := field + ": bad\nproxy-groups:\n  - {name: x, type: select, proxies: [DIRECT]}\n"
		if err := ValidateTemplate(template); err != nil {
			t.Fatalf("rejected stripped field %s: %v", field, err)
		}
		stripped, err := RenderClash(make([]byte, 32), "uuid", template, nodes)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(stripped), field) {
			t.Fatalf("stripped field %s leaked into output", field)
		}
	}
	if strings.Contains(string(out), "private_key") || strings.Contains(string(out), nodes[1].Secret["private_key"].(string)) {
		t.Fatal("private key leaked")
	}
}

func TestDefaultClashMetaTemplate(t *testing.T) {
	if err := ValidateTemplate(DefaultClashMetaTemplate); err != nil {
		t.Fatalf("default template rejected: %v", err)
	}
	nodes := []Node{testNode(t, "shadowsocks"), testNode(t, "hysteria2")}
	nodes[1].ID = 2
	nodes[1].Name = "node two"
	out, err := RenderClash(make([]byte, 32), "user-uuid", DefaultClashMetaTemplate, nodes)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	if config["tun"] == nil {
		t.Fatal("tun config missing from rendered default template")
	}
	if _, ok := config["external-controller"]; ok {
		t.Fatal("external-controller leaked into rendered default template")
	}
	if proxies := config["proxies"].([]any); len(proxies) != 2 {
		t.Fatalf("expected 2 generated proxies, got %v", proxies)
	}
	groups := config["proxy-groups"].([]any)
	autoSelect, ok := groups[0].(map[string]any)
	if !ok || autoSelect["name"] != "AutoSelect" {
		t.Fatalf("unexpected first group: %v", groups[0])
	}
	if items := autoSelect["proxies"].([]any); len(items) != 2 {
		t.Fatalf("expected AutoSelect to expand all proxies, got %v", items)
	}
}

func TestRenderHysteria2ObfsAndHopPorts(t *testing.T) {
	key := make([]byte, 32)
	node := testNode(t, "hysteria2")
	node.Settings["obfs"] = map[string]any{"open": true, "type": "salamander", "password": "obfs-secret"}
	node.Settings["hop_interval"] = "30000-40000"

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

func TestRenderIPv6ExtraEntry(t *testing.T) {
	appKey := make([]byte, 32)
	node := testNode(t, "shadowsocks")
	node.Address = "hk01.example.com"
	node.IPv6Address = "2001:db8::1"

	out, err := RenderClash(appKey, "user-uuid", DefaultClashMetaTemplate, []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	proxies := config["proxies"].([]any)
	if len(proxies) != 2 {
		t.Fatalf("expected a primary and an IPv6 proxy, got %v", proxies)
	}
	primary := proxies[0].(map[string]any)
	extra := proxies[1].(map[string]any)
	if primary["server"] != "hk01.example.com" || extra["server"] != "2001:db8::1" {
		t.Fatalf("unexpected servers: %v / %v", primary["server"], extra["server"])
	}
	if extra["name"] != "node one-v6" || extra["port"] != primary["port"] || extra["password"] != primary["password"] {
		t.Fatalf("IPv6 proxy must differ only by host and name: %v", extra)
	}

	encoded, err := RenderGeneral(appKey, "user-uuid", []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Count(text, "\n") != 1 || !strings.Contains(text, "hk01.example.com:443") || !strings.Contains(text, "[2001:db8::1]:443") {
		t.Fatalf("expected two share links for both families, got %q", text)
	}

	node.IPv6Address = "hk01.example.com"
	out, err = RenderClash(appKey, "user-uuid", DefaultClashMetaTemplate, []Node{node})
	if err != nil {
		t.Fatal(err)
	}
	config = map[string]any{}
	if err := yaml.Unmarshal(out, &config); err != nil {
		t.Fatal(err)
	}
	if got := config["proxies"].([]any); len(got) != 1 {
		t.Fatalf("matching IPv6 address must not duplicate the entry, got %v", got)
	}
}
