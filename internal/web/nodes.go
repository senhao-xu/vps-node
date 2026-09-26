package web

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

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
	ServerID    int64           `json:"server_id"`
	Address     string          `json:"address"`
	IPv6Enabled bool            `json:"ipv6_enabled"`
	IPv6Address string          `json:"ipv6_address"`
	Name        string          `json:"name"`
	Protocol    string          `json:"protocol"`
	Port        int             `json:"port"`
	Rate        *float64        `json:"rate"`
	Tags        *[]string       `json:"tags"`
	Settings    json.RawMessage `json:"settings"`
	ChainNodeID *int64          `json:"chain_node_id"`
}

// validateChainTarget verifies a chain exit reference: it must exist, be
// active, and not close a chain loop. nodeID == 0 on create.
func (h *Handler) validateChainTarget(ctx context.Context, nodeID, chainNodeID int64) error {
	target, err := h.repo.ValidateNodeChain(ctx, nodeID, chainNodeID)
	if err == repo.ErrNotFound {
		return errValidation("chain_node_id refers to a node that does not exist")
	}
	if errors.Is(err, repo.ErrChainCycle) {
		return errValidation("chain_node_id would create a node chain cycle")
	}
	if err != nil {
		return err
	}
	if target.Status != repo.NodeStatusActive {
		return errValidation("chain_node_id refers to a disabled node")
	}
	return nil
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
	if err := validateNodeSpec(req.Address, req.Name, req.Protocol, req.Port); err != nil {
		writeErr(w, err)
		return
	}
	req.IPv6Address = strings.TrimSpace(req.IPv6Address)
	if err := validateNodeIPv6(req.IPv6Enabled, req.IPv6Address); err != nil {
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
	if req.ChainNodeID != nil {
		if *req.ChainNodeID < 1 {
			writeErr(w, errValidation("chain_node_id must be a node id"))
			return
		}
		if err := h.validateChainTarget(r.Context(), 0, *req.ChainNodeID); err != nil {
			writeErr(w, err)
			return
		}
	}

	id, err := h.repo.CreateNodeAndBump(r.Context(), repo.NewNode{
		ServerID:         req.ServerID,
		Address:          req.Address,
		IPv6Enabled:      req.IPv6Enabled,
		IPv6Address:      req.IPv6Address,
		Name:             req.Name,
		Protocol:         req.Protocol,
		Port:             req.Port,
		ProtocolSettings: settingsJSON,
		Rate:             rate,
		Tags:             string(tagsJSON),
		SecretEnc:        secretEnc,
		ChainNodeID:      req.ChainNodeID,
	})
	if err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("a node with this port is already enabled on this server"))
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
	h.populateChainRef(r.Context(), &n)
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

func validateNodeSpec(address, name, protocol string, port int) error {
	if address == "" || len(address) > 255 {
		return errValidation("address must be 1-255 characters")
	}
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

func validateNodeIPv6(enabled bool, address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		if enabled {
			return errValidation("ipv6_address is required when ipv6_enabled is true")
		}
		return nil
	}
	if len(address) > 255 {
		return errValidation("ipv6_address must be at most 255 characters")
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
	h.populateChainRef(r.Context(), &n)
	settings := json.RawMessage(n.ProtocolSettings)
	if len(settings) == 0 {
		settings = json.RawMessage("{}")
	}
	httpx.WriteJSON(w, http.StatusOK, nodeDetailDTO{
		nodeDTO:     toNodeDTO(n),
		UserCount:   userCount,
		OnlineUsers: onlineUsers[id],
		Server:      nodeRefDTO{ID: server.ID, Name: server.Name},
		Settings:    settings,
	})
}

type updateNodeRequest struct {
	Address     *string         `json:"address"`
	IPv6Enabled *bool           `json:"ipv6_enabled"`
	IPv6Address *string         `json:"ipv6_address"`
	Name        *string         `json:"name"`
	Port        *int            `json:"port"`
	Rate        *float64        `json:"rate"`
	Tags        *[]string       `json:"tags"`
	Settings    json.RawMessage `json:"settings"`
	Status      *string         `json:"status"`
	// ChainNodeID is tri-state: absent keeps the current link, JSON null
	// unlinks, and a node id retargets the chain exit.
	ChainNodeID json.RawMessage `json:"chain_node_id"`
}

// resolveChainNodeID decodes the tri-state chain_node_id field; the second
// return value reports whether the field was present in the request body.
func resolveChainNodeID(raw json.RawMessage, current *int64) (*int64, error) {
	if len(raw) == 0 {
		return current, nil
	}
	if string(raw) == "null" {
		return nil, nil
	}
	var id int64
	if err := json.Unmarshal(raw, &id); err != nil || id < 1 {
		return current, errValidation("chain_node_id must be a node id or null")
	}
	return &id, nil
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

	address, name, port := current.Address, current.Name, current.Port
	ipv6Enabled, ipv6Address := current.IPv6Enabled, current.IPv6Address
	if req.Address != nil {
		address = *req.Address
	}
	if req.IPv6Enabled != nil {
		ipv6Enabled = *req.IPv6Enabled
	}
	if req.IPv6Address != nil {
		ipv6Address = strings.TrimSpace(*req.IPv6Address)
	}
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
	if err := validateNodeSpec(address, name, current.Protocol, port); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateNodeIPv6(ipv6Enabled, ipv6Address); err != nil {
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

	chainNodeID, err := resolveChainNodeID(req.ChainNodeID, current.ChainNodeID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if chainNodeID != nil {
		if err := h.validateChainTarget(r.Context(), id, *chainNodeID); err != nil {
			writeErr(w, err)
			return
		}
	}

	if err := h.repo.UpdateNodeAndBump(r.Context(), id, current.ServerID, address, name, ipv6Address, ipv6Enabled, port, settingsJSON, secretEnc, rate, string(tagsJSON), chainNodeID, req.Status); err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("a node with this port is already enabled on this server"))
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
	h.populateChainRef(r.Context(), &n)
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
	referencing, err := h.repo.ListNodesByChainTarget(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(referencing) > 0 {
		names := make([]string, 0, len(referencing))
		for _, ref := range referencing {
			names = append(names, ref.Name)
		}
		writeErr(w, errConflict(fmt.Sprintf("node is the chain exit of %s; unlink those nodes first", strings.Join(names, ", "))))
		return
	}
	if err := h.repo.DeleteNodeAndBump(r.Context(), id, n.ServerID); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{})
}

// populateChainRef fills the chain exit display names for handlers that read
// nodes through the plain (non-joined) select.
func (h *Handler) populateChainRef(ctx context.Context, n *repo.Node) {
	if n.ChainNodeID == nil {
		return
	}
	exit, err := h.repo.GetNode(ctx, *n.ChainNodeID)
	if err != nil {
		return
	}
	server, err := h.repo.GetServer(ctx, exit.ServerID)
	if err != nil {
		return
	}
	n.ChainNodeName = exit.Name
	n.ChainServerName = server.Name
}

// handleNodeCopy replicates a node verbatim (name, address, port, protocol
// settings and secrets) onto the same server. The copy starts disabled so it
// may temporarily reuse the source port; enabling it later requires freeing
// the port, which the partial unique index enforces.
func (h *Handler) handleNodeCopy(w http.ResponseWriter, r *http.Request) {
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
	newID, err := h.repo.CreateNodeAndBump(r.Context(), repo.NewNode{
		ServerID:         current.ServerID,
		Address:          current.Address,
		IPv6Enabled:      current.IPv6Enabled,
		IPv6Address:      current.IPv6Address,
		Name:             current.Name,
		Protocol:         current.Protocol,
		Port:             current.Port,
		ProtocolSettings: current.ProtocolSettings,
		Rate:             current.Rate,
		Tags:             current.Tags,
		SecretEnc:        current.SecretEnc,
		Status:           repo.NodeStatusDisabled,
	})
	if err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("a node with this port is already enabled on this server"))
			return
		}
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetNode(r.Context(), newID)
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
	httpx.WriteJSON(w, http.StatusCreated, toNodeDTO(n))
}
