package singbox

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const ContractVersion = "singbox-render-v2"

const ssCredDomain = "ss-cred-v1"

// RelayCredDomain is the derivation domain for chain relay credentials: pseudo
// users injected into exit-node inbounds and the matching client credentials
// rendered into entry-server outbounds. Relay credentials are never stored;
// both sides derive them from the panel app key.
const RelayCredDomain = "relay-cred-v1"

// RelayUserPrefix prefixes the pseudo-user names injected into exit-node
// inbounds for chained traffic. Agents never map these names to a panel user,
// so relay traffic is never attributed or reported.
const RelayUserPrefix = "relay-"

const (
	ProtocolShadowsocks = "shadowsocks"
	ProtocolVLESS       = "vless"
	ProtocolHysteria2   = "hysteria2"
	ProtocolAnyTLS      = "anytls"

	SSMethod2022Aes128Gcm = "2022-blake3-aes-128-gcm"
	SSMethod2022Aes256Gcm = "2022-blake3-aes-256-gcm"
	SSMethod2022Chacha20  = "2022-blake3-chacha20-poly1305"
)

var ssKeyLens = map[string]int{
	SSMethod2022Aes128Gcm: 16,
	SSMethod2022Aes256Gcm: 32,
	SSMethod2022Chacha20:  32,
}

var ErrUnrenderable = errors.New("node configuration cannot be rendered")

func SSMethodSupported(method string) bool {
	_, ok := ssKeyLens[method]
	return ok
}

func IsSS2022(method string) bool {
	return strings.HasPrefix(method, "2022-blake3-")
}

