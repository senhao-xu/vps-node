package web

import (
	"net/http"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

var validServerStatuses = map[string]bool{
	repo.ServerStatusActive:   true,
	repo.ServerStatusDisabled: true,
}

func (h *Handler) handleServerList(w http.ResponseWriter, r *http.Request) {
	page, err := parsePageQuery(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	servers, total, err := h.repo.ListServersPage(r.Context(), page.Page, page.PageSize)
	if err != nil {
		writeErr(w, err)
		return
	}

	ids := make([]int64, 0, len(servers))
	for _, s := range servers {
		ids = append(ids, s.ID)
	}
	offlineAfter := h.offlineAfter(r.Context())
	now := time.Now()
	nodeCounts, err := h.repo.CountNodesByServerIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, err)
		return
	}
	onlineUsers, err := h.repo.CountOnlineUsersByServerIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, err)
		return
	}

	items := make([]serverDTO, 0, len(servers))
	for _, s := range servers {
		dto := toServerDTO(s, nodeCounts[s.ID], onlineUsers[s.ID])
		dto.Status = effectiveServerStatus(s, offlineAfter, now)
		items = append(items, dto)
	}
	writePage(w, items, total, page)
}

func (h *Handler) handleServerCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateServerNameAddress(req.Name, req.Address); err != nil {
		writeErr(w, err)
		return
	}
	id, err := h.repo.CreateServer(r.Context(), req.Name, req.Address, repo.ServerStatusActive)
	if err != nil {
		writeErr(w, err)
		return
	}
	s, err := h.repo.GetServer(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, toServerDTO(s, 0, 0))
}

func validateServerNameAddress(name, address string) error {
	if name == "" || len(name) > 128 {
		return errValidation("name must be 1-128 characters")
	}
	if address == "" || len(address) > 255 {
		return errValidation("address must be 1-255 characters")
	}
	return nil
}

func (h *Handler) handleServerGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	s, err := h.repo.GetServer(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	revision, err := h.repo.GetServerRevision(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	nodes, err := h.repo.ListNodesByServer(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}

	var agent *agentInfoDTO
	if a, err := h.repo.GetAgentByServerID(r.Context(), id); err == nil {
		agent = &agentInfoDTO{
			ID:          a.ID,
			Version:     a.Version,
			LastSeenAt:  rfc3339Ptr(a.LastSeenAt),
			ConnectedAt: rfc3339(a.CreatedAt),
		}
	} else if err != repo.ErrNotFound {
		writeErr(w, err)
		return
	}

	offlineAfter := h.offlineAfter(r.Context())
	nodeCounts, err := h.repo.CountNodesByServerIDs(r.Context(), []int64{id})
	if err != nil {
		writeErr(w, err)
		return
	}
	onlineUsers, err := h.repo.CountOnlineUsersByServerIDs(r.Context(), []int64{id})
	if err != nil {
		writeErr(w, err)
		return
	}

	dto := serverDetailDTO{
		serverDTO: toServerDTO(s, nodeCounts[id], onlineUsers[id]),
		Revision:  revision,
		Agent:     agent,
		Nodes:     toNodeDTOs(nodes),
	}
	dto.Status = effectiveServerStatus(s, offlineAfter, time.Now())
	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) handleServerVisits(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetServer(r.Context(), id); err != nil {
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
	filter.ServerID = id
	filter.Page = page.Page
	filter.PageSize = page.PageSize

	visits, total, err := h.repo.ListVisits(r.Context(), filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	writePage(w, visitDTOs(visits), total, page)
}

func (h *Handler) handleServerUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	current, err := h.repo.GetServer(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		Name    *string `json:"name"`
		Address *string `json:"address"`
		Status  *string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}

	name, address, status := current.Name, current.Address, current.Status
	if req.Name != nil {
		name = *req.Name
	}
	if req.Address != nil {
		address = *req.Address
	}
	if req.Status != nil {
		if !validServerStatuses[*req.Status] {
			writeErr(w, errInvalid("invalid status, want active or disabled"))
			return
		}
		status = *req.Status
	}
	if err := validateServerNameAddress(name, address); err != nil {
		writeErr(w, err)
		return
	}

	s, err := h.repo.UpdateServerAndBump(r.Context(), id, name, address, status)
	if err != nil {
		writeErr(w, err)
		return
	}
	nodeCounts, err := h.repo.CountNodesByServerIDs(r.Context(), []int64{id})
	if err != nil {
		writeErr(w, err)
		return
	}
	onlineUsers, err := h.repo.CountOnlineUsersByServerIDs(r.Context(), []int64{id})
	if err != nil {
		writeErr(w, err)
		return
	}
	dto := toServerDTO(s, nodeCounts[id], onlineUsers[id])
	dto.Status = effectiveServerStatus(s, h.offlineAfter(r.Context()), time.Now())
	httpx.WriteJSON(w, http.StatusOK, dto)
}

func (h *Handler) handleServerDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.DeleteServerCascade(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{})
}

const registerTokenTTL = 24 * time.Hour

func (h *Handler) handleServerRegisterToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetServer(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	token, err := adminauth.NewToken()
	if err != nil {
		writeErr(w, err)
		return
	}
	expiresAt := time.Now().Add(registerTokenTTL)
	if err := h.repo.SetServerRegisterTokenHash(r.Context(), id, adminauth.HashToken(token), &expiresAt); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"register_token": token,
		"expires_at":     rfc3339(expiresAt),
	})
}

func (h *Handler) handleServerAgentToken(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	agent, err := h.repo.GetAgentByServerID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			writeErr(w, errNotFound("no agent registered for this server"))
			return
		}
		writeErr(w, err)
		return
	}
	token, err := adminauth.NewToken()
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.RotateAgentTokenHash(r.Context(), agent.ID, adminauth.HashToken(token)); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"agent_token":  token,
		"expires_hint": nil,
	})
}
