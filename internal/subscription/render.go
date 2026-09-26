package subscription

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
	"vps-node/internal/singbox"
)

const DefaultClashMetaTemplate = `mixed-port: 7890
allow-lan: true
bind-address: "*"
mode: rule
log-level: info
ipv6: false
unified-delay: true
tcp-concurrent: true
find-process-mode: strict
dns:
  enable: true
  listen: 0.0.0.0:1053
  ipv6: false
  prefer-h3: false
  respect-rules: true
  use-hosts: true
  use-system-hosts: true
  cache-algorithm: arc
  cache-size: 8192
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter:
    - geosite:private
    - "*.lan"
    - "*.qq.com"
    - "*.qpic.cn"
    - "*.qlogo.cn"
    - "*.weixin.qq.com"
    - "*.wechat.com"
    - "*.push.apple.com"
    - "*.apple.com"
    - "*.icloud.com"
    - "*.mzstatic.com"
    - www.msftconnecttest.com
    - www.msftncsi.com
    - dns.msftncsi.com
    - "*.msftconnecttest.com"
    - "*.msftncsi.com"
    - "*.windows.com"
    - "*.onedrive.live.com"
  default-nameserver:
    - 223.5.5.5
    - 119.29.29.29
  nameserver:
    - https://cloudflare-dns.com/dns-query#Proxy
    - https://dns.google/dns-query#Proxy
  proxy-server-nameserver:
    - https://223.5.5.5/dns-query
    - https://doh.pub/dns-query
  direct-nameserver:
    - https://223.5.5.5/dns-query
    - https://doh.pub/dns-query
  direct-nameserver-follow-policy: true
  nameserver-policy:
    geosite:cn:
      - https://223.5.5.5/dns-query
      - https://doh.pub/dns-query
    geosite:tencent:
      - https://223.5.5.5/dns-query
      - https://doh.pub/dns-query
    geosite:apple-cn:
      - https://223.5.5.5/dns-query
    geosite:microsoft@cn:
      - https://223.5.5.5/dns-query
    geosite:onedrive:
      - https://223.5.5.5/dns-query
  fallback:
    - https://cloudflare-dns.com/dns-query#Proxy
    - https://dns.google/dns-query#Proxy
  fallback-filter:
    geoip: true
    geoip-code: CN
tun:
  enable: true
  stack: gvisor
  auto-route: true
  auto-detect-interface: true
  strict-route: true
  dns-hijack:
    - any:53
  mtu: 1500
proxy-groups:
  - name: AutoSelect
    type: url-test
    url: http://www.gstatic.com/generate_204
    interval: 300
    proxies:
      - __ALL_PROXIES__
  - name: Proxy
    type: select
    proxies:
      - AutoSelect
      - __ALL_PROXIES__
  - name: OpenAi
    type: select
    proxies:
      - __ALL_PROXIES__
  - name: Others
    type: select
    proxies:
      - Proxy
      - DIRECT
rules:
  - GEOSITE,private,DIRECT
  - GEOIP,private,DIRECT,no-resolve
  - GEOIP,lan,DIRECT,no-resolve
  - PROCESS-NAME,Weixin.exe,DIRECT
  - PROCESS-NAME,WeChat.exe,DIRECT
  - GEOSITE,category-ads-all,REJECT
  - GEOSITE,tencent,DIRECT
  - GEOSITE,openai,OpenAi
  - GEOSITE,anthropic,OpenAi
  - GEOSITE,cursor,OpenAi
  - DOMAIN-SUFFIX,models.dev,OpenAi
  - GEOSITE,huggingface,Proxy
  - GEOSITE,category-ai-!cn,OpenAi
  - GEOSITE,category-scholar-!cn,Proxy
  - GEOSITE,google,Proxy
  - GEOSITE,youtube,Proxy
  - GEOSITE,github,Proxy
  - GEOSITE,twitter,Proxy
  - GEOSITE,pixiv,Proxy
  - GEOSITE,discord,Proxy
  - GEOSITE,wallhaven,Proxy
  - GEOSITE,onedrive,DIRECT
  - GEOSITE,microsoft,DIRECT
  - GEOSITE,apple-cn,DIRECT
  - GEOSITE,steam@cn,DIRECT
  - GEOSITE,category-games@cn,DIRECT
  - GEOSITE,geolocation-cn,DIRECT
  - DOMAIN-SUFFIX,ipwho.is,Proxy
  - DOMAIN-SUFFIX,ipapi.co,Proxy
  - DOMAIN-SUFFIX,aloxaf.com,Proxy
  - GEOSITE,geolocation-!cn,Proxy
  - GEOSITE,cn,DIRECT
  - GEOIP,telegram,Proxy,no-resolve
  - GEOIP,google,Proxy,no-resolve
  - GEOIP,CN,DIRECT
  - MATCH,Others
`

