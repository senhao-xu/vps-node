package web

import (
	"net/http"

	"vps-node/internal/httpx"
)

type userNodesResponse struct {
	NodeIDs []int64   `json:"node_ids"`
	Nodes   []nodeDTO `json:"nodes"`
}

func (h *Handler) handleUserNodesGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	nodeIDs, err := h.repo.ListNodeIDsByUser(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	nodes, err := h.repo.ListNodesByIDs(r.Context(), nodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userNodesResponse{NodeIDs: nodeIDs, Nodes: toNodeDTOs(nodes)})
}

func (h *Handler) handleUserNodesPut(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		NodeIDs []int64 `json:"node_ids"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.NodeIDs == nil {
		writeErr(w, errInvalid("node_ids is required"))
		return
	}
	nodeIDs, err := h.validateNodeIDs(r.Context(), req.NodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.SetUserNodesAndBump(r.Context(), id, nodeIDs); err != nil {
		writeErr(w, err)
		return
	}

	nodes, err := h.repo.ListNodesByIDs(r.Context(), nodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userNodesResponse{NodeIDs: nodeIDs, Nodes: toNodeDTOs(nodes)})
}
