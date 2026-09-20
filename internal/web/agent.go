package web

import (
	"context"
	"net/http"
	"strings"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

const (
	ctxKeyAgentID       ctxKey = 2
	ctxKeyAgentServerID ctxKey = 3

	defaultHeartbeatIntervalSeconds = 30
	defaultSyncIntervalSeconds      = 30
	defaultTrafficIntervalSeconds   = 60

	maxVersionLength = 64
)

func errForbidden(msg string) *apiError {
	return &apiError{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}

func (h *Handler) requireAgent(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r)
		if token == "" {
			writeErr(w, errUnauthorized("agent token required"))
			return
		}
		agent, err := h.repo.GetAgentByTokenHash(r.Context(), adminauth.HashToken(token))
		if err != nil {
			if err == repo.ErrNotFound {
				writeErr(w, errUnauthorized("invalid agent token"))
				return
			}
			writeErr(w, err)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAgentID, agent.ID)
		ctx = context.WithValue(ctx, ctxKeyAgentServerID, agent.ServerID)
		next(w, r.WithContext(ctx))
	}
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, prefix))
}

func agentIDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxKeyAgentID).(int64)
	return id
}

func agentServerIDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxKeyAgentServerID).(int64)
	return id
}

func (h *Handler) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RegisterToken string `json:"register_token"`
		Version       string `json:"version"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.RegisterToken == "" {
		writeErr(w, errInvalid("register_token is required"))
		return
	}
	version := strings.TrimSpace(req.Version)
	if len(version) > maxVersionLength {
		writeErr(w, errValidation("version must be at most 64 characters"))
		return
	}

	server, err := h.repo.GetServerByRegisterTokenHash(r.Context(), adminauth.HashToken(req.RegisterToken))
	if err != nil {
		if err == repo.ErrNotFound {
			writeErr(w, errUnauthorized("invalid registration token"))
			return
		}
		writeErr(w, err)
		return
	}
	if server.RegisterTokenExpiresAt != nil && server.RegisterTokenExpiresAt.Before(time.Now()) {
		writeErr(w, errUnauthorized("registration token expired"))
		return
	}

	token, err := adminauth.NewToken()
	if err != nil {
		writeErr(w, err)
		return
	}
	agentID, err := h.repo.RegisterAgent(r.Context(), server.ID, adminauth.HashToken(token), version)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"agent_id":                   agentID,
		"agent_token":                token,
		"server_id":                  server.ID,
		"heartbeat_interval_seconds": h.agentHeartbeatIntervalSeconds,
		"sync_interval_seconds":      h.agentSyncIntervalSeconds,
		"traffic_interval_seconds":   h.agentTrafficIntervalSeconds,
	})
}

type heartbeatRequest struct {
	Version       string   `json:"version"`
	CPUPercent    *float64 `json:"cpu_percent"`
	MemoryPercent *float64 `json:"memory_percent"`
	DiskPercent   *float64 `json:"disk_percent"`
	UptimeSeconds *int64   `json:"uptime_seconds"`
}

func (h *Handler) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	agentID, serverID := agentIDFrom(r.Context()), agentServerIDFrom(r.Context())
	var req heartbeatRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	version := strings.TrimSpace(req.Version)
	if version == "" || len(version) > maxVersionLength {
		writeErr(w, errValidation("version must be 1-64 characters"))
		return
	}
	if req.CPUPercent == nil || req.MemoryPercent == nil || req.DiskPercent == nil || req.UptimeSeconds == nil {
		writeErr(w, errValidation("cpu_percent, memory_percent, disk_percent and uptime_seconds are required"))
		return
	}
	if !validPercent(*req.CPUPercent) || !validPercent(*req.MemoryPercent) || !validPercent(*req.DiskPercent) {
		writeErr(w, errValidation("percent metrics must be between 0 and 100"))
		return
	}
	if *req.UptimeSeconds < 0 {
		writeErr(w, errValidation("uptime_seconds must be non-negative"))
		return
	}

	if err := h.repo.RecordHeartbeat(r.Context(), agentID, serverID, version,
		*req.CPUPercent, *req.MemoryPercent, *req.DiskPercent, *req.UptimeSeconds, time.Now().UTC()); err != nil {
		writeErr(w, err)
		return
	}
	revision, err := h.repo.GetServerRevision(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":                         true,
		"server_revision":            revision,
		"heartbeat_interval_seconds": h.agentHeartbeatIntervalSeconds,
	})
}

func validPercent(v float64) bool {
	return v >= 0 && v <= 100
}