type Node struct {
	ID          int64
	Name        string
	Protocol    string
	Address     string
	IPv6Address string
	Port        int
	Settings    map[string]any
	Secret      map[string]any
}

// expandIPv6 returns the render targets for a node: the node itself plus, when
// an IPv6 address is configured, a second entry differing only in the
// client-facing host and name. Both entries share the sing-box inbound, so
// traffic and logs stay attributed to the same node.
func expandIPv6(n Node) []Node {
	if n.IPv6Address == "" || n.IPv6Address == n.Address {
		return []Node{n}
	}
	v6 := n
	v6.Address = n.IPv6Address
	v6.Name = n.Name + "-v6"
	v6.IPv6Address = ""
	return []Node{n, v6}
}

func expandNodes(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, expandIPv6(n)...)
	}
	return out
}

func RenderGeneral(appKey []byte, userUUID string, nodes []Node) (string, error) {
	links := make([]string, 0, len(nodes))
	for _, node := range expandNodes(nodes) {
		link, err := renderURI(appKey, userUUID, node)
		if err != nil {
			return "", err
		}
		links = append(links, link)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))), nil
}

// CustomSource is one administrator-maintained external node source merged
// into subscription output after the managed nodes. Links are share URIs
// (one per entry); Proxies are Clash proxy mappings lifted from an upstream
// Clash subscription.
type CustomSource struct {
	ID      int64
	Name    string
	Links   []string
	Proxies []map[string]any
}

// RenderGeneralLinks renders each node's share link, skipping nodes that
// cannot be rendered (reported via onSkip) so a single broken node does not
// break the whole subscription.
func RenderGeneralLinks(appKey []byte, userUUID string, nodes []Node, onSkip func(Node, error)) string {
	return RenderGeneralLinksMerged(appKey, userUUID, nodes, nil, onSkip)
}

