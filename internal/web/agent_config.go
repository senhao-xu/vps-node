package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
	"vps-node/internal/singbox"
)

func (h *Handler) handleAgentConfig(w http.ResponseWriter, r *http.Request) {
	serverID := agentServerIDFrom(r.Context())
	applied, err := parseAppliedVersion(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	server, err := h.repo.GetServer(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if server.Status == repo.ServerStatusDisabled {
		writeErr(w, errForbidden("config sync is disabled for this server"))
		return
	}
	revision, err := h.repo.GetServerRevision(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}
	if applied == revision {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"status":   "current",
			"revision": revision,
		})
		return
	}

	payload, err := h.buildAgentConfigPayload(r.Context(), server)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"status":           "updated",
		"revision":         revision,
		"renderer_version": singbox.ContractVersion,
		"config":           map[string]any{"singbox": payload.Config},
		"users":            payload.Users,
	})
}

func parseAppliedVersion(r *http.Request) (int64, error) {
	raw := r.URL.Query().Get("version")
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		return 0, errInvalid("invalid version query parameter")
	}
	return v, nil
}

type agentUserNodeDTO struct {
	ID         int64          `json:"id"`
	Protocol   string         `json:"protocol"`
	Port       int            `json:"port"`
	Credential map[string]any `json:"credential"`
}

type agentUserDTO struct {
	ID         int64              `json:"id"`
	UUID       string             `json:"uuid"`
	Status     string             `json:"status"`
	QuotaBytes int64              `json:"quota_bytes"`
	UsedBytes  int64              `json:"used_bytes"`
	ExpiresAt  *string            `json:"expires_at"`
	Nodes      []agentUserNodeDTO `json:"nodes"`
}

type agentConfigPayload struct {
	Config map[string]any
	Users  []agentUserDTO
}

func (h *Handler) buildAgentConfigPayload(ctx context.Context, server repo.Server) (agentConfigPayload, error) {
	nodes, err := h.repo.ListNodesByServer(ctx, server.ID)
	if err != nil {
		return agentConfigPayload{}, err
	}
	active := make([]repo.Node, 0, len(nodes))
	nodeByID := make(map[int64]repo.Node, len(nodes))
	ports := make(map[int]bool, len(nodes))
	for _, n := range nodes {
		if n.Status != repo.NodeStatusActive {
			continue
		}
		active = append(active, n)
		nodeByID[n.ID] = n
		ports[n.Port] = true
	}

	users, err := h.repo.ListEligibleUsersByServer(ctx, server.ID, time.Now())
	if err != nil {
		return agentConfigPayload{}, err
	}

	usersByNode := make(map[int64][]singbox.User, len(users))
	nodesByUser := make(map[int64][]int64, len(users))
	for _, u := range users {
		nodeIDs, err := h.repo.ListNodeIDsByUser(ctx, u.ID)
		if err != nil {
			return agentConfigPayload{}, err
		}
		for _, nodeID := range nodeIDs {
			if _, ok := nodeByID[nodeID]; !ok {
				continue
			}
			usersByNode[nodeID] = append(usersByNode[nodeID], singbox.User{ID: u.ID, UUID: u.UUID})
			nodesByUser[u.ID] = append(nodesByUser[u.ID], nodeID)
		}
	}

	renderNodes := make([]singbox.Node, 0, len(active))
	renderByID := make(map[int64]singbox.Node, len(active))
	for _, n := range active {
		sbNode, err := h.singboxNode(n, usersByNode[n.ID])
		if err != nil {
			return agentConfigPayload{}, err
		}
		renderNodes = append(renderNodes, sbNode)
		renderByID[n.ID] = sbNode
	}

	clash, err := singbox.NewClashAPI(ports)
	if err != nil {
		return agentConfigPayload{}, err
	}
	config, err := singbox.Render(h.appKey, renderNodes, clash)
	if err != nil {
		return agentConfigPayload{}, err
	}

	out := make([]agentUserDTO, 0, len(users))
	for _, u := range users {
		nodeDTOs := make([]agentUserNodeDTO, 0, len(nodesByUser[u.ID]))
		for _, nodeID := range nodesByUser[u.ID] {
			sbNode := renderByID[nodeID]
			credential, err := agentCredential(h.appKey, sbNode, u.UUID)
			if err != nil {
				return agentConfigPayload{}, err
			}
			n := nodeByID[nodeID]
			nodeDTOs = append(nodeDTOs, agentUserNodeDTO{
				ID:         n.ID,
				Protocol:   n.Protocol,
				Port:       n.Port,
				Credential: credential,
			})
		}
		out = append(out, agentUserDTO{
			ID:         u.ID,
			UUID:       u.UUID,
			Status:     u.Status,
			QuotaBytes: u.QuotaBytes,
			UsedBytes:  u.UsedBytes,
			ExpiresAt:  rfc3339Ptr(u.ExpiresAt),
			Nodes:      nodeDTOs,
		})
	}

	return agentConfigPayload{Config: config, Users: out}, nil
}

func (h *Handler) singboxNode(n repo.Node, users []singbox.User) (singbox.Node, error) {
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(n.Settings), &settings); err != nil {
		return singbox.Node{}, err
	}
	secret := map[string]any{}
	if len(n.SecretEnc) > 0 {
		plain, err := secrets.Decrypt(h.appKey, n.SecretEnc)
		if err != nil {
			h.logger.Error("decrypt node secret failed", "node_id", n.ID)
			return singbox.Node{}, err
		}
		if err := json.Unmarshal(plain, &secret); err != nil {
			return singbox.Node{}, err
		}
	}
	return singbox.Node{
		ID:       n.ID,
		Name:     n.Name,
		Protocol: n.Protocol,
		Port:     n.Port,
		Settings: settings,
		Secret:   secret,
		Users:    users,
	}, nil
}

func agentCredential(appKey []byte, n singbox.Node, userUUID string) (map[string]any, error) {
	switch n.Protocol {
	case singbox.ProtocolShadowsocks:
		method := singbox.SettingString(n.Settings, "method")
		password, err := singbox.DeriveSSPassword(appKey, n.ID, userUUID, method)
		if err != nil {
			return nil, err
		}
		return map[string]any{"contract": "ss-cred-v1", "method": method, "password": password}, nil
	case singbox.ProtocolVLESS:
		return map[string]any{"contract": "uuid-v1", "uuid": userUUID, "flow": "xtls-rprx-vision"}, nil
	case singbox.ProtocolHysteria2:
		return map[string]any{"contract": "uuid-v1", "password": userUUID}, nil
	default:
		return nil, fmt.Errorf("unknown protocol %q", n.Protocol)
	}
}
