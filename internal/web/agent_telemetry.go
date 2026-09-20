package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

const (
	maxAgentBatchRecords = 1000
	agentTimestampWindow = 24 * time.Hour
	maxIPStrLen          = 64
	maxProtocolLen       = 32
)

type pairKey struct {
	user int64
	node int64
}

func (h *Handler) agentScopes(ctx context.Context, serverID int64) (map[int64]bool, map[pairKey]bool, error) {
	nodes, err := h.repo.ListNodesByServer(ctx, serverID)
	if err != nil {
		return nil, nil, err
	}
	nodeSet := make(map[int64]bool, len(nodes))
	for _, n := range nodes {
		nodeSet[n.ID] = true
	}
	pairs, err := h.repo.ListUserNodePairsByServer(ctx, serverID)
	if err != nil {
		return nil, nil, err
	}
	pairSet := make(map[pairKey]bool, len(pairs))
	for _, p := range pairs {
		pairSet[pairKey{p.UserID, p.NodeID}] = true
	}
	return nodeSet, pairSet, nil
}

func parseAgentTimestamp(raw string, now time.Time) (time.Time, error) {
	if raw == "" {
		return time.Time{}, errors.New("is required")
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errors.New("must be an RFC3339 timestamp")
	}
	if now.Sub(t) > agentTimestampWindow || t.Sub(now) > agentTimestampWindow {
		return time.Time{}, errors.New("is outside the acceptance window")
	}
	return t, nil
}

type agentTrafficRecord struct {
	UserID        int64  `json:"user_id"`
	NodeID        int64  `json:"node_id"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	RecordedAt    string `json:"recorded_at"`
}

func (h *Handler) handleAgentTraffic(w http.ResponseWriter, r *http.Request) {
	agentID, serverID := agentIDFrom(r.Context()), agentServerIDFrom(r.Context())
	var req struct {
		BatchSeq int64                `json:"batch_seq"`
		Records  []agentTrafficRecord `json:"records"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.BatchSeq < 1 {
		writeErr(w, errValidation("batch_seq must be a positive integer"))
		return
	}
	if len(req.Records) > maxAgentBatchRecords {
		writeErr(w, errTooLarge(fmt.Sprintf("batch exceeds %d records", maxAgentBatchRecords)))
		return
	}
	nodeSet, pairSet, err := h.agentScopes(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}

	now := time.Now()
	records := make([]repo.NewTrafficRecord, 0, len(req.Records))
	deltas := make(map[int64]int64)
	for i, in := range req.Records {
		field := fmt.Sprintf("records[%d]", i)
		if in.UserID < 1 || in.NodeID < 1 {
			writeErr(w, errValidation(field+" user_id and node_id are required"))
			return
		}
		if in.UploadBytes < 0 || in.DownloadBytes < 0 {
			writeErr(w, errValidation(field+" counters must be non-negative"))
			return
		}
		if !nodeSet[in.NodeID] {
			writeErr(w, errValidation(field+" node does not belong to this server"))
			return
		}
		if !pairSet[pairKey{in.UserID, in.NodeID}] {
			writeErr(w, errValidation(field+" user is not authorized on this node"))
			return
		}
		ts, err := parseAgentTimestamp(in.RecordedAt, now)
		if err != nil {
			writeErr(w, errValidation(field+" recorded_at "+err.Error()))
			return
		}
		records = append(records, repo.NewTrafficRecord{
			UserID:        in.UserID,
			NodeID:        in.NodeID,
			ServerID:      serverID,
			UploadBytes:   in.UploadBytes,
			DownloadBytes: in.DownloadBytes,
			CreatedAt:     ts,
		})
		deltas[in.UserID] += in.UploadBytes + in.DownloadBytes
	}

	count, _, err := h.repo.IngestTrafficBatch(r.Context(), agentID, req.BatchSeq, records, deltas)
	if errors.Is(err, repo.ErrConflict) {
		count, _, err = h.repo.TrafficBatchCount(r.Context(), agentID, req.BatchSeq)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"batch_seq": req.BatchSeq,
		"records":   count,
	})
}

