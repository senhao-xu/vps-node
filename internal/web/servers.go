package web

import (
	"net"
	"net/http"
	"regexp"
	"strings"
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
		dto.MonthlyUploadBytes, dto.MonthlyDownloadBytes, dto.MonthlyUsedBytes, err = h.repo.ServerMonthlyUsage(r.Context(), s, now)
		if err != nil {
			writeErr(w, err)
			return
		}
		dto.Status = effectiveServerStatus(s, offlineAfter, now)
		items = append(items, dto)
	}
	writePage(w, items, total, page)
}

func (h *Handler) handleServerCreate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name              string  `json:"name"`
		Notes             string  `json:"notes"`
		PublicVisible     *bool   `json:"public_visible"`
		OfflineNotify     bool    `json:"offline_notify"`
		IPv6              string  `json:"ipv6"`
		TrafficAccounting string  `json:"traffic_accounting"`
		TrafficResetDay   int     `json:"traffic_reset_day"`
		BillingCycle      string  `json:"billing_cycle"`
		IP                string  `json:"ip"`
		Region            string  `json:"region"`
		PriceCents        int64   `json:"price_cents"`
		PriceCurrency     string  `json:"price_currency"`
		TrafficLimitBytes int64   `json:"traffic_limit_bytes"`
		ExpiresAt         *string `json:"expires_at"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	if err := validateServerName(req.Name); err != nil {
		writeErr(w, err)
		return
	}
	inventory, err := validateServerInventory(req.IP, req.Region, req.PriceCents, req.PriceCurrency, req.TrafficLimitBytes, req.ExpiresAt)
	if err != nil {
		writeErr(w, err)
		return
	}
	inventory.Notes = req.Notes
	inventory.PublicVisible = req.PublicVisible == nil || *req.PublicVisible
	inventory.OfflineNotify = req.OfflineNotify
	inventory.IPv6 = req.IPv6
	inventory.TrafficAccounting = req.TrafficAccounting
	inventory.TrafficResetDay = req.TrafficResetDay
	inventory.BillingCycle = req.BillingCycle
	if err := validateServerDetails(&inventory); err != nil {
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
	id, err := h.repo.CreateServerWithAgentKey(r.Context(), req.Name, repo.ServerStatusActive, adminauth.HashToken(key), keyEnc, inventory)
	if err != nil {
		writeErr(w, err)
		return
	}
	s, err := h.repo.GetServer(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, serverCreateDTO{serverDTO: toServerDTO(s, 0, 0), AgentKey: key})
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
	dto.MonthlyUploadBytes, dto.MonthlyDownloadBytes, dto.MonthlyUsedBytes, err = h.repo.ServerMonthlyUsage(r.Context(), s, time.Now())
	if err != nil {
		writeErr(w, err)
		return
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
		Name              *string `json:"name"`
		Notes             *string `json:"notes"`
		PublicVisible     *bool   `json:"public_visible"`
		OfflineNotify     *bool   `json:"offline_notify"`
		IPv6              *string `json:"ipv6"`
		TrafficAccounting *string `json:"traffic_accounting"`
		TrafficResetDay   *int    `json:"traffic_reset_day"`
		BillingCycle      *string `json:"billing_cycle"`
		Status            *string `json:"status"`
		IP                *string `json:"ip"`
		Region            *string `json:"region"`
		PriceCents        *int64  `json:"price_cents"`
		PriceCurrency     *string `json:"price_currency"`
		TrafficLimitBytes *int64  `json:"traffic_limit_bytes"`
		ExpiresAt         *string `json:"expires_at"`
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

	ip, region, cents, currency, limit := current.IP, current.Region, current.PriceCents, current.PriceCurrency, current.TrafficLimitBytes
	if req.IP != nil {
		ip = *req.IP
	}
	if req.Region != nil {
		region = *req.Region
	}
	if req.PriceCents != nil {
		cents = *req.PriceCents
	}
	if req.PriceCurrency != nil {
		currency = *req.PriceCurrency
	}
	if req.TrafficLimitBytes != nil {
		limit = *req.TrafficLimitBytes
	}
	var expiry *string
	if current.ExpiresAt != nil {
		value := current.ExpiresAt.Format(time.RFC3339)
		expiry = &value
	}
	if req.ExpiresAt != nil {
		expiry = req.ExpiresAt
	}
	inventory, err := validateServerInventory(ip, region, cents, currency, limit, expiry)
	if err != nil {
		writeErr(w, err)
		return
	}
	inventory.Notes = current.Notes
	inventory.PublicVisible = current.PublicVisible
	inventory.OfflineNotify = current.OfflineNotify
	inventory.IPv6 = current.IPv6
	inventory.TrafficAccounting = current.TrafficAccounting
	inventory.TrafficResetDay = current.TrafficResetDay
	inventory.BillingCycle = current.BillingCycle
	if req.Notes != nil {
		inventory.Notes = *req.Notes
	}
	if req.PublicVisible != nil {
		inventory.PublicVisible = *req.PublicVisible
	}
	if req.OfflineNotify != nil {
		inventory.OfflineNotify = *req.OfflineNotify
	}
	if req.IPv6 != nil {
		inventory.IPv6 = *req.IPv6
	}
	if req.TrafficAccounting != nil {
		inventory.TrafficAccounting = *req.TrafficAccounting
	}
	if req.TrafficResetDay != nil {
		inventory.TrafficResetDay = *req.TrafficResetDay
	}
	if req.BillingCycle != nil {
		inventory.BillingCycle = *req.BillingCycle
	}
	if err := validateServerDetails(&inventory); err != nil {
		writeErr(w, err)
		return
	}
	s, err := h.repo.UpdateServerAndBump(r.Context(), id, name, status, inventory)
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
	dto.MonthlyUploadBytes, dto.MonthlyDownloadBytes, dto.MonthlyUsedBytes, err = h.repo.ServerMonthlyUsage(r.Context(), s, time.Now())
	if err != nil {
		writeErr(w, err)
		return
	}
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

var currencyCode = regexp.MustCompile(`^[A-Z]{3}$`)

func validateServerInventory(ip, region string, cents int64, currency string, limit int64, expiry *string) (repo.ServerInventory, error) {
	v := repo.ServerInventory{IP: strings.TrimSpace(ip), Region: strings.ToUpper(strings.TrimSpace(region)), PriceCents: cents, PriceCurrency: strings.ToUpper(strings.TrimSpace(currency)), TrafficLimitBytes: limit}
	if v.PriceCurrency == "" {
		v.PriceCurrency = "USD"
	}
	if v.IP != "" && net.ParseIP(v.IP) == nil {
		return v, errValidation("ip must be a valid IP address")
	}
	if len(v.Region) > 8 || cents < 0 || limit < 0 || !currencyCode.MatchString(v.PriceCurrency) {
		return v, errValidation("invalid server inventory")
	}
	if expiry != nil && *expiry != "" {
		parsed, err := time.Parse(time.RFC3339, *expiry)
		if err != nil {
			return v, errValidation("expires_at must be RFC3339")
		}
		v.ExpiresAt = &parsed
	}
	return v, nil
}

func (h *Handler) handleServerReorder(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	servers, err := h.repo.ListServers(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(req.IDs) != len(servers) {
		writeErr(w, errValidation("ids must contain every server"))
		return
	}
	known := make(map[int64]bool, len(servers))
	for _, server := range servers {
		known[server.ID] = true
	}
	for _, id := range req.IDs {
		if !known[id] {
			writeErr(w, errValidation("ids must be unique existing servers"))
			return
		}
		delete(known, id)
	}
	if err := h.repo.ReorderServers(r.Context(), req.IDs); err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{})
}

func validateServerDetails(v *repo.ServerInventory) error {
	v.Notes = strings.TrimSpace(v.Notes)
	v.IPv6 = strings.TrimSpace(v.IPv6)
	if v.TrafficAccounting == "" {
		v.TrafficAccounting = "max"
	}
	if v.TrafficResetDay == 0 {
		v.TrafficResetDay = 1
	}
	if v.BillingCycle == "" {
		v.BillingCycle = "monthly"
	}
	if len(v.Notes) > 2000 {
		return errValidation("notes must be at most 2000 characters")
	}
	if v.IPv6 != "" {
		parsed := net.ParseIP(v.IPv6)
		if parsed == nil || parsed.To4() != nil {
			return errValidation("ipv6 must be a valid IPv6 address")
		}
	}
	if v.TrafficAccounting != "sum" && v.TrafficAccounting != "max" {
		return errValidation("invalid traffic_accounting")
	}
	if v.TrafficResetDay < 1 || v.TrafficResetDay > 31 {
		return errValidation("traffic_reset_day must be 1-31")
	}
	switch v.BillingCycle {
	case "monthly", "quarterly", "semiannual", "yearly", "one_time":
	default:
		return errValidation("invalid billing_cycle")
	}
	return nil
}
