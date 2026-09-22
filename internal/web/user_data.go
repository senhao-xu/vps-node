package web

import (
	"net/http"

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

func (h *Handler) handleUserDevices(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	devices, err := h.repo.ListDevicesByUser(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	items := make([]deviceDTO, 0, len(devices))
	for _, d := range devices {
		items = append(items, toDeviceDTO(d))
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) handleUserVisits(w http.ResponseWriter, r *http.Request) {
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
	filter, err := visitFilterFromQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	filter.UserID = id
	filter.Page = page.Page
	filter.PageSize = page.PageSize

	visits, total, err := h.repo.ListVisits(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	writePage(w, visitDTOs(visits), total, page)
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
			UploadBytes:   b.U,
			DownloadBytes: b.D,
		})
	}
	httpx.WriteJSON(w, http.StatusOK, trafficSeriesDTO{
		TotalUploadBytes:   upload,
		TotalDownloadBytes: download,
		Series:             series,
	})
}
