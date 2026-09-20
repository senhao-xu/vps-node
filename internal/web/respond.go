package web

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"strconv"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

const maxBodyBytes = 1 << 20

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	return e.Code + ": " + e.Message
}

func errInvalid(msg string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: "invalid_request", Message: msg}
}

func errUnauthorized(msg string) *apiError {
	return &apiError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: msg}
}

func errValidation(msg string) *apiError {
	return &apiError{Status: http.StatusUnprocessableEntity, Code: "validation", Message: msg}
}

func errNotFound(msg string) *apiError {
	return &apiError{Status: http.StatusNotFound, Code: "not_found", Message: msg}
}

func errConflict(msg string) *apiError {
	return &apiError{Status: http.StatusConflict, Code: "conflict", Message: msg}
}

func errTooLarge(msg string) *apiError {
	return &apiError{Status: http.StatusRequestEntityTooLarge, Code: "payload_too_large", Message: msg}
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *apiError
	if errors.As(err, &ae) {
		httpx.WriteError(w, ae.Status, ae.Code, ae.Message)
		return
	}
	switch {
	case errors.Is(err, repo.ErrNotFound):
		httpx.WriteError(w, http.StatusNotFound, "not_found", "resource not found")
	case errors.Is(err, repo.ErrConflict):
		httpx.WriteError(w, http.StatusConflict, "conflict", "resource conflict")
	default:
		httpx.WriteError(w, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return errTooLarge("request body too large")
		}
		return errInvalid("malformed JSON body")
	}
	return nil
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errInvalid("invalid resource id")
	}
	return id, nil
}

type pageQuery struct {
	Page     int
	PageSize int
}

func parsePageQuery(r *http.Request) (pageQuery, error) {
	p := pageQuery{Page: 1, PageSize: 20}
	if v := r.URL.Query().Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, errInvalid("invalid page")
		}
		p.Page = n
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return p, errInvalid("invalid page_size")
		}
		p.PageSize = n
	}
	return p, nil
}

func writePage(w http.ResponseWriter, items any, total int64, page pageQuery) {
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"items":     items,
		"total":     total,
		"page":      page.Page,
		"page_size": page.PageSize,
	})
}

func parseTimeParam(r *http.Request, name string) (*time.Time, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, errInvalid("invalid " + name + " timestamp, want RFC3339")
	}
	return &t, nil
}

func parseTimeBody(raw *string, field string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	if *raw == "" {
		return nil, errValidation(field + " must be an RFC3339 timestamp or null")
	}
	t, err := time.Parse(time.RFC3339, *raw)
	if err != nil {
		return nil, errValidation(field + " must be an RFC3339 timestamp or null")
	}
	return &t, nil
}

func roundPercent(used, quota int64) float64 {
	if quota <= 0 {
		return 0
	}
	p := float64(used) / float64(quota) * 100
	if p < 0 {
		return 0
	}
	if math.IsNaN(p) || math.IsInf(p, 0) {
		return 0
	}
	return math.Round(p*100) / 100
}