// RenderGeneralLinksMerged behaves like RenderGeneralLinks and then appends
// the custom sources' share links verbatim (including lines that would not
// parse — general clients ignore them).
func RenderGeneralLinksMerged(appKey []byte, userUUID string, nodes []Node, custom []CustomSource, onSkip func(Node, error)) string {
	links := make([]string, 0, len(nodes))
	for _, node := range expandNodes(nodes) {
		link, err := renderURI(appKey, userUUID, node)
		if err != nil {
			if onSkip != nil {
				onSkip(node, err)
			}
			continue
		}
		links = append(links, link)
	}
	for _, source := range custom {
		links = append(links, source.Links...)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

// RenderClashFiltered assembles the Clash Meta config from the renderable
// nodes, skipping broken nodes (reported via onSkip).
func RenderClashFiltered(appKey []byte, userUUID, template string, nodes []Node, onSkip func(Node, error)) ([]byte, error) {
	return RenderClashFilteredMerged(appKey, userUUID, template, nodes, nil, onSkip, nil)
}

// RenderClashFilteredMerged behaves like RenderClashFiltered and then merges
// custom sources: share links are parsed into Clash proxies (unparseable
// lines are skipped and reported via onSkipCustom), upstream proxies are
// appended as-is. Custom proxy names colliding with an earlier name are
// suffixed with the source id so Clash proxy names stay unique. Custom
// proxies join the __ALL_PROXIES__ group only.
func RenderClashFilteredMerged(appKey []byte, userUUID, template string, nodes []Node, custom []CustomSource, onSkip func(Node, error), onSkipCustom func(sourceID int64, item string, err error)) ([]byte, error) {
	proxies := make([]map[string]any, 0, len(nodes))
	names := map[string][]string{"all": {}}
	usedNames := map[string]bool{}
	for _, n := range expandNodes(nodes) {
		proxy, err := renderProxy(appKey, userUUID, n)
		if err != nil {
			if onSkip != nil {
				onSkip(n, err)
			}
			continue
		}
		proxies = append(proxies, proxy)
		names["all"] = append(names["all"], n.Name)
		names[n.Protocol] = append(names[n.Protocol], n.Name)
		usedNames[n.Name] = true
	}
	for _, source := range custom {
		for _, link := range source.Links {
			proxy, err := ParseShareURI(link)
			if err != nil {
				if onSkipCustom != nil {
					onSkipCustom(source.ID, link, err)
				}
				continue
			}
			name, _ := proxy["name"].(string)
			name = uniqueProxyName(usedNames, name, source.ID)
			proxy["name"] = name
			proxies = append(proxies, proxy)
			names["all"] = append(names["all"], name)
		}
		for _, proxy := range source.Proxies {
			name, _ := proxy["name"].(string)
			if name == "" {
				if onSkipCustom != nil {
					onSkipCustom(source.ID, "", fmt.Errorf("upstream proxy without a name"))
				}
				continue
			}
			name = uniqueProxyName(usedNames, name, source.ID)
			proxy["name"] = name
			proxies = append(proxies, proxy)
			names["all"] = append(names["all"], name)
		}
	}
	return assembleClash(template, proxies, names)
}

// uniqueProxyName returns name, or "<name> <sourceID>" (with a numeric tie
// breaker) when name is already taken.
func uniqueProxyName(used map[string]bool, name string, sourceID int64) string {
	if !used[name] {
		used[name] = true
		return name
	}
	candidate := name + " " + strconv.FormatInt(sourceID, 10)
	for i := 2; used[candidate]; i++ {
		candidate = name + " " + strconv.FormatInt(sourceID, 10) + "-" + strconv.Itoa(i)
	}
	used[candidate] = true
	return candidate
}

func renderURI(appKey []byte, userUUID string, n Node) (string, error) {
	host := net.JoinHostPort(n.Address, strconv.Itoa(n.Port))
	name := url.PathEscape(n.Name)
	switch n.Protocol {
	case singbox.ProtocolShadowsocks:
		cipher := singbox.SettingString(n.Settings, "cipher")
		password, err := ssClientPassword(appKey, n, userUUID, cipher)
		if err != nil {
			return "", err
		}
		credential := base64.RawURLEncoding.EncodeToString([]byte(cipher + ":" + password))
		return "ss://" + credential + "@" + host + "#" + name, nil
	case singbox.ProtocolVLESS:
		reality := singbox.SettingMap(n.Settings, "reality_settings")
		serverName := singbox.SettingString(reality, "server_name")
		if serverName == "" {
			return "", fmt.Errorf("vless node %d has no server name", n.ID)
		}
		publicKey := singbox.SettingString(reality, "public_key")
		if publicKey == "" {
			derived, err := singbox.RealityPublicKey(singbox.SettingString(n.Secret, "private_key"))
			if err != nil {
				return "", err
			}
			publicKey = derived
		}
		q := url.Values{"security": {"reality"}, "encryption": {"none"}, "flow": {"xtls-rprx-vision"}, "sni": {serverName}, "pbk": {publicKey}, "type": {"tcp"}}
		if sid := singbox.SettingString(reality, "short_id"); sid != "" {
			q.Set("sid", sid)
		}
		return "vless://" + url.PathEscape(userUUID) + "@" + host + "?" + q.Encode() + "#" + name, nil
	case singbox.ProtocolHysteria2:
		serverName := singbox.SettingString(singbox.SettingMap(n.Settings, "tls"), "server_name")
		if serverName == "" {
			return "", fmt.Errorf("hysteria2 node %d has no server name", n.ID)
		}
		q := url.Values{"sni": {serverName}}
		obfs := singbox.SettingMap(n.Settings, "obfs")
		if obfsPassword := singbox.SettingString(obfs, "password"); obfsPassword != "" && obfsEnabled(obfs) {
			q.Set("obfs", "salamander")
			q.Set("obfs-password", obfsPassword)
		}
		if hopPorts := singbox.SettingString(n.Settings, "hop_interval"); hopPorts != "" {
			q.Set("mport", hopPorts)
		}
		return "hysteria2://" + url.PathEscape(userUUID) + "@" + host + "?" + q.Encode() + "#" + name, nil
	case singbox.ProtocolAnyTLS:
		serverName := singbox.SettingString(singbox.SettingMap(n.Settings, "tls"), "server_name")
		if serverName == "" {
			return "", fmt.Errorf("anytls node %d has no server name", n.ID)
		}
		q := url.Values{"sni": {serverName}}
		return "anytls://" + url.PathEscape(userUUID) + "@" + host + "?" + q.Encode() + "#" + name, nil
	default:
		return "", fmt.Errorf("unsupported protocol %q", n.Protocol)
	}
}

var allowedTopLevel = map[string]bool{
	"mixed-port": true, "allow-lan": true, "bind-address": true, "mode": true, "log-level": true, "ipv6": true,
	"unified-delay": true, "tcp-concurrent": true, "find-process-mode": true,
	"global-client-fingerprint": true, "dns": true, "proxy-groups": true, "proxies": true, "rules": true,
	"rule-providers":  true,
	"proxy-providers": true, "listeners": true, "tun": true, "external-controller": true,
	"external-controller-tls": true, "secret": true, "authentication": true, "skip-auth-prefixes": true,
	"script": true,
}

// replacedTopLevel fields are accepted so a full Clash config can be pasted
// verbatim, but are rebuilt from panel-generated values during assembly.
// droppedTopLevel fields are accepted for paste compatibility, then removed
// before rendering because they would expose control-plane or resource-fetching
// capabilities to every subscriber. Neither set has its contents validated.
var replacedTopLevel = map[string]bool{
	"proxy-groups": true, "proxies": true,
}

var droppedTopLevel = map[string]bool{
	"proxy-providers": true, "listeners": true, "external-controller": true,
	"external-controller-tls": true, "secret": true, "authentication": true, "skip-auth-prefixes": true,
	"script": true,
}

var forbiddenRecursive = map[string]bool{
	"proxies": true, "proxy-providers": true, "listeners": true, "tun": true, "external-controller": true,
	"external-controller-tls": true, "secret": true, "authentication": true, "skip-auth-prefixes": true,
	"script": true, "path": true, "url": true, "header": true, "headers": true,
}

func ValidateTemplate(text string) error {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(text), &config); err != nil {
		return fmt.Errorf("invalid Clash Meta YAML: %w", err)
	}
	if len(config) == 0 {
		return fmt.Errorf("Clash Meta template must be a non-empty mapping")
	}
	for key, value := range config {
		if !allowedTopLevel[key] {
			return fmt.Errorf("Clash Meta template field %q is not allowed", key)
		}
		if !replacedTopLevel[key] && !droppedTopLevel[key] && hasForbidden(value) {
			return fmt.Errorf("Clash Meta template field %q contains a forbidden capability", key)
		}
	}
	_, err := expandGroups(config, nil, true)
	return err
}

func hasForbidden(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			if forbiddenRecursive[strings.ToLower(key)] || hasForbidden(child) {
				return true
			}
		}
	case []any:
		for _, child := range v {
			if hasForbidden(child) {
				return true
			}
		}
	}
	return false
}

