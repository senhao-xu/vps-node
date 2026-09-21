package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"vps-node/internal/httpx"
	"vps-node/internal/subscription"
)

var subscribePathPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

func validateSubscribeURLs(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	for _, part := range strings.Split(raw, ",") {
		u, err := url.Parse(strings.TrimSpace(part))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return errValidation("subscribe_urls must contain comma-separated HTTP(S) origins")
		}
	}
	return nil
}

type settingsUpdateRequest struct {
	RetentionRawLogDays       optInt64 `json:"retention_raw_log_days"`
	RetentionAggregateDays    optInt64 `json:"retention_aggregate_days"`
	CollectionConnectionLogs  *bool    `json:"collection_connection_logs"`
	SessionFreshnessSeconds   optInt64 `json:"session_freshness_seconds"`
	ServerOfflineAfterSeconds optInt64 `json:"server_offline_after_seconds"`
	SubscribeURLs             *string  `json:"subscribe_urls"`
	SubscribePath             *string  `json:"subscribe_path"`
	ClashMetaTemplate         *string  `json:"clash_meta_template"`
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
	if req.SubscribeURLs != nil {
		if err := validateSubscribeURLs(*req.SubscribeURLs); err != nil {
			writeErr(w, err)
			return
		}
		updates[settingSubscribeURLs] = strings.TrimSpace(*req.SubscribeURLs)
	}
	if req.SubscribePath != nil {
		if !subscribePathPattern.MatchString(*req.SubscribePath) {
			writeErr(w, errValidation("subscribe_path must be a safe path segment"))
			return
		}
		updates[settingSubscribePath] = *req.SubscribePath
	}
	if req.ClashMetaTemplate != nil {
		if err := subscription.ValidateTemplate(*req.ClashMetaTemplate); err != nil {
			writeErr(w, errValidation(err.Error()))
			return
		}
		updates[settingClashTemplate] = *req.ClashMetaTemplate
	}

	if err := h.repo.SetSettings(r.Context(), updates); err != nil {
		writeErr(w, err)
		return
	}

	dto, err := h.settingsDTO(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}
