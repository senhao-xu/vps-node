package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
	"vps-node/internal/subscription"
)

type customNodeDTO struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	SourceType string  `json:"source_type"`
	Status     string  `json:"status"`
	HasCache   bool    `json:"has_cache"`
	FetchedAt  *string `json:"fetched_at"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// customNodeResultDTO is the create/update response: the DTO plus non-fatal
// parse warnings for links-type content.
type customNodeResultDTO struct {
	customNodeDTO
	Warnings []string `json:"warnings,omitempty"`
}

func toCustomNodeDTO(n repo.CustomNode) customNodeDTO {
	var fetchedAt *string
	if !n.FetchedAt.IsZero() {
		fetchedAt = rfc3339Ptr(&n.FetchedAt)
	}
	return customNodeDTO{
		ID:         n.ID,
		Name:       n.Name,
		SourceType: n.SourceType,
		Status:     n.Status,
		HasCache:   n.CachedContent != "",
		FetchedAt:  fetchedAt,
		CreatedAt:  rfc3339(n.CreatedAt),
		UpdatedAt:  rfc3339(n.UpdatedAt),
	}
}

func toCustomNodeDTOs(nodes []repo.CustomNode) []customNodeDTO {
	out := make([]customNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toCustomNodeDTO(n))
	}
	return out
}

func (h *Handler) handleCustomNodeList(w http.ResponseWriter, r *http.Request) {
	nodes, err := h.repo.ListCustomNodes(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"items": toCustomNodeDTOs(nodes)})
}

type customNodeRequest struct {
	Name    string  `json:"name"`
	Content *string `json:"content"`
	Status  string  `json:"status"`
}

type createCustomNodeRequest struct {
	Name       string `json:"name"`
	SourceType string `json:"source_type"`
	Content    string `json:"content"`
}

func validateCustomNodeName(name string) error {
	if name == "" || len(name) > 128 {
		return errValidation("name must be 1-128 characters")
	}
	return nil
}

func validateCustomNodeStatus(status string) error {
	if status != repo.CustomNodeStatusActive && status != repo.CustomNodeStatusDisabled {
		return errValidation("status must be active or disabled")
	}
	return nil
}

func validateCustomNodeContent(sourceType, content string) error {
	if strings.TrimSpace(content) == "" {
		return errValidation("content is required")
	}
	if sourceType == repo.CustomNodeSourceSubscription {
		u, err := url.Parse(strings.TrimSpace(content))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return errValidation("content must be a valid http(s) subscription URL")
		}
	}
	return nil
}

// linkWarnings parses each link line and reports the ones that cannot be
// converted into a Clash proxy. The entry is still stored; general-format
// subscriptions pass unparseable lines through unchanged.
func linkWarnings(content string) []string {
	warnings := []string{}
	for _, line := range subscription.SplitLinkLines(content) {
		if _, err := subscription.ParseShareURI(line); err != nil {
			warnings = append(warnings, line+": "+err.Error())
		}
	}
	return warnings
}

func (h *Handler) handleCustomNodeCreate(w http.ResponseWriter, r *http.Request) {
	var req createCustomNodeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validateCustomNodeName(req.Name); err != nil {
		writeErr(w, err)
		return
	}
	if req.SourceType != repo.CustomNodeSourceLinks && req.SourceType != repo.CustomNodeSourceSubscription {
		writeErr(w, errValidation("source_type must be links or subscription"))
		return
	}
	if err := validateCustomNodeContent(req.SourceType, req.Content); err != nil {
		writeErr(w, err)
		return
	}
	content := strings.TrimSpace(req.Content)
	enc, err := secrets.Encrypt(h.appKey, []byte(content))
	if err != nil {
		writeErr(w, err)
		return
	}
	id, err := h.repo.CreateCustomNode(r.Context(), repo.NewCustomNode{
		Name:       req.Name,
		SourceType: req.SourceType,
		ContentEnc: enc,
	})
	if err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("a custom node with this name already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	result := customNodeResultDTO{customNodeDTO: toCustomNodeDTO(n)}
	if req.SourceType == repo.CustomNodeSourceLinks {
		result.Warnings = linkWarnings(content)
	}
	httpx.WriteJSON(w, http.StatusCreated, result)
}

func (h *Handler) handleCustomNodeUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	existing, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req customNodeRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	name := existing.Name
	if trimmed := strings.TrimSpace(req.Name); trimmed != "" {
		if err := validateCustomNodeName(trimmed); err != nil {
			writeErr(w, err)
			return
		}
		name = trimmed
	}
	status := existing.Status
	if req.Status != "" {
		if err := validateCustomNodeStatus(req.Status); err != nil {
			writeErr(w, err)
			return
		}
		status = req.Status
	}
	var contentEnc []byte
	var warnings []string
	if req.Content != nil {
		if err := validateCustomNodeContent(existing.SourceType, *req.Content); err != nil {
			writeErr(w, err)
			return
		}
		content := strings.TrimSpace(*req.Content)
		contentEnc, err = secrets.Encrypt(h.appKey, []byte(content))
		if err != nil {
			writeErr(w, err)
			return
		}
		if existing.SourceType == repo.CustomNodeSourceLinks {
			warnings = linkWarnings(content)
		}
	}
	if err := h.repo.UpdateCustomNode(r.Context(), id, name, contentEnc, status); err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("a custom node with this name already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	n, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	result := customNodeResultDTO{customNodeDTO: toCustomNodeDTO(n), Warnings: warnings}
	httpx.WriteJSON(w, http.StatusOK, result)
}

func (h *Handler) handleCustomNodeDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.DeleteCustomNode(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type userCustomNodesResponse struct {
	CustomNodeIDs []int64         `json:"custom_node_ids"`
	CustomNodes   []customNodeDTO `json:"custom_nodes"`
}

func (h *Handler) handleUserCustomNodesGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	customNodeIDs, err := h.repo.ListCustomNodeIDsByUser(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	nodes, err := h.repo.ListCustomNodesByIDs(r.Context(), customNodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userCustomNodesResponse{CustomNodeIDs: customNodeIDs, CustomNodes: toCustomNodeDTOs(nodes)})
}

func (h *Handler) handleUserCustomNodesPut(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		CustomNodeIDs []int64 `json:"custom_node_ids"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if req.CustomNodeIDs == nil {
		writeErr(w, errInvalid("custom_node_ids is required"))
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	customNodeIDs, err := h.validateCustomNodeIDs(r.Context(), req.CustomNodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Custom nodes never reach the agent config, so no server revision bump.
	if err := h.repo.SetUserCustomNodes(r.Context(), id, customNodeIDs); err != nil {
		writeErr(w, err)
		return
	}
	nodes, err := h.repo.ListCustomNodesByIDs(r.Context(), customNodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userCustomNodesResponse{CustomNodeIDs: customNodeIDs, CustomNodes: toCustomNodeDTOs(nodes)})
}

func (h *Handler) validateCustomNodeIDs(ctx context.Context, customNodeIDs []int64) ([]int64, error) {
	unique := dedupeIDs(customNodeIDs)
	if len(unique) == 0 {
		return []int64{}, nil
	}
	nodes, err := h.repo.ListCustomNodesByIDs(ctx, unique)
	if err != nil {
		return nil, err
	}
	seen := make(map[int64]bool, len(nodes))
	for _, n := range nodes {
		seen[n.ID] = true
	}
	for _, id := range unique {
		if !seen[id] {
			return nil, errValidation("unknown custom_node_id")
		}
	}
	return unique, nil
}