type agentSessionReport struct {
	UserID        int64  `json:"user_id"`
	NodeID        int64  `json:"node_id"`
	IP            string `json:"ip"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	ConnectedAt   string `json:"connected_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

func (h *Handler) handleAgentSessions(w http.ResponseWriter, r *http.Request) {
	serverID := agentServerIDFrom(r.Context())
	var req struct {
		ReportedAt string               `json:"reported_at"`
		Sessions   []agentSessionReport `json:"sessions"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.ReportedAt == "" {
		writeErr(w, errValidation("reported_at is required"))
		return
	}
	if _, err := time.Parse(time.RFC3339, req.ReportedAt); err != nil {
		writeErr(w, errValidation("reported_at must be an RFC3339 timestamp"))
		return
	}
	nodeSet, pairSet, err := h.agentScopes(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}

	sessions := make([]repo.NewSession, 0, len(req.Sessions))
	for i, in := range req.Sessions {
		field := fmt.Sprintf("sessions[%d]", i)
		if in.UserID < 1 || in.NodeID < 1 {
			writeErr(w, errValidation(field+" user_id and node_id are required"))
			return
		}
		if in.IP == "" || len(in.IP) > maxIPStrLen {
			writeErr(w, errValidation(field+" ip must be 1-64 characters"))
			return
		}
		if in.UploadBytes < 0 || in.DownloadBytes < 0 {
			writeErr(w, errValidation(field+" counters must be non-negative"))
			return
		}
		if !nodeSet[in.NodeID] {
			writeErr(w, errValidation(field+" node does not belong to this server"))
			return
		}
		if !pairSet[pairKey{in.UserID, in.NodeID}] {
			writeErr(w, errValidation(field+" user is not authorized on this node"))
			return
		}
		connectedAt, err := parseAgentTimestamp(in.ConnectedAt, time.Now())
		if err != nil {
			writeErr(w, errValidation(field+" connected_at "+err.Error()))
			return
		}
		lastSeenAt, err := parseAgentTimestamp(in.LastSeenAt, time.Now())
		if err != nil {
			writeErr(w, errValidation(field+" last_seen_at "+err.Error()))
			return
		}
		if connectedAt.After(lastSeenAt) {
			writeErr(w, errValidation(field+" connected_at must not be after last_seen_at"))
			return
		}
		sessions = append(sessions, repo.NewSession{
			UserID:        in.UserID,
			NodeID:        in.NodeID,
			ServerID:      serverID,
			IP:            in.IP,
			UploadBytes:   in.UploadBytes,
			DownloadBytes: in.DownloadBytes,
			ConnectedAt:   connectedAt,
			LastSeenAt:    lastSeenAt,
		})
	}

	if err := h.repo.ReplaceServerSessions(r.Context(), serverID, sessions); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accepted": true,
		"sessions": len(sessions),
	})
}

type agentLogReport struct {
	UserID        int64   `json:"user_id"`
	NodeID        int64   `json:"node_id"`
	IP            string  `json:"ip"`
	Protocol      string  `json:"protocol"`
	UploadBytes   int64   `json:"upload_bytes"`
	DownloadBytes int64   `json:"download_bytes"`
	ConnectedAt   string  `json:"connected_at"`
	ClosedAt      *string `json:"closed_at"`
	Status        string  `json:"status"`
}

func decodeJSONStrict(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return errTooLarge("request body too large")
		}
		if strings.Contains(err.Error(), "unknown field") {
			return errValidation(err.Error())
		}
		return errInvalid("malformed JSON body")
	}
	return nil
}

func (h *Handler) handleAgentConnectionLogs(w http.ResponseWriter, r *http.Request) {
	agentID, serverID := agentIDFrom(r.Context()), agentServerIDFrom(r.Context())
	var req struct {
		BatchSeq int64            `json:"batch_seq"`
		Logs     []agentLogReport `json:"logs"`
	}
	if err := decodeJSONStrict(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.BatchSeq < 1 {
		writeErr(w, errValidation("batch_seq must be a positive integer"))
		return
	}
	if len(req.Logs) > maxAgentBatchRecords {
		writeErr(w, errTooLarge(fmt.Sprintf("batch exceeds %d logs", maxAgentBatchRecords)))
		return
	}
	nodeSet, pairSet, err := h.agentScopes(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}

	now := time.Now()
	logs := make([]repo.NewConnectionLog, 0, len(req.Logs))
	for i, in := range req.Logs {
		field := fmt.Sprintf("logs[%d]", i)
		if in.UserID < 1 || in.NodeID < 1 {
			writeErr(w, errValidation(field+" user_id and node_id are required"))
			return
		}
		if in.IP == "" || len(in.IP) > maxIPStrLen {
			writeErr(w, errValidation(field+" ip must be 1-64 characters"))
			return
		}
		if in.Protocol == "" || len(in.Protocol) > maxProtocolLen {
			writeErr(w, errValidation(field+" protocol must be 1-32 characters"))
			return
		}
		if in.Status != "active" && in.Status != "closed" {
			writeErr(w, errInvalid(field+" status must be active or closed"))
			return
		}
		if in.UploadBytes < 0 || in.DownloadBytes < 0 {
			writeErr(w, errValidation(field+" counters must be non-negative"))
			return
		}
		if !nodeSet[in.NodeID] {
			writeErr(w, errValidation(field+" node does not belong to this server"))
			return
		}
		if !pairSet[pairKey{in.UserID, in.NodeID}] {
			writeErr(w, errValidation(field+" user is not authorized on this node"))
			return
		}
		connectedAt, err := parseAgentTimestamp(in.ConnectedAt, now)
		if err != nil {
			writeErr(w, errValidation(field+" connected_at "+err.Error()))
			return
		}
		var closedAt *time.Time
		if in.ClosedAt != nil {
			t, err := parseAgentTimestamp(*in.ClosedAt, now)
			if err != nil {
				writeErr(w, errValidation(field+" closed_at "+err.Error()))
				return
			}
			closedAt = &t
		}
		logs = append(logs, repo.NewConnectionLog{
			UserID:        in.UserID,
			NodeID:        in.NodeID,
			ServerID:      serverID,
			IP:            in.IP,
			Protocol:      in.Protocol,
			UploadBytes:   in.UploadBytes,
			DownloadBytes: in.DownloadBytes,
			ConnectedAt:   connectedAt,
			ClosedAt:      closedAt,
			Status:        in.Status,
		})
	}

	count, _, err := h.repo.IngestLogBatch(r.Context(), agentID, req.BatchSeq, logs)
	if errors.Is(err, repo.ErrConflict) {
		count, _, err = h.repo.LogBatchCount(r.Context(), agentID, req.BatchSeq)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"batch_seq": req.BatchSeq,
		"logs":      count,
	})
}
