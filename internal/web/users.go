package web

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

var validUserStatuses = map[string]bool{
	repo.UserStatusActive:   true,
	repo.UserStatusDisabled: true,
	repo.UserStatusExpired:  true,
}

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,64}$`)

func validateUsername(raw string) (string, error) {
	username := strings.TrimSpace(raw)
	if username == "" {
		return "", errValidation("username is required")
	}
	if !usernamePattern.MatchString(username) {
		return "", errValidation("username must be 1-64 characters of letters, digits, '_', '-' or '.'")
	}
	return username, nil
}

type optInt64 struct {
	Set   bool
	Value int64
}

func (o *optInt64) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = 0
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}

type optString struct {
	Set   bool
	Value *string
}

func (o *optString) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	o.Value = &s
	return nil
}

func newUUIDv4() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

func (h *Handler) handleUserList(w http.ResponseWriter, r *http.Request) {
	page, err := parsePageQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	q := r.URL.Query()
	status := q.Get("status")
	if status != "" && !validUserStatuses[status] {
		writeErr(w, errInvalid("invalid status filter"))
		return
	}
	expiry := q.Get("expiry")
	if expiry != "" && expiry != "valid" && expiry != "expired" {
		writeErr(w, errInvalid("invalid expiry filter"))
		return
	}

	query := q.Get("query")
	filter := repo.UserFilter{
		Query:    query,
		Status:   status,
		Expiry:   expiry,
		Page:     page.Page,
		PageSize: page.PageSize,
	}
	if query != "" {
		filter.TokenHash = adminauth.HashToken(query)
	}

	users, total, err := h.repo.ListUsers(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}

	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	now := time.Now()
	freshCutoff := now.Add(-h.sessionFreshness(r.Context()))
	nodeCounts, err := h.repo.CountNodesByUserIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, err)
		return
	}
	sessionCounts, err := h.repo.CountFreshSessionsByUserIDs(r.Context(), ids, freshCutoff)
	if err != nil {
		writeErr(w, err)
		return
	}

	items := make([]userDTO, 0, len(users))
	for _, u := range users {
		items = append(items, toUserDTO(u, nodeCounts[u.ID], sessionCounts[u.ID]))
	}
	writePage(w, items, total, page)
}

type createUserRequest struct {
	Username   string  `json:"username"`
	QuotaBytes *int64  `json:"quota_bytes"`
	StartedAt  *string `json:"started_at"`
	ExpiresAt  *string `json:"expires_at"`
	NodeIDs    []int64 `json:"node_ids"`
}

type userCreatedDTO struct {
	userDetailDTO
	Token string `json:"token"`
}

func (h *Handler) handleUserCreate(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}

	quota := int64(0)
	if req.QuotaBytes != nil {
		if *req.QuotaBytes < 0 {
			writeErr(w, errValidation("quota_bytes must be >= 0"))
			return
		}
		quota = *req.QuotaBytes
	}
	username, err := validateUsername(req.Username)
	if err != nil {
		writeErr(w, err)
		return
	}
	startedAt, err := parseTimeBody(req.StartedAt, "started_at")
	if err != nil {
		writeErr(w, err)
		return
	}
	expiresAt, err := parseTimeBody(req.ExpiresAt, "expires_at")
	if err != nil {
		writeErr(w, err)
		return
	}
	nodeIDs, err := h.validateNodeIDs(r.Context(), req.NodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}

	uuid, err := newUUIDv4()
	if err != nil {
		writeErr(w, err)
		return
	}
	token, err := adminauth.NewToken()
	if err != nil {
		writeErr(w, err)
		return
	}

	id, err := h.repo.CreateUserWithNodes(r.Context(), repo.NewUser{
		UUID:       uuid,
		Username:   username,
		TokenHash:  adminauth.HashToken(token),
		Status:     repo.UserStatusActive,
		QuotaBytes: quota,
		StartedAt:  startedAt,
		ExpiresAt:  expiresAt,
	}, nodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}

	dto, err := h.userDetail(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, userCreatedDTO{userDetailDTO: dto, Token: token})
}

func (h *Handler) userDetail(ctx context.Context, id int64) (userDetailDTO, error) {
	u, err := h.repo.GetUser(ctx, id)
	if err != nil {
		return userDetailDTO{}, err
	}
	nodeMap, err := h.repo.CountNodesByUserIDs(ctx, []int64{id})
	if err != nil {
		return userDetailDTO{}, err
	}
	freshCutoff := time.Now().Add(-h.sessionFreshness(ctx))
	sessionMap, err := h.repo.CountFreshSessionsByUserIDs(ctx, []int64{id}, freshCutoff)
	if err != nil {
		return userDetailDTO{}, err
	}
	return toUserDetailDTO(u, nodeMap[id], sessionMap[id]), nil
}

func (h *Handler) handleUserGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	dto, err := h.userDetail(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}

type updateUserRequest struct {
	Status     *string   `json:"status"`
	Username   optString `json:"username"`
	QuotaBytes optInt64  `json:"quota_bytes"`
	StartedAt  optString `json:"started_at"`
	ExpiresAt  optString `json:"expires_at"`
}

func (h *Handler) handleUserUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}

	patch := repo.UserPatch{}
	if req.Status != nil {
		if !validUserStatuses[*req.Status] {
			writeErr(w, errInvalid("invalid status"))
			return
		}
		patch.SetStatus = true
		patch.Status = *req.Status
	}
	if req.Username.Set {
		if req.Username.Value == nil {
			writeErr(w, errValidation("username cannot be null"))
			return
		}
		username, err := validateUsername(*req.Username.Value)
		if err != nil {
			writeErr(w, err)
			return
		}
		patch.SetUsername = true
		patch.Username = username
	}
	if req.QuotaBytes.Set {
		if req.QuotaBytes.Value < 0 {
			writeErr(w, errValidation("quota_bytes must be >= 0"))
			return
		}
		patch.SetQuota = true
		patch.QuotaBytes = req.QuotaBytes.Value
	}
	if req.StartedAt.Set {
		t, err := parseTimeBody(req.StartedAt.Value, "started_at")
		if err != nil {
			writeErr(w, err)
			return
		}
		patch.SetStartedAt = true
		patch.StartedAt = t
	}
	if req.ExpiresAt.Set {
		t, err := parseTimeBody(req.ExpiresAt.Value, "expires_at")
		if err != nil {
			writeErr(w, err)
			return
		}
		patch.SetExpiresAt = true
		patch.ExpiresAt = t
	}

	if _, err := h.repo.UpdateUserAndBump(r.Context(), id, patch); err != nil {
		writeErr(w, err)
		return
	}
	dto, err := h.userDetail(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) handleUserDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.DeleteUserAndBump(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{})
}

func (h *Handler) handleUserResetToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	token, err := adminauth.NewToken()
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.ResetUserTokenHash(r.Context(), id, adminauth.HashToken(token)); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"token": token})
}

func (h *Handler) handleUserResetTraffic(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.ResetUserTrafficAndBump(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	dto, err := h.userDetail(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) handleUserExpireNow(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	now := time.Now()
	if _, err := h.repo.UpdateUserAndBump(r.Context(), id, repo.UserPatch{
		SetStatus:    true,
		Status:       repo.UserStatusExpired,
		SetExpiresAt: true,
		ExpiresAt:    &now,
	}); err != nil {
		writeErr(w, err)
		return
	}
	dto, err := h.userDetail(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) validateNodeIDs(ctx context.Context, nodeIDs []int64) ([]int64, error) {
	unique := dedupeIDs(nodeIDs)
	if len(unique) == 0 {
		return []int64{}, nil
	}
	nodes, err := h.repo.ListNodesByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]repo.Node, len(nodes))
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, id := range unique {
		n, ok := byID[id]
		if !ok {
			return nil, errValidation("unknown node_id")
		}
		if n.Status != repo.NodeStatusActive {
			return nil, errValidation("node is not active")
		}
	}
	return unique, nil
}
