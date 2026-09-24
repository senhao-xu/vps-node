package web

import (
	"net/http"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
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
		Name string `json:"name"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateServerName(req.Name); err != nil {
		writeErr(w, err)
		return
	}
	id, err := h.repo.CreateServer(r.Context(), req.Name, repo.ServerStatusActive)
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

func validateServerName(name string) error {
	if name == "" || len(name) > 128 {
		return errValidation("name must be 1-128 characters")
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
		Name   *string `json:"name"`
		Status *string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}

	name, status := current.Name, current.Status
	if req.Name != nil {
		name = *req.Name
	}
	if req.Status != nil {
		if !validServerStatuses[*req.Status] {
			writeErr(w, errInvalid("invalid status, want active or disabled"))
			return
		}
		status = *req.Status
	}
	if err := validateServerName(name); err != nil {
		writeErr(w, err)
		return
	}

	s, err := h.repo.UpdateServerAndBump(r.Context(), id, name, status)
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

func (h *Handler) handleServerAgentKeyGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetServer(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	agent, err := h.repo.GetAgentByServerID(r.Context(), id)
	if err != nil {
		if err == repo.ErrNotFound {
			writeErr(w, errConflict("no agent key for this server; generate one first"))
			return
		}
		writeErr(w, err)
		return
	}
	if len(agent.KeyEnc) == 0 {
		writeErr(w, errConflict("agent key was not generated on this panel version; reset it first"))
		return
	}
	plain, err := secrets.Decrypt(h.appKey, agent.KeyEnc)
	if err != nil {
		writeErr(w, errConflict("agent key cannot be decrypted; reset it first"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"agent_key": string(plain)})
}

func (h *Handler) handleServerAgentKeyGenerate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetServer(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	key, err := adminauth.NewToken()
	if err != nil {
		writeErr(w, err)
		return
	}
	keyEnc, err := secrets.Encrypt(h.appKey, []byte(key))
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := h.repo.UpsertAgentKey(r.Context(), id, adminauth.HashToken(key), keyEnc); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"agent_key": key})
}
