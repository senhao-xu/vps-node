package web

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
)

var validProtocols = map[string]bool{
	repo.ProtocolShadowsocks: true,
	repo.ProtocolVLESS:       true,
	repo.ProtocolHysteria2:   true,
}

var shadowsocksMethods = map[string]bool{
	"2022-blake3-aes-128-gcm":       true,
	"2022-blake3-aes-256-gcm":       true,
	"2022-blake3-chacha20-poly1305": true,
}

func (h *Handler) handleNodeList(w http.ResponseWriter, r *http.Request) {
	page, err := parsePageQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var filter repo.NodeFilter
	if raw := r.URL.Query().Get("server_id"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &filter.ServerID); err != nil || filter.ServerID < 1 {
			writeErr(w, errInvalid("invalid server_id"))
			return
		}
	}
	if raw := r.URL.Query().Get("protocol"); raw != "" {
		if !validProtocols[raw] {
			writeErr(w, errInvalid("invalid protocol, want shadowsocks, vless or hysteria2"))
			return
		}
		filter.Protocol = raw
	}
	if raw := r.URL.Query().Get("status"); raw != "" {
		if raw != repo.NodeStatusActive && raw != repo.NodeStatusDisabled {
			writeErr(w, errInvalid("invalid status, want active or disabled"))
			return
		}
		filter.Status = raw
	}
	filter.Query = r.URL.Query().Get("q")
	filter.Page = page.Page
	filter.PageSize = page.PageSize
	nodes, total, err := h.repo.ListNodesPage(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	writePage(w, toNodeDTOs(nodes), total, page)
}

type createNodeRequest struct {
	ServerID int64           `json:"server_id"`
	Name     string          `json:"name"`
	Protocol string          `json:"protocol"`
	Port     int             `json:"port"`
	Settings json.RawMessage `json:"settings"`
}

func (h *Handler) handleNodeCreate(w http.ResponseWriter, r *http.Request) {
	var req createNodeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.ServerID < 1 {
		writeErr(w, errValidation("server_id is required"))
		return
	}
	server, err := h.repo.GetServer(r.Context(), req.ServerID)
	if err != nil {
		if err == repo.ErrNotFound {
			writeErr(w, errValidation("unknown server_id"))
			return
		}
		writeErr(w, err)
		return
	}
	if err := validateNodeSpec(req.Name, req.Protocol, req.Port); err != nil {
		writeErr(w, err)
		return
	}
	settingsJSON, secretEnc, err := h.buildNodeSettings(req.Protocol, req.Settings, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}

	id, err := h.repo.CreateNodeAndBump(r.Context(), repo.NewNode{
		ServerID:  req.ServerID,
		Name:      req.Name,
		Protocol:  req.Protocol,
		Port:      req.Port,
		Settings:  settingsJSON,
		SecretEnc: secretEnc,
	})
	if err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("node name or port already exists on this server"))
			return
		}
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	n.ServerName = server.Name
	httpx.WriteJSON(w, http.StatusCreated, toNodeDTO(n))
}

func (h *Handler) handleRealityKeypairGenerate(w http.ResponseWriter, _ *http.Request) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		writeErr(w, err)
		return
	}
	shortID := make([]byte, 8)
	if _, err := io.ReadFull(rand.Reader, shortID); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]string{
		"private_key": base64.RawURLEncoding.EncodeToString(privateKey.Bytes()),
		"public_key":  base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()),
		"short_id":    hex.EncodeToString(shortID),
	})
}

func validateNodeSpec(name, protocol string, port int) error {
	if name == "" || len(name) > 128 {
		return errValidation("name must be 1-128 characters")
	}
	if !validProtocols[protocol] {
		return errInvalid("invalid protocol, want shadowsocks, vless or hysteria2")
	}
	if port < 1 || port > 65535 {
		return errValidation("port must be 1-65535")
	}
	return nil
}

func (h *Handler) handleNodeGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	userCount, err := h.repo.CountUsersByNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	freshCutoff := time.Now().Add(-h.sessionFreshness(r.Context()))
	onlineUsers, err := h.repo.CountFreshSessionsByNodeIDs(r.Context(), []int64{id}, freshCutoff)
	if err != nil {
		writeErr(w, err)
		return
	}
	server, err := h.repo.GetServer(r.Context(), n.ServerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, nodeDetailDTO{
		nodeDTO:     toNodeDTO(n),
		UserCount:   userCount,
		OnlineUsers: onlineUsers[id],
		Server:      nodeRefDTO{ID: server.ID, Name: server.Name},
	})
}

