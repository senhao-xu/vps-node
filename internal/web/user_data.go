package web

import (
	"net/http"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

func toNodeDTOs(nodes []repo.Node) []nodeDTO {
	items := make([]nodeDTO, 0, len(nodes))
	for _, n := range nodes {
		items = append(items, toNodeDTO(n))
	}
	return items
}

func (h *Handler) handleUserSessions(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}

	includeStale := r.URL.Query().Get("include_stale") == "true" || r.URL.Query().Get("include_stale") == "1"
	var cutoff time.Time
	if !includeStale {
		cutoff = time.Now().Add(-h.sessionFreshness(r.Context()))
	}
	sessions, err := h.repo.ListSessionsByUser(r.Context(), id, cutoff)
	if err != nil {
		writeErr(w, err)
		return
	}
	items := make([]sessionDTO, 0, len(sessions))
	for _, s := range sessions {
		items = append(items, toSessionDTO(s))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) handleUserConnectionLogs(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	page, err := parsePageQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	from, err := parseTimeParam(r, "from")
	if err != nil {
		writeErr(w, err)
		return
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		writeErr(w, err)
		return
	}

	logs, total, err := h.repo.ListConnectionLogs(r.Context(), repo.LogFilter{
		UserID:   id,
		From:     from,
		To:       to,
		Page:     page.Page,
		PageSize: page.PageSize,
	})
	if err != nil {
		writeErr(w, err)
		return
	}
	items := make([]connectionLogDTO, 0, len(logs))
	for _, l := range logs {
		items = append(items, toConnectionLogDTO(l))
	}
	writePage(w, items, total, page)
}

func (h *Handler) handleUserTraffic(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	from, err := parseTimeParam(r, "from")
	if err != nil {
		writeErr(w, err)
		return
	}
	to, err := parseTimeParam(r, "to")
	if err != nil {
		writeErr(w, err)
		return
	}
	bucket := r.URL.Query().Get("bucket")
	if bucket == "" {
		bucket = "day"
	}
	if bucket != "hour" && bucket != "day" {
		writeErr(w, errInvalid("invalid bucket, want hour or day"))
		return
	}
	bucketSeconds := int64(86400)
	if bucket == "hour" {
		bucketSeconds = 3600
	}

	filter := repo.TrafficFilter{UserID: id, From: from, To: to}
	upload, download, err := h.repo.SumTraffic(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	buckets, err := h.repo.SumTrafficBuckets(r.Context(), filter, bucketSeconds)
	if err != nil {
		writeErr(w, err)
		return
	}
	series := make([]trafficBucketDTO, 0, len(buckets))
	for _, b := range buckets {
		series = append(series, trafficBucketDTO{
			BucketStart:   rfc3339(b.BucketStart),
			UploadBytes:   b.UploadBytes,
			DownloadBytes: b.DownloadBytes,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, trafficSeriesDTO{
		TotalUploadBytes:   upload,
		TotalDownloadBytes: download,
		Series:             series,
	})
}
