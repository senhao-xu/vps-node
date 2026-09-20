package web

import (
	"net/http"
	"strconv"

	"vps-node/internal/httpx"
)

type settingsUpdateRequest struct {
	RetentionRawLogDays       optInt64 `json:"retention_raw_log_days"`
	RetentionAggregateDays    optInt64 `json:"retention_aggregate_days"`
	CollectionConnectionLogs  *bool    `json:"collection_connection_logs"`
	SessionFreshnessSeconds   optInt64 `json:"session_freshness_seconds"`
	ServerOfflineAfterSeconds optInt64 `json:"server_offline_after_seconds"`
}

var settingsRanges = map[string]struct {
	Min int
	Max int
}{
	settingRetentionRawLog:    {Min: 1, Max: 3650},
	settingRetentionAggregate: {Min: 1, Max: 3650},
	settingSessionFreshness:   {Min: 10, Max: 86400},
	settingServerOfflineAfter: {Min: 10, Max: 86400},
}

func (h *Handler) handleSettingsGet(w http.ResponseWriter, r *http.Request) {
	dto, err := h.settingsDTO(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) handleSettingsPut(w http.ResponseWriter, r *http.Request) {
	var req settingsUpdateRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}

	updates := map[string]string{}
	applyInt := func(field optInt64, key string) error {
		if !field.Set {
			return nil
		}
		rng := settingsRanges[key]
		if field.Value < int64(rng.Min) || field.Value > int64(rng.Max) {
			return errValidation(key + " out of range")
		}
		updates[key] = strconv.FormatInt(field.Value, 10)
		return nil
	}
	if err := applyInt(req.RetentionRawLogDays, settingRetentionRawLog); err != nil {
		writeErr(w, err)
		return
	}
	if err := applyInt(req.RetentionAggregateDays, settingRetentionAggregate); err != nil {
		writeErr(w, err)
		return
	}
	if err := applyInt(req.SessionFreshnessSeconds, settingSessionFreshness); err != nil {
		writeErr(w, err)
		return
	}
	if err := applyInt(req.ServerOfflineAfterSeconds, settingServerOfflineAfter); err != nil {
		writeErr(w, err)
		return
	}
	if req.CollectionConnectionLogs != nil {
		updates[settingCollectionLogs] = strconv.FormatBool(*req.CollectionConnectionLogs)
	}

	for key, value := range updates {
		if err := h.repo.SetSetting(r.Context(), key, value); err != nil {
			writeErr(w, err)
			return
		}
	}

	dto, err := h.settingsDTO(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}
