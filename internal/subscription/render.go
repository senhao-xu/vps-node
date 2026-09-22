package subscription

import (
	"crypto/ecdh"
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
allow-lan: false
mode: rule
log-level: info
dns:
  enable: true
  enhanced-mode: fake-ip
  nameserver:
    - https://1.1.1.1/dns-query
proxy-groups:
  - name: 节点选择
    type: select
    proxies:
      - __ALL_PROXIES__
  - name: 自动选择
    type: url-test
    url: https://www.gstatic.com/generate_204
    interval: 300
    proxies:
      - __ALL_PROXIES__
rules:
  - MATCH,节点选择
`

type Node struct {
	ID       int64
	Name     string
	Protocol string
	Address  string
	Port     int
	Settings map[string]any
	Secret   map[string]any
}

func RenderGeneral(appKey []byte, userUUID string, nodes []Node) (string, error) {
	links := make([]string, 0, len(nodes))
	for _, node := range nodes {
		link, err := renderURI(appKey, userUUID, node)
		if err != nil {
			return "", err
		}
		links = append(links, link)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n"))), nil
}

// RenderGeneralLinks renders each node's share link, skipping nodes that
// cannot be rendered (reported via onSkip) so a single broken node does not
// break the whole subscription.
func RenderGeneralLinks(appKey []byte, userUUID string, nodes []Node, onSkip func(Node, error)) string {
	links := make([]string, 0, len(nodes))
	for _, node := range nodes {
		link, err := renderURI(appKey, userUUID, node)
		if err != nil {
			if onSkip != nil {
				onSkip(node, err)
			}
			continue
		}
		links = append(links, link)
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

// RenderClashFiltered assembles the Clash Meta config from the renderable
// nodes, skipping broken nodes (reported via onSkip).
func RenderClashFiltered(appKey []byte, userUUID, template string, nodes []Node, onSkip func(Node, error)) ([]byte, error) {
	proxies := make([]map[string]any, 0, len(nodes))
	names := map[string][]string{"all": {}}
	for _, n := range nodes {
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
	}
	return assembleClash(template, proxies, names)
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
			privateRaw, err := base64.RawURLEncoding.DecodeString(singbox.SettingString(n.Secret, "private_key"))
			if err != nil {
				return "", fmt.Errorf("derive reality key: %w", err)
			}
			privateKey, err := ecdh.X25519().NewPrivateKey(privateRaw)
			if err != nil {
				return "", fmt.Errorf("derive reality key: %w", err)
			}
			publicKey = base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())
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
	"global-client-fingerprint": true, "dns": true, "proxy-groups": true, "rules": true, "rule-providers": true,
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
		if key != "proxy-groups" && hasForbidden(value) {
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
	for _, n := range nodes {
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
			raw, err := base64.RawURLEncoding.DecodeString(singbox.SettingString(n.Secret, "private_key"))
			if err != nil {
				return nil, err
			}
			key, err := ecdh.X25519().NewPrivateKey(raw)
			if err != nil {
				return nil, err
			}
			publicKey = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
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
	if !singbox.IsSS2022(cipher) {
		return userKey
	}
	return serverKey + ":" + userKey
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
