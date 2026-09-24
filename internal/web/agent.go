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
		key := bearerToken(r)
		if key == "" {
			writeErr(w, errUnauthorized("agent key required"))
			return
		}
		agent, err := h.repo.GetAgentByKeyHash(r.Context(), adminauth.HashToken(key))
		if err != nil {
			if err == repo.ErrNotFound {
				writeErr(w, errUnauthorized("invalid agent key"))
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
	trafficSeq, err := h.repo.LastTrafficSeq(r.Context(), agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	deviceSeq, err := h.repo.LastDeviceSeq(r.Context(), agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	visitSeq, err := h.repo.LastVisitSeq(r.Context(), agentID)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":                         true,
		"server_id":                  serverID,
		"server_revision":            revision,
		"heartbeat_interval_seconds": h.agentHeartbeatIntervalSeconds,
		"traffic_seq":                trafficSeq,
		"device_seq":                 deviceSeq,
		"visit_seq":                  visitSeq,
	})
}

func validPercent(v float64) bool {
	return v >= 0 && v <= 100
}