type updateNodeRequest struct {
	Name     *string         `json:"name"`
	Port     *int            `json:"port"`
	Settings json.RawMessage `json:"settings"`
	Status   *string         `json:"status"`
}

func (h *Handler) handleNodeUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	current, err := h.repo.GetNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req updateNodeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}

	name, port := current.Name, current.Port
	if req.Name != nil {
		name = *req.Name
	}
	if req.Port != nil {
		port = *req.Port
	}
	if req.Status != nil {
		if *req.Status != repo.NodeStatusActive && *req.Status != repo.NodeStatusDisabled {
			writeErr(w, errInvalid("invalid status, want active or disabled"))
			return
		}
	}
	if err := validateNodeSpec(name, current.Protocol, port); err != nil {
		writeErr(w, err)
		return
	}

	settingsJSON, secretEnc, err := h.buildNodeSettings(current.Protocol, req.Settings, current.Settings, current.SecretEnc)
	if err != nil {
		writeErr(w, err)
		return
	}

	if err := h.repo.UpdateNodeAndBump(r.Context(), id, current.ServerID, name, port, settingsJSON, secretEnc, req.Status); err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("node name or port already exists on this server"))
			return
		}
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	server, err := h.repo.GetServer(r.Context(), current.ServerID)
	if err != nil {
		writeErr(w, err)
		return
	}
	n.ServerName = server.Name
	httpx.WriteJSON(w, http.StatusOK, toNodeDTO(n))
}

func (h *Handler) handleNodeDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.DeleteNodeAndBump(r.Context(), id, n.ServerID); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{})
}

func (h *Handler) buildNodeSettings(protocol string, raw json.RawMessage, currentSettings string, currentSecret []byte) (string, []byte, error) {
	plain := map[string]any{}
	if currentSettings != "" {
		if err := json.Unmarshal([]byte(currentSettings), &plain); err != nil {
			return "", nil, fmt.Errorf("decode current node settings: %w", err)
		}
	}
	secretFields := map[string]any{}
	if len(currentSecret) > 0 {
		secretJSON, err := secrets.Decrypt(h.appKey, currentSecret)
		if err != nil {
			return "", nil, fmt.Errorf("decrypt current node settings: %w", err)
		}
		if err := json.Unmarshal(secretJSON, &secretFields); err != nil {
			return "", nil, fmt.Errorf("decode current node secrets: %w", err)
		}
	}

	parsed := map[string]any{}
	if len(raw) > 0 && string(raw) != "null" {
		dec := json.NewDecoder(bytes.NewReader(raw))
		if err := dec.Decode(&parsed); err != nil {
			return "", nil, errValidation("settings must be a JSON object")
		}
	}
	if protocol == repo.ProtocolHysteria2 {
		_, certSet := parsed["certificate"]
		_, keySet := parsed["private_key"]
		if certSet != keySet {
			return "", nil, errValidation("hysteria2 certificate and private_key must be supplied together")
		}
	}
	for key, value := range parsed {
		switch protocol {
		case repo.ProtocolShadowsocks:
			switch key {
			case "method":
				s, ok := value.(string)
				if !ok || !shadowsocksMethods[s] {
					return "", nil, errValidation("settings.method must be a supported shadowsocks cipher")
				}
				plain[key] = s
			case "password":
				s, err := secretString(value, "settings.password")
				if err != nil {
					return "", nil, err
				}
				secretFields[key] = s
			default:
				return "", nil, errValidation(fmt.Sprintf("unknown settings field %q for shadowsocks", key))
			}
		case repo.ProtocolVLESS:
			switch key {
			case "private_key":
				s, err := secretString(value, "settings.private_key")
				if err != nil {
					return "", nil, err
				}
				secretFields[key] = s
			case "short_id":
				s, ok := value.(string)
				if !ok || len(s) > 64 {
					return "", nil, errValidation("settings.short_id must be a string of at most 64 characters")
				}
				plain[key] = s
			case "server_names":
				names, err := stringSlice(value, "settings.server_names")
				if err != nil {
					return "", nil, err
				}
				plain[key] = names
			default:
				return "", nil, errValidation(fmt.Sprintf("unknown settings field %q for vless", key))
			}
		case repo.ProtocolHysteria2:
			switch key {
			case "password":
				s, err := secretString(value, "settings.password")
				if err != nil {
					return "", nil, err
				}
				secretFields[key] = s
			case "up_mbps", "down_mbps":
				n, err := nonNegativeInt(value, "settings."+key)
				if err != nil {
					return "", nil, err
				}
				plain[key] = n
			case "server_name":
				s, err := secretString(value, "settings.server_name")
				if err != nil {
					return "", nil, err
				}
				plain[key] = s
			case "certificate", "private_key":
				s, ok := value.(string)
				if !ok || s == "" || len(s) > 256*1024 {
					return "", nil, errValidation("settings." + key + " must be a non-empty PEM string")
				}
				secretFields[key] = s
			default:
				return "", nil, errValidation(fmt.Sprintf("unknown settings field %q for hysteria2", key))
			}
		default:
			return "", nil, errInvalid("invalid protocol")
		}
	}
	if err := validateProtocolSettings(protocol, plain, secretFields); err != nil {
		return "", nil, err
	}

	settingsBytes, err := json.Marshal(plain)
	if err != nil {
		return "", nil, err
	}
	if len(secretFields) == 0 {
		return string(settingsBytes), nil, nil
	}
	secretBytes, err := json.Marshal(secretFields)
	if err != nil {
		return "", nil, err
	}
	secretEnc, err := secrets.Encrypt(h.appKey, secretBytes)
	if err != nil {
		return "", nil, err
	}
	return string(settingsBytes), secretEnc, nil
}

