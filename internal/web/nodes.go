package web

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	var serverID int64
	if raw := r.URL.Query().Get("server_id"); raw != "" {
		if _, err := fmt.Sscanf(raw, "%d", &serverID); err != nil || serverID < 1 {
			writeErr(w, errInvalid("invalid server_id"))
			return
		}
	}
	nodes, total, err := h.repo.ListNodesPage(r.Context(), serverID, page.Page, page.PageSize)
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
	if _, err := h.repo.GetServer(r.Context(), req.ServerID); err != nil {
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
	settingsJSON, secretEnc, err := h.buildNodeSettings(r.Context(), req.Protocol, req.Settings, "", nil)
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
	httpx.WriteJSON(w, http.StatusCreated, toNodeDTO(n))
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

	settingsJSON, secretEnc, err := h.buildNodeSettings(r.Context(), current.Protocol, req.Settings, current.Settings, current.SecretEnc)
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

func (h *Handler) buildNodeSettings(ctx context.Context, protocol string, raw json.RawMessage, currentSettings string, currentSecret []byte) (string, []byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return currentSettings, currentSecret, nil
	}
	var parsed map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(&parsed); err != nil {
		return "", nil, errValidation("settings must be a JSON object")
	}

	plain := map[string]any{}
	secretFields := map[string]any{}
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
			default:
				return "", nil, errValidation(fmt.Sprintf("unknown settings field %q for hysteria2", key))
			}
		default:
			return "", nil, errInvalid("invalid protocol")
		}
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
