package web

import (
	"context"
	"errors"
	"net/http"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

type nodeUsersResponse struct {
	UserIDs []int64   `json:"user_ids"`
	Users   []userDTO `json:"users"`
}

func (h *Handler) nodeUsersPayload(ctx context.Context, nodeID int64) (nodeUsersResponse, error) {
	userIDs, err := h.repo.ListUserIDsByNode(ctx, nodeID)
	if err != nil {
		return nodeUsersResponse{}, err
	}
	users, err := h.repo.ListUsersAll(ctx)
	if err != nil {
		return nodeUsersResponse{}, err
	}
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	nodeCounts, err := h.repo.CountNodesByUserIDs(ctx, ids)
	if err != nil {
		return nodeUsersResponse{}, err
	}
	items := make([]userDTO, 0, len(users))
	for _, u := range users {
		items = append(items, toUserDTO(u, nodeCounts[u.ID]))
	}
	return nodeUsersResponse{UserIDs: userIDs, Users: items}, nil
}

func (h *Handler) handleNodeUsersGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetNode(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	payload, err := h.nodeUsersPayload(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, payload)
}

func (h *Handler) handleNodeUsersPut(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		UserIDs []int64 `json:"user_ids"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.UserIDs == nil {
		writeErr(w, errInvalid("user_ids is required"))
		return
	}
	if _, err := h.repo.GetNode(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.validateUserIDs(r.Context(), req.UserIDs); err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.SetNodeUsersAndBump(r.Context(), id, req.UserIDs); err != nil {
		writeErr(w, err)
		return
	}
	payload, err := h.nodeUsersPayload(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, payload)
}

func (h *Handler) validateUserIDs(ctx context.Context, userIDs []int64) error {
	unique := dedupeIDs(userIDs)
	for _, id := range unique {
		_, err := h.repo.GetUser(ctx, id)
		if errors.Is(err, repo.ErrNotFound) {
			return errValidation("unknown user_id")
		}
		if err != nil {
			return err
		}
	}
	return nil
}