func validateProtocolSettings(protocol string, plain, secretFields map[string]any) error {
	switch protocol {
	case repo.ProtocolShadowsocks:
		method, _ := plain["method"].(string)
		if !shadowsocksMethods[method] {
			return errValidation("settings.method is required and must be a supported shadowsocks cipher")
		}
	case repo.ProtocolVLESS:
		privateKey, _ := secretFields["private_key"].(string)
		decoded, err := base64.RawURLEncoding.DecodeString(privateKey)
		if err != nil || len(decoded) != 32 {
			return errValidation("settings.private_key must be a valid X25519 private key")
		}
		if _, err := ecdh.X25519().NewPrivateKey(decoded); err != nil {
			return errValidation("settings.private_key must be a valid X25519 private key")
		}
		names, ok := plain["server_names"].([]string)
		if !ok {
			rawNames, rawOK := plain["server_names"].([]any)
			if rawOK {
				names = make([]string, 0, len(rawNames))
				for _, item := range rawNames {
					name, nameOK := item.(string)
					if !nameOK {
						return errValidation("settings.server_names must contain non-empty strings")
					}
					names = append(names, name)
				}
			}
		}
		if len(names) == 0 {
			return errValidation("settings.server_names must contain at least one server name")
		}
		for _, name := range names {
			if name == "" || len(name) > 253 {
				return errValidation("settings.server_names must contain non-empty strings of at most 253 characters")
			}
		}
		if shortID, _ := plain["short_id"].(string); shortID != "" {
			decoded, err := hex.DecodeString(shortID)
			if err != nil || len(decoded) > 8 {
				return errValidation("settings.short_id must be an even-length hexadecimal string of at most 16 characters")
			}
		}
	case repo.ProtocolHysteria2:
		name, _ := plain["server_name"].(string)
		certificate, certOK := secretFields["certificate"].(string)
		privateKey, keyOK := secretFields["private_key"].(string)
		if name == "" || !certOK || !keyOK {
			return errValidation("hysteria2 requires server_name, certificate and private_key")
		}
		pair, err := tls.X509KeyPair([]byte(certificate), []byte(privateKey))
		if err != nil || len(pair.Certificate) == 0 {
			return errValidation("hysteria2 certificate and private_key must match")
		}
		cert, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil || cert.NotBefore.After(time.Now()) || cert.NotAfter.Before(time.Now()) || cert.VerifyHostname(name) != nil {
			return errValidation("hysteria2 certificate does not cover server_name")
		}
	default:
		return errInvalid("invalid protocol")
	}
	return nil
}

func secretString(value any, field string) (string, error) {
	s, ok := value.(string)
	if !ok || len(s) < 1 || len(s) > 256 {
		return "", errValidation(field + " must be a string of 1-256 characters")
	}
	return s, nil
}

func stringSlice(value any, field string) ([]string, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, errValidation(field + " must be an array of strings")
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		s, ok := item.(string)
		if !ok || len(s) < 1 || len(s) > 253 {
			return nil, errValidation(field + " entries must be strings of 1-253 characters")
		}
		out = append(out, s)
	}
	return out, nil
}

func nonNegativeInt(value any, field string) (int64, error) {
	f, ok := value.(float64)
	if !ok || f < 0 || f > math.MaxInt64 || f != math.Trunc(f) {
		return 0, errValidation(field + " must be a non-negative integer")
	}
	return int64(f), nil
}
