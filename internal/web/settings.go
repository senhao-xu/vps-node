package web

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

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
	RetentionAggregateDays      optInt64 `json:"retention_aggregate_days"`
	RetentionVisitDays          optInt64 `json:"retention_visit_days"`
	RetentionVisitAggregateDays optInt64 `json:"retention_visit_aggregate_days"`
	CollectionVisits            *bool    `json:"collection_visits"`
	ServerOfflineAfterSeconds   optInt64 `json:"server_offline_after_seconds"`
	SubscribeURLs               *string  `json:"subscribe_urls"`
	SubscribePath               *string  `json:"subscribe_path"`
	SubscribeName               *string  `json:"subscribe_name"`
	ClashMetaTemplate           *string  `json:"clash_meta_template"`
}

var settingsRanges = map[string]struct {
	Min int
	Max int
}{
	settingRetentionAggregate:      {Min: 1, Max: 3650},
	settingRetentionVisit:          {Min: 1, Max: 3650},
	settingRetentionVisitAggregate: {Min: 1, Max: 3650},
	settingServerOfflineAfter:      {Min: 10, Max: 86400},
}

func (h *Handler) visitsCollectionEnabled(ctx context.Context) bool {
	raw, err := h.repo.ListSettings(ctx)
	if err != nil {
		return true
	}
	return settingBool(raw, settingCollectionVisits, true)
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
	if err := applyInt(req.RetentionAggregateDays, settingRetentionAggregate); err != nil {
		writeErr(w, err)
		return
	}
	if err := applyInt(req.RetentionVisitDays, settingRetentionVisit); err != nil {
		writeErr(w, err)
		return
	}
	if err := applyInt(req.RetentionVisitAggregateDays, settingRetentionVisitAggregate); err != nil {
		writeErr(w, err)
		return
	}
	if err := applyInt(req.ServerOfflineAfterSeconds, settingServerOfflineAfter); err != nil {
		writeErr(w, err)
		return
	}
	if req.CollectionVisits != nil {
		updates[settingCollectionVisits] = strconv.FormatBool(*req.CollectionVisits)
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
	if req.SubscribeName != nil {
		name := strings.TrimSpace(*req.SubscribeName)
		if utf8.RuneCountInString(name) > 64 || strings.ContainsAny(name, "\r\n") || strings.ContainsRune(name, 0x7f) {
			writeErr(w, errValidation("subscribe_name must be at most 64 characters without line breaks"))
			return
		}
		updates[settingSubscribeName] = name
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