func RenderClash(appKey []byte, userUUID, template string, nodes []Node) ([]byte, error) {
	proxies := make([]map[string]any, 0, len(nodes))
	names := map[string][]string{"all": {}}
	for _, n := range expandNodes(nodes) {
		proxy, err := renderProxy(appKey, userUUID, n)
		if err != nil {
			return nil, err
		}
		proxies = append(proxies, proxy)
		names["all"] = append(names["all"], n.Name)
		names[n.Protocol] = append(names[n.Protocol], n.Name)
	}
	return assembleClash(template, proxies, names)
}

func assembleClash(template string, proxies []map[string]any, names map[string][]string) ([]byte, error) {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(template), &config); err != nil {
		return nil, err
	}
	if err := ValidateTemplate(template); err != nil {
		return nil, err
	}
	groups, err := expandGroups(config, names, false)
	if err != nil {
		return nil, err
	}
	for key := range config {
		if droppedTopLevel[key] {
			delete(config, key)
		}
	}
	config["proxy-groups"] = groups
	config["proxies"] = proxies
	return yaml.Marshal(config)
}

func renderProxy(appKey []byte, userUUID string, n Node) (map[string]any, error) {
	p := map[string]any{"name": n.Name, "server": n.Address, "port": n.Port, "type": n.Protocol}
	switch n.Protocol {
	case singbox.ProtocolShadowsocks:
		cipher := singbox.SettingString(n.Settings, "cipher")
		password, err := ssClientPassword(appKey, n, userUUID, cipher)
		if err != nil {
			return nil, err
		}
		p["cipher"], p["password"], p["udp"] = cipher, password, true
	case singbox.ProtocolVLESS:
		reality := singbox.SettingMap(n.Settings, "reality_settings")
		serverName := singbox.SettingString(reality, "server_name")
		if serverName == "" {
			return nil, fmt.Errorf("vless node %d has no server name", n.ID)
		}
		publicKey := singbox.SettingString(reality, "public_key")
		if publicKey == "" {
			derived, err := singbox.RealityPublicKey(singbox.SettingString(n.Secret, "private_key"))
			if err != nil {
				return nil, err
			}
			publicKey = derived
		}
		p["uuid"], p["flow"], p["network"], p["tls"] = userUUID, "xtls-rprx-vision", "tcp", true
		p["servername"], p["client-fingerprint"] = serverName, "chrome"
		p["reality-opts"] = map[string]any{"public-key": publicKey, "short-id": singbox.SettingString(reality, "short_id")}
	case singbox.ProtocolHysteria2:
		p["password"], p["sni"] = userUUID, singbox.SettingString(singbox.SettingMap(n.Settings, "tls"), "server_name")
		p["skip-cert-verify"] = false
		obfs := singbox.SettingMap(n.Settings, "obfs")
		if obfsPassword := singbox.SettingString(obfs, "password"); obfsPassword != "" && obfsEnabled(obfs) {
			p["obfs"] = "salamander"
			p["obfs-password"] = obfsPassword
		}
		if hopPorts := singbox.SettingString(n.Settings, "hop_interval"); hopPorts != "" {
			p["ports"] = hopPorts
		}
	case singbox.ProtocolAnyTLS:
		serverName := singbox.SettingString(singbox.SettingMap(n.Settings, "tls"), "server_name")
		if serverName == "" {
			return nil, fmt.Errorf("anytls node %d has no server name", n.ID)
		}
		p["password"], p["sni"] = userUUID, serverName
		p["skip-cert-verify"] = false
		p["udp"] = true
	default:
		return nil, fmt.Errorf("unsupported protocol %q", n.Protocol)
	}
	return p, nil
}

