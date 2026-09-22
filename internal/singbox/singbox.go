package singbox

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const ContractVersion = "singbox-render-v1"

const ssCredDomain = "ss-cred-v1"

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
}

func Render(appKey []byte, nodes []Node) (map[string]any, error) {
	inbounds := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		inbound, err := renderInbound(appKey, n)
		if err != nil {
			return nil, err
		}
		inbounds = append(inbounds, inbound)
	}
	return map[string]any{
		"log":       map[string]any{"level": "info", "timestamp": true},
		"inbounds":  inbounds,
		"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"final": "direct"},
	}, nil
}

func renderInbound(appKey []byte, n Node) (map[string]any, error) {
	switch n.Protocol {
	case ProtocolShadowsocks:
		return renderShadowsocks(appKey, n)
	case ProtocolVLESS:
		return renderVLESS(n)
	case ProtocolHysteria2:
		return renderHysteria2(n)
	case ProtocolAnyTLS:
		return renderAnyTLS(n)
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
	users := make([]map[string]any, 0, len(n.Users))
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

func renderVLESS(n Node) (map[string]any, error) {
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
	users := make([]map[string]any, 0, len(n.Users))
	for _, u := range n.Users {
		users = append(users, map[string]any{
			"name": "u-" + strconv.FormatInt(u.ID, 10),
			"uuid": u.UUID,
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

func renderHysteria2(n Node) (map[string]any, error) {
	serverName := SettingString(SettingMap(n.Settings, "tls"), "server_name")
	certificate := SettingString(n.Secret, "certificate")
	privateKey := SettingString(n.Secret, "private_key")
	users := make([]map[string]any, 0, len(n.Users))
	for _, u := range n.Users {
		users = append(users, map[string]any{
			"name":     "u-" + strconv.FormatInt(u.ID, 10),
			"password": u.UUID,
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

func renderAnyTLS(n Node) (map[string]any, error) {
	serverName := SettingString(SettingMap(n.Settings, "tls"), "server_name")
	certificate := SettingString(n.Secret, "certificate")
	privateKey := SettingString(n.Secret, "private_key")
	users := make([]map[string]any, 0, len(n.Users))
	for _, u := range n.Users {
		users = append(users, map[string]any{
			"name":     "u-" + strconv.FormatInt(u.ID, 10),
			"password": u.UUID,
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
