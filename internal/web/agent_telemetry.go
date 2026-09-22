package web

import (
	"context"
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
	maxVisitHostLen      = 253
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
	UserID     int64  `json:"user_id"`
	NodeID     int64  `json:"node_id"`
	U          int64  `json:"u"`
	D          int64  `json:"d"`
	RecordedAt string `json:"recorded_at"`
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
	for i, in := range req.Records {
		field := fmt.Sprintf("records[%d]", i)
		if in.UserID < 1 || in.NodeID < 1 {
			writeErr(w, errValidation(field+" user_id and node_id are required"))
			return
		}
		if in.U < 0 || in.D < 0 {
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
			UserID:    in.UserID,
			NodeID:    in.NodeID,
			ServerID:  serverID,
			U:         in.U,
			D:         in.D,
			CreatedAt: ts,
		})
	}

	count, _, err := h.repo.IngestTrafficBatch(r.Context(), agentID, req.BatchSeq, records)
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

type agentDeviceReport struct {
	UserID int64    `json:"user_id"`
	NodeID int64    `json:"node_id"`
	IPs    []string `json:"ips"`
	Online int      `json:"online"`
}

func (h *Handler) handleAgentDevices(w http.ResponseWriter, r *http.Request) {
	agentID, serverID := agentIDFrom(r.Context()), agentServerIDFrom(r.Context())
	var req struct {
		BatchSeq   int64               `json:"batch_seq"`
		RecordedAt string              `json:"recorded_at"`
		Devices    []agentDeviceReport `json:"devices"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.BatchSeq < 1 {
		writeErr(w, errValidation("batch_seq must be a positive integer"))
		return
	}
	if len(req.Devices) > maxAgentBatchRecords {
		writeErr(w, errTooLarge(fmt.Sprintf("batch exceeds %d devices", maxAgentBatchRecords)))
		return
	}
	seenAt, err := parseAgentTimestamp(req.RecordedAt, time.Now())
	if err != nil {
		writeErr(w, errValidation("recorded_at "+err.Error()))
		return
	}
	nodeSet, pairSet, err := h.agentScopes(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}

	devices := make([]repo.NewOnlineDevice, 0, len(req.Devices))
	onlineByUser := make(map[int64]int)
	for i, in := range req.Devices {
		field := fmt.Sprintf("devices[%d]", i)
		if in.UserID < 1 || in.NodeID < 1 {
			writeErr(w, errValidation(field+" user_id and node_id are required"))
			return
		}
		if in.Online < 0 {
			writeErr(w, errValidation(field+" online must be non-negative"))
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
		seenIPs := map[string]bool{}
		for j, ip := range in.IPs {
			if ip == "" || len(ip) > maxIPStrLen {
				writeErr(w, errValidation(fmt.Sprintf("%s.ips[%d] must be 1-64 characters", field, j)))
				return
			}
			if seenIPs[ip] {
				continue
			}
			seenIPs[ip] = true
			devices = append(devices, repo.NewOnlineDevice{UserID: in.UserID, NodeID: in.NodeID, IP: ip, Online: in.Online})
		}
		onlineByUser[in.UserID] += in.Online
	}

	count, _, err := h.repo.IngestDeviceBatch(r.Context(), agentID, req.BatchSeq, serverID, devices, onlineByUser, seenAt.Unix())
	if errors.Is(err, repo.ErrConflict) {
		count, _, err = h.repo.DeviceBatchCount(r.Context(), agentID, req.BatchSeq)
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"accepted":  true,
		"batch_seq": req.BatchSeq,
		"devices":   count,
	})
}

type agentVisitRecord struct {
	UserID     int64  `json:"user_id"`
	NodeID     int64  `json:"node_id"`
	DestHost   string `json:"dest_host"`
	DestPort   int    `json:"dest_port"`
	Network    string `json:"network"`
	ClientIP   string `json:"client_ip"`
	RecordedAt string `json:"recorded_at"`
}

func (h *Handler) handleAgentVisits(w http.ResponseWriter, r *http.Request) {
	agentID, serverID := agentIDFrom(r.Context()), agentServerIDFrom(r.Context())
	var req struct {
		BatchSeq int64              `json:"batch_seq"`
		Records  []agentVisitRecord `json:"records"`
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
	if !h.visitsCollectionEnabled(r.Context()) {
		httpx.WriteJSON(w, http.StatusOK, map[string]any{
			"accepted":  false,
			"batch_seq": req.BatchSeq,
			"records":   0,
		})
		return
	}
	nodeSet, pairSet, err := h.agentScopes(r.Context(), serverID)
	if err != nil {
		writeErr(w, err)
		return
	}

	now := time.Now()
	records := make([]repo.NewVisitRecord, 0, len(req.Records))
	for i, in := range req.Records {
		field := fmt.Sprintf("records[%d]", i)
		if in.UserID < 1 || in.NodeID < 1 {
			writeErr(w, errValidation(field+" user_id and node_id are required"))
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
		host := strings.ToLower(strings.TrimSpace(in.DestHost))
		if host == "" || len(host) > maxVisitHostLen {
			writeErr(w, errValidation(fmt.Sprintf("%s dest_host must be 1-%d characters", field, maxVisitHostLen)))
			return
		}
		if in.DestPort < 0 || in.DestPort > 65535 {
			writeErr(w, errValidation(field+" dest_port must be between 0 and 65535"))
			return
		}
		if in.Network != "" && in.Network != "tcp" && in.Network != "udp" {
			writeErr(w, errValidation(field+" network must be tcp, udp, or empty"))
			return
		}
		if len(in.ClientIP) > maxIPStrLen {
			writeErr(w, errValidation(fmt.Sprintf("%s client_ip must be at most %d characters", field, maxIPStrLen)))
			return
		}
		ts, err := parseAgentTimestamp(in.RecordedAt, now)
		if err != nil {
			writeErr(w, errValidation(field+" recorded_at "+err.Error()))
			return
		}
		records = append(records, repo.NewVisitRecord{
			UserID:    in.UserID,
			NodeID:    in.NodeID,
			ServerID:  serverID,
			DestHost:  host,
			DestPort:  in.DestPort,
			Network:   in.Network,
			ClientIP:  in.ClientIP,
			CreatedAt: ts,
		})
	}

	count, _, err := h.repo.IngestVisitBatch(r.Context(), agentID, req.BatchSeq, serverID, records)
	if errors.Is(err, repo.ErrConflict) {
		count, _, err = h.repo.VisitBatchCount(r.Context(), agentID, req.BatchSeq)
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