func ssClientPassword(appKey []byte, n Node, userUUID, cipher string) (string, error) {
	userKey, err := singbox.DeriveSSPassword(appKey, n.ID, userUUID, cipher)
	if err != nil {
		return "", err
	}
	if !singbox.IsSS2022(cipher) {
		return combineSSClientPassword(cipher, "", userKey), nil
	}
	serverKey := singbox.SettingString(n.Secret, "password")
	if serverKey == "" {
		if serverKey, err = singbox.DeriveSSServerPassword(appKey, n.ID, cipher); err != nil {
			return "", err
		}
	}
	return combineSSClientPassword(cipher, serverKey, userKey), nil
}

func combineSSClientPassword(cipher, serverKey, userKey string) string {
	return singbox.CombineSSClientPassword(cipher, serverKey, userKey)
}

func obfsEnabled(obfs map[string]any) bool {
	if v, ok := obfs["open"].(bool); ok {
		return v
	}
	return true
}

func expandGroups(config map[string]any, names map[string][]string, validating bool) ([]any, error) {
	raw, ok := config["proxy-groups"]
	if !ok {
		return nil, fmt.Errorf("Clash Meta template requires proxy-groups")
	}
	groups, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("proxy-groups must be a sequence")
	}
	groupNames := map[string]bool{}
	for _, rawGroup := range groups {
		group, ok := rawGroup.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("proxy-groups entries must be mappings")
		}
		name, _ := group["name"].(string)
		if name == "" || groupNames[name] {
			return nil, fmt.Errorf("proxy group names must be non-empty and unique")
		}
		groupNames[name] = true
		for key := range group {
			if key != "name" && key != "type" && key != "proxies" && key != "url" && key != "interval" && key != "tolerance" && key != "lazy" && key != "disable-udp" {
				return nil, fmt.Errorf("proxy group field %q is not allowed", key)
			}
		}
	}
	placeholders := map[string]string{"__ALL_PROXIES__": "all", "__SHADOWSOCKS_PROXIES__": singbox.ProtocolShadowsocks, "__VLESS_PROXIES__": singbox.ProtocolVLESS, "__HYSTERIA2_PROXIES__": singbox.ProtocolHysteria2, "__ANYTLS_PROXIES__": singbox.ProtocolAnyTLS}
	for _, rawGroup := range groups {
		group := rawGroup.(map[string]any)
		items, ok := group["proxies"].([]any)
		if !ok {
			return nil, fmt.Errorf("proxy group %q requires proxies", group["name"])
		}
		expanded := []any{}
		for _, item := range items {
			name, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("proxy group entries must be strings")
			}
			if kind, found := placeholders[name]; found {
				if !validating {
					for _, dynamic := range names[kind] {
						expanded = append(expanded, dynamic)
					}
				}
				continue
			}
			if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
				return nil, fmt.Errorf("unknown proxy placeholder %q", name)
			}
			if !groupNames[name] && name != "DIRECT" && name != "REJECT" {
				return nil, fmt.Errorf("proxy group references unknown target %q", name)
			}
			expanded = append(expanded, name)
		}
		group["proxies"] = expanded
	}
	return groups, nil
}
