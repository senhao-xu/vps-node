package singbox

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
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

type ClashAPI struct {
	Port   int
	Secret string
}

func Render(appKey []byte, nodes []Node, clash ClashAPI) (map[string]any, error) {
	inbounds := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		inbound, err := renderInbound(appKey, n)
		if err != nil {
			return nil, err
		}
		inbounds = append(inbounds, inbound)
	}
	return map[string]any{
		"log": map[string]any{"level": "info", "timestamp": true},
		"experimental": map[string]any{
			"clash_api": map[string]any{
				"external_controller": fmt.Sprintf("127.0.0.1:%d", clash.Port),
				"secret":              clash.Secret,
			},
		},
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
	default:
		return nil, fmt.Errorf("%w: node %d: unknown protocol %q", ErrUnrenderable, n.ID, n.Protocol)
	}
}

func inboundTag(n Node) string {
	return n.Protocol + "-" + strconv.FormatInt(n.ID, 10)
}

func renderShadowsocks(appKey []byte, n Node) (map[string]any, error) {
	method := SettingString(n.Settings, "method")
	if !SSMethodSupported(method) {
		return nil, fmt.Errorf("%w: node %d: unsupported shadowsocks method %q", ErrUnrenderable, n.ID, method)
	}
	serverPassword := SettingString(n.Secret, "password")
	if serverPassword == "" {
		derived, err := DeriveSSServerPassword(appKey, n.ID, method)
		if err != nil {
			return nil, err
		}
		serverPassword = derived
	}
	users := make([]map[string]any, 0, len(n.Users))
	for _, u := range n.Users {
		password, err := DeriveSSPassword(appKey, n.ID, u.UUID, method)
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
		"method":      method,
		"password":    serverPassword,
		"users":       users,
	}, nil
}

func renderVLESS(n Node) (map[string]any, error) {
	privateKey := SettingString(n.Secret, "private_key")
	if privateKey == "" {
		return nil, fmt.Errorf("%w: node %d: missing vless private_key", ErrUnrenderable, n.ID)
	}
	serverNames := settingStrings(n.Settings, "server_names")
	if len(serverNames) == 0 || serverNames[0] == "" {
		return nil, fmt.Errorf("%w: node %d: missing vless server_names", ErrUnrenderable, n.ID)
	}
	serverName := serverNames[0]
	shortIDs := []string{}
	if shortID := SettingString(n.Settings, "short_id"); shortID != "" {
		shortIDs = append(shortIDs, shortID)
	}
	users := make([]map[string]any, 0, len(n.Users))
	for _, u := range n.Users {
		users = append(users, map[string]any{
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
				"handshake":   map[string]any{"server": serverName, "server_port": 443},
				"private_key": privateKey,
				"short_id":    shortIDs,
			},
		},
	}, nil
}

func renderHysteria2(n Node) (map[string]any, error) {
	serverName := SettingString(n.Settings, "server_name")
	certificate := SettingString(n.Secret, "certificate")
	privateKey := SettingString(n.Secret, "private_key")
	users := make([]map[string]any, 0, len(n.Users))
	for _, u := range n.Users {
		users = append(users, map[string]any{"password": u.UUID})
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
	if v, ok := settingInt(n.Settings, "up_mbps"); ok {
		inbound["up_mbps"] = v
	}
	if v, ok := settingInt(n.Settings, "down_mbps"); ok {
		inbound["down_mbps"] = v
	}
	if obfsPassword := SettingString(n.Settings, "obfs_password"); obfsPassword != "" {
		inbound["obfs"] = map[string]any{"type": "salamander", "password": obfsPassword}
	}
	return inbound, nil
}

func SettingString(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func settingStrings(m map[string]any, key string) []string {
	if m == nil {
		return nil
	}
	switch v := m[key].(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	default:
		return nil
	}
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

func NewClashAPI(reservedPorts map[int]bool) (ClashAPI, error) {
	for i := 0; i < 16; i++ {
		port, err := randomPort()
		if err != nil {
			return ClashAPI{}, err
		}
		if reservedPorts[port] {
			continue
		}
		secret := make([]byte, 16)
		if _, err := rand.Read(secret); err != nil {
			return ClashAPI{}, err
		}
		return ClashAPI{Port: port, Secret: hex.EncodeToString(secret)}, nil
	}
	return ClashAPI{}, errors.New("no free clash api port available")
}

func randomPort() (int, error) {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	return 20000 + int(binary.BigEndian.Uint32(buf[:])%20000), nil
}
