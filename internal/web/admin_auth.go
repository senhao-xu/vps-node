package web

import (
	"errors"
	"net/http"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

func (h *Handler) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.Username == "" || req.Password == "" {
		writeErr(w, errInvalid("username and password are required"))
		return
	}

	key := clientIP(r) + "|" + req.Username
	if h.limiter.Blocked(key, time.Now()) {
		writeErr(w, &apiError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "too many login attempts, try again later"})
		return
	}

	admin, err := h.repo.GetAdminByUsername(r.Context(), req.Username)
	if errors.Is(err, repo.ErrNotFound) {
		adminauth.CheckDummyPassword(req.Password)
		h.limiter.Fail(key, time.Now())
		writeErr(w, errUnauthorized("invalid username or password"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	if !adminauth.CheckPassword(admin.PasswordHash, req.Password) {
		h.limiter.Fail(key, time.Now())
		writeErr(w, errUnauthorized("invalid username or password"))
		return
	}

	h.limiter.Reset(key)
	token, err := h.sessions.Create(r.Context(), admin.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	adminauth.SetSessionCookie(w, token, h.sessions.Secure)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"admin": map[string]any{"id": admin.ID, "username": admin.Username},
	})
}

func (h *Handler) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(adminauth.CookieName); err == nil {
		if err := h.sessions.Revoke(r.Context(), cookie.Value); err != nil {
			writeErr(w, err)
			return
		}
	}
	adminauth.ClearSessionCookie(w, h.sessions.Secure)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{})
}

func (h *Handler) handleMe(w http.ResponseWriter, r *http.Request) {
	admin, err := h.repo.GetAdmin(r.Context(), adminIDFrom(r.Context()))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"admin": map[string]any{"id": admin.ID, "username": admin.Username},
	})
}
