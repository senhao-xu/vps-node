package web

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

var validProtocols = map[string]bool{
	repo.ProtocolShadowsocks: true,
	repo.ProtocolVLESS:       true,
	repo.ProtocolHysteria2:   true,
	repo.ProtocolAnyTLS:      true,
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
			writeErr(w, errInvalid("invalid protocol, want shadowsocks, vless, hysteria2 or anytls"))
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
	Rate     *float64        `json:"rate"`
	Tags     *[]string       `json:"tags"`
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
	rate := 1.0
	if req.Rate != nil {
		rate = *req.Rate
	}
	tags := []string{}
	if req.Tags != nil {
		tags = *req.Tags
	}
	if err := validateNodeMeta(rate, tags); err != nil {
		writeErr(w, err)
		return
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		writeErr(w, err)
		return
	}
	settingsJSON, secretEnc, err := h.buildNodeSettings(req.Protocol, req.Settings, "", nil)
	if err != nil {
		writeErr(w, err)
		return
	}

	id, err := h.repo.CreateNodeAndBump(r.Context(), repo.NewNode{
		ServerID:         req.ServerID,
		Name:             req.Name,
		Protocol:         req.Protocol,
		Port:             req.Port,
		ProtocolSettings: settingsJSON,
		Rate:             rate,
		Tags:             string(tagsJSON),
		SecretEnc:        secretEnc,
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
		return errInvalid("invalid protocol, want shadowsocks, vless, hysteria2 or anytls")
	}
	if port < 1 || port > 65535 {
		return errValidation("port must be 1-65535")
	}
	return nil
}

func validateNodeMeta(rate float64, tags []string) error {
	if !(rate > 0) || math.IsInf(rate, 0) || math.IsNaN(rate) {
		return errValidation("rate must be a positive number")
	}
	if len(tags) > 20 {
		return errValidation("tags must contain at most 20 entries")
	}
	for _, tag := range tags {
		if tag == "" || len(tag) > 32 {
			return errValidation("each tag must be 1-32 characters")
		}
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
	onlineUsers, err := h.repo.CountOnlineUsersByNodeIDs(r.Context(), []int64{id})
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
	Rate     *float64        `json:"rate"`
	Tags     *[]string       `json:"tags"`
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
	rate := current.Rate
	if req.Rate != nil {
		rate = *req.Rate
	}
	tags := nodeTags(current.Tags)
	if req.Tags != nil {
		tags = *req.Tags
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
	if err := validateNodeMeta(rate, tags); err != nil {
		writeErr(w, err)
		return
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		writeErr(w, err)
		return
	}

	settingsJSON, secretEnc, err := h.buildNodeSettings(current.Protocol, req.Settings, current.ProtocolSettings, current.SecretEnc)
	if err != nil {
		writeErr(w, err)
		return
	}

	if err := h.repo.UpdateNodeAndBump(r.Context(), id, current.ServerID, name, port, settingsJSON, secretEnc, rate, string(tagsJSON), req.Status); err != nil {
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