func hmacSum(appKey []byte, message string) []byte {
	mac := hmac.New(sha256.New, appKey)
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

func deriveSSKey(appKey []byte, message, method string) (string, error) {
	keyLen, ok := ssKeyLens[method]
	if !ok {
		return "", fmt.Errorf("%w: unsupported shadowsocks method %q", ErrUnrenderable, method)
	}
	sum := hmacSum(appKey, message)
	return base64.StdEncoding.EncodeToString(sum[:keyLen]), nil
}

func DeriveSSPassword(appKey []byte, nodeID int64, userUUID, method string) (string, error) {
	return deriveSSKey(appKey, fmt.Sprintf("%s:%d:%s", ssCredDomain, nodeID, userUUID), method)
}

func DeriveSSServerPassword(appKey []byte, nodeID int64, method string) (string, error) {
	return deriveSSKey(appKey, fmt.Sprintf("%s:server:%d", ssCredDomain, nodeID), method)
}

// CombineSSClientPassword joins the server and user keys the way sing-box
// 2022 multi-user clients expect; legacy methods use the user key alone.
func CombineSSClientPassword(cipher, serverKey, userKey string) string {
	if !IsSS2022(cipher) {
		return userKey
	}
	return serverKey + ":" + userKey
}

// RealityPublicKey derives the base64url X25519 public key from a base64url
// private key; subscriptions and chain outbounds share this derivation.
func RealityPublicKey(privateKeyB64 string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(privateKeyB64)
	if err != nil {
		return "", fmt.Errorf("derive reality key: %w", err)
	}
	key, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return "", fmt.Errorf("derive reality key: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

// RelayUserName is the pseudo-user name an entry server authenticates with on
// the exit node's inbound. One name per (exit node, entry server) pair.
func RelayUserName(entryServerID int64) string {
	return RelayUserPrefix + strconv.FormatInt(entryServerID, 10)
}

func relayCredSubject(entryServerID int64) string {
	return "relay:" + strconv.FormatInt(entryServerID, 10)
}

// DeriveRelaySSPassword derives the relay user key for a Shadowsocks exit
// node, reusing the ss-cred-v1 domain with a relay: pseudo-UUID.
func DeriveRelaySSPassword(appKey []byte, exitNodeID, entryServerID int64, method string) (string, error) {
	return DeriveSSPassword(appKey, exitNodeID, relayCredSubject(entryServerID), method)
}

// DeriveRelayUUID derives the deterministic relay UUID for a VLESS exit node
// (UUIDv5-shaped over the HMAC of the relay identity).
func DeriveRelayUUID(appKey []byte, exitNodeID, entryServerID int64) string {
	sum := hmacSum(appKey, fmt.Sprintf("%s:uuid:%d:%d", RelayCredDomain, exitNodeID, entryServerID))
	b := make([]byte, 16)
	copy(b, sum[:16])
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// DeriveRelayPassword derives the relay password for Hysteria2/AnyTLS exit
// nodes.
func DeriveRelayPassword(appKey []byte, exitNodeID, entryServerID int64) string {
	sum := hmacSum(appKey, fmt.Sprintf("%s:password:%d:%d", RelayCredDomain, exitNodeID, entryServerID))
	return hex.EncodeToString(sum[:16])
}

type User struct {
	ID   int64
	UUID string
}

type Node struct {
	ID       int64
	Name     string
	Protocol string
	Port     int
	Settings map[string]any
	Secret   map[string]any
	Users    []User
	// Relays are pseudo users injected into this node's inbound so entry
	// servers chaining through this node can authenticate.
	Relays []Relay
}

// Relay identifies one entry server allowed to relay through an exit node.
type Relay struct {
	EntryServerID int64
}

// ChainExit describes one chained entry node: traffic hitting the entry
// node's inbound is forwarded through a protocol client outbound to the exit
// node with derived relay credentials.
type ChainExit struct {
	EntryNodeID   int64
	EntryServerID int64
	Exit          Node
	DialAddress   string
}

func Render(appKey []byte, nodes []Node, chains ...ChainExit) (map[string]any, error) {
	inbounds := make([]map[string]any, 0, len(nodes))
	nodeByID := make(map[int64]Node, len(nodes))
	for _, n := range nodes {
		inbound, err := renderInbound(appKey, n)
		if err != nil {
			return nil, err
		}
		inbounds = append(inbounds, inbound)
		nodeByID[n.ID] = n
	}
	outbounds := []map[string]any{{"type": "direct", "tag": "direct"}}
	var rules []map[string]any
	for _, c := range chains {
		entry, ok := nodeByID[c.EntryNodeID]
		if !ok {
			continue
		}
		outbound, err := renderChainOutbound(appKey, c)
		if err != nil {
			return nil, err
		}
		outbounds = append(outbounds, outbound)
		rules = append(rules, map[string]any{
			"inbound":  []string{inboundTag(entry)},
			"outbound": chainOutboundTag(c.EntryNodeID),
		})
	}
	route := map[string]any{"final": "direct"}
	if len(rules) > 0 {
		route["rules"] = rules
	}
	return map[string]any{
		"log":       map[string]any{"level": "info", "timestamp": true},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}, nil
}

func chainOutboundTag(entryNodeID int64) string {
	return "chain-" + strconv.FormatInt(entryNodeID, 10)
}

func renderInbound(appKey []byte, n Node) (map[string]any, error) {
	switch n.Protocol {
	case ProtocolShadowsocks:
		return renderShadowsocks(appKey, n)
	case ProtocolVLESS:
		return renderVLESS(appKey, n)
	case ProtocolHysteria2:
		return renderHysteria2(appKey, n)
	case ProtocolAnyTLS:
		return renderAnyTLS(appKey, n)
	default:
		return nil, fmt.Errorf("%w: node %d: unknown protocol %q", ErrUnrenderable, n.ID, n.Protocol)
	}
}

func inboundTag(n Node) string {
	return n.Protocol + "-" + strconv.FormatInt(n.ID, 10)
}

func renderShadowsocks(appKey []byte, n Node) (map[string]any, error) {
	cipher := SettingString(n.Settings, "cipher")
	if !SSMethodSupported(cipher) {
		return nil, fmt.Errorf("%w: node %d: unsupported shadowsocks cipher %q", ErrUnrenderable, n.ID, cipher)
	}
	serverPassword := SettingString(n.Secret, "password")
	if serverPassword == "" {
		derived, err := DeriveSSServerPassword(appKey, n.ID, cipher)
		if err != nil {
			return nil, err
		}
		serverPassword = derived
	}
	users := make([]map[string]any, 0, len(n.Users)+len(n.Relays))
	for _, u := range n.Users {
		password, err := DeriveSSPassword(appKey, n.ID, u.UUID, cipher)
		if err != nil {
			return nil, err
		}
		users = append(users, map[string]any{
			"name":     "u-" + strconv.FormatInt(u.ID, 10),
			"password": password,
		})
	}
	for _, relay := range n.Relays {
		password, err := DeriveRelaySSPassword(appKey, n.ID, relay.EntryServerID, cipher)
		if err != nil {
			return nil, err
		}
		users = append(users, map[string]any{
			"name":     RelayUserName(relay.EntryServerID),
			"password": password,
		})
	}
	return map[string]any{
		"type":        ProtocolShadowsocks,
		"tag":         inboundTag(n),
		"listen":      "::",
		"listen_port": n.Port,
		"method":      cipher,
		"password":    serverPassword,
		"users":       users,
	}, nil
}

func renderVLESS(appKey []byte, n Node) (map[string]any, error) {
	privateKey := SettingString(n.Secret, "private_key")
	if privateKey == "" {
		return nil, fmt.Errorf("%w: node %d: missing vless private_key", ErrUnrenderable, n.ID)
	}
	reality := SettingMap(n.Settings, "reality_settings")
	serverName := SettingString(reality, "server_name")
	if serverName == "" {
		return nil, fmt.Errorf("%w: node %d: missing vless reality server_name", ErrUnrenderable, n.ID)
	}
	handshakePort := int64(443)
	if v, ok := settingInt(reality, "server_port"); ok {
		handshakePort = v
	}
	shortIDs := []string{}
	if shortID := SettingString(reality, "short_id"); shortID != "" {
		shortIDs = append(shortIDs, shortID)
	}
	users := make([]map[string]any, 0, len(n.Users)+len(n.Relays))
	for _, u := range n.Users {
		users = append(users, map[string]any{
			"name": "u-" + strconv.FormatInt(u.ID, 10),
			"uuid": u.UUID,
			"flow": "xtls-rprx-vision",
		})
	}
	for _, relay := range n.Relays {
		users = append(users, map[string]any{
			"name": RelayUserName(relay.EntryServerID),
			"uuid": DeriveRelayUUID(appKey, n.ID, relay.EntryServerID),
			"flow": "xtls-rprx-vision",
		})
	}
	return map[string]any{
		"type":        ProtocolVLESS,
		"tag":         inboundTag(n),
		"listen":      "::",
		"listen_port": n.Port,
		"users":       users,
		"tls": map[string]any{
			"enabled":     true,
			"server_name": serverName,
			"reality": map[string]any{
				"enabled":     true,
				"handshake":   map[string]any{"server": serverName, "server_port": handshakePort},
				"private_key": privateKey,
				"short_id":    shortIDs,
			},
		},
	}, nil
}

func renderHysteria2(appKey []byte, n Node) (map[string]any, error) {
	serverName := SettingString(SettingMap(n.Settings, "tls"), "server_name")
	certificate := SettingString(n.Secret, "certificate")
	privateKey := SettingString(n.Secret, "private_key")
	users := make([]map[string]any, 0, len(n.Users)+len(n.Relays))
	for _, u := range n.Users {
		users = append(users, map[string]any{
			"name":     "u-" + strconv.FormatInt(u.ID, 10),
			"password": u.UUID,
		})
	}
	for _, relay := range n.Relays {
		users = append(users, map[string]any{
			"name":     RelayUserName(relay.EntryServerID),
			"password": DeriveRelayPassword(appKey, n.ID, relay.EntryServerID),
		})
	}
	tls := map[string]any{"enabled": true}
	if serverName != "" && certificate != "" && privateKey != "" {
		tls["server_name"] = serverName
		tls["certificate"] = strings.Split(certificate, "\n")
		tls["key"] = strings.Split(privateKey, "\n")
	}
	inbound := map[string]any{
		"type":        ProtocolHysteria2,
		"tag":         inboundTag(n),
		"listen":      "::",
		"listen_port": n.Port,
		"users":       users,
		"tls":         tls,
	}
	bandwidth := SettingMap(n.Settings, "bandwidth")
	if v, ok := settingInt(bandwidth, "up"); ok {
		inbound["up_mbps"] = v
	}
	if v, ok := settingInt(bandwidth, "down"); ok {
		inbound["down_mbps"] = v
	}
	obfs := SettingMap(n.Settings, "obfs")
	obfsPassword := SettingString(obfs, "password")
	obfsOpen := true
	if v, ok := obfs["open"].(bool); ok {
		obfsOpen = v
	}
	if obfsOpen && obfsPassword != "" {
		inbound["obfs"] = map[string]any{"type": "salamander", "password": obfsPassword}
	}
	return inbound, nil
}

func renderAnyTLS(appKey []byte, n Node) (map[string]any, error) {
	serverName := SettingString(SettingMap(n.Settings, "tls"), "server_name")
	certificate := SettingString(n.Secret, "certificate")
	privateKey := SettingString(n.Secret, "private_key")
	users := make([]map[string]any, 0, len(n.Users)+len(n.Relays))
	for _, u := range n.Users {
		users = append(users, map[string]any{
			"name":     "u-" + strconv.FormatInt(u.ID, 10),
			"password": u.UUID,
		})
	}
	for _, relay := range n.Relays {
		users = append(users, map[string]any{
			"name":     RelayUserName(relay.EntryServerID),
			"password": DeriveRelayPassword(appKey, n.ID, relay.EntryServerID),
		})
	}
	tls := map[string]any{"enabled": true}
	if serverName != "" && certificate != "" && privateKey != "" {
		tls["server_name"] = serverName
		tls["certificate"] = strings.Split(certificate, "\n")
		tls["key"] = strings.Split(privateKey, "\n")
	}
	return map[string]any{
		"type":        ProtocolAnyTLS,
		"tag":         inboundTag(n),
		"listen":      "::",
		"listen_port": n.Port,
		"users":       users,
		"tls":         tls,
	}, nil
}

func renderChainOutbound(appKey []byte, c ChainExit) (map[string]any, error) {
	tag := chainOutboundTag(c.EntryNodeID)
	switch c.Exit.Protocol {
	case ProtocolShadowsocks:
		return renderShadowsocksOutbound(appKey, c, tag)
	case ProtocolVLESS:
		return renderVLESSOutbound(appKey, c, tag)
	case ProtocolHysteria2:
		return renderHysteria2Outbound(appKey, c, tag)
	case ProtocolAnyTLS:
		return renderAnyTLSOutbound(appKey, c, tag)
	default:
		return nil, fmt.Errorf("%w: node %d: unknown protocol %q", ErrUnrenderable, c.Exit.ID, c.Exit.Protocol)
	}
}

func renderShadowsocksOutbound(appKey []byte, c ChainExit, tag string) (map[string]any, error) {
	cipher := SettingString(c.Exit.Settings, "cipher")
	if !SSMethodSupported(cipher) {
		return nil, fmt.Errorf("%w: node %d: unsupported shadowsocks cipher %q", ErrUnrenderable, c.Exit.ID, cipher)
	}
	serverKey := SettingString(c.Exit.Secret, "password")
	if serverKey == "" {
		derived, err := DeriveSSServerPassword(appKey, c.Exit.ID, cipher)
		if err != nil {
			return nil, err
		}
		serverKey = derived
	}
	userKey, err := DeriveRelaySSPassword(appKey, c.Exit.ID, c.EntryServerID, cipher)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"type":        ProtocolShadowsocks,
		"tag":         tag,
		"server":      c.DialAddress,
		"server_port": c.Exit.Port,
		"method":      cipher,
		"password":    CombineSSClientPassword(cipher, serverKey, userKey),
	}, nil
}

func renderVLESSOutbound(appKey []byte, c ChainExit, tag string) (map[string]any, error) {
	reality := SettingMap(c.Exit.Settings, "reality_settings")
	serverName := SettingString(reality, "server_name")
	if serverName == "" {
		return nil, fmt.Errorf("%w: node %d: missing vless reality server_name", ErrUnrenderable, c.Exit.ID)
	}
	publicKey := SettingString(reality, "public_key")
	if publicKey == "" {
		privateKey := SettingString(c.Exit.Secret, "private_key")
		if privateKey == "" {
			return nil, fmt.Errorf("%w: node %d: missing vless private_key", ErrUnrenderable, c.Exit.ID)
		}
		derived, err := RealityPublicKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("%w: node %d: %v", ErrUnrenderable, c.Exit.ID, err)
		}
		publicKey = derived
	}
	return map[string]any{
		"type":        ProtocolVLESS,
		"tag":         tag,
		"server":      c.DialAddress,
		"server_port": c.Exit.Port,
		"uuid":        DeriveRelayUUID(appKey, c.Exit.ID, c.EntryServerID),
		"flow":        "xtls-rprx-vision",
		"tls": map[string]any{
			"enabled":     true,
			"server_name": serverName,
			"reality": map[string]any{
				"enabled":    true,
				"public_key": publicKey,
				"short_id":   SettingString(reality, "short_id"),
			},
		},
	}, nil
}

// chainTLS builds the client TLS block for relay outbounds. The exit node's
// certificate (when stored) is pinned as the trusted chain; otherwise the
// handshake falls back to insecure, matching the panel-generated self-signed
// default.
func chainTLS(n Node) map[string]any {
	tls := map[string]any{"enabled": true}
	if serverName := SettingString(SettingMap(n.Settings, "tls"), "server_name"); serverName != "" {
		tls["server_name"] = serverName
	}
	if certificate := SettingString(n.Secret, "certificate"); certificate != "" {
		tls["certificate"] = strings.Split(certificate, "\n")
	} else {
		tls["insecure"] = true
	}
	return tls
}

func renderHysteria2Outbound(appKey []byte, c ChainExit, tag string) (map[string]any, error) {
	outbound := map[string]any{
		"type":        ProtocolHysteria2,
		"tag":         tag,
		"server":      c.DialAddress,
		"server_port": c.Exit.Port,
		"password":    DeriveRelayPassword(appKey, c.Exit.ID, c.EntryServerID),
		"tls":         chainTLS(c.Exit),
	}
	obfs := SettingMap(c.Exit.Settings, "obfs")
	obfsPassword := SettingString(obfs, "password")
	obfsOpen := true
	if v, ok := obfs["open"].(bool); ok {
		obfsOpen = v
	}
	if obfsOpen && obfsPassword != "" {
		outbound["obfs"] = map[string]any{"type": "salamander", "password": obfsPassword}
	}
	return outbound, nil
}

func renderAnyTLSOutbound(appKey []byte, c ChainExit, tag string) (map[string]any, error) {
	return map[string]any{
		"type":        ProtocolAnyTLS,
		"tag":         tag,
		"server":      c.DialAddress,
		"server_port": c.Exit.Port,
		"password":    DeriveRelayPassword(appKey, c.Exit.ID, c.EntryServerID),
		"tls":         chainTLS(c.Exit),
	}, nil
}

func SettingString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func SettingMap(m map[string]any, key string) map[string]any {
	if m == nil {
		return nil
	}
	v, _ := m[key].(map[string]any)
	return v
}

func settingInt(m map[string]any, key string) (int64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		if v == float64(int64(v)) {
			return int64(v), true
		}
		return 0, false
	default:
		return 0, false
	}
}
