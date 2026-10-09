package web

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
	"vps-node/internal/singbox"
	"vps-node/internal/subscription"
)

type customNodeDTO struct {
	ID                 int64   `json:"id"`
	Name               string  `json:"name"`
	SourceType         string  `json:"source_type"`
	UserAgent          string  `json:"user_agent"`
	InsecureSkipVerify bool    `json:"insecure_skip_verify"`
	Status             string  `json:"status"`
	HasCache           bool    `json:"has_cache"`
	FetchedAt          *string `json:"fetched_at"`
	CreatedAt          string  `json:"created_at"`
	UpdatedAt          string  `json:"updated_at"`
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
		ID:                 n.ID,
		Name:               n.Name,
		SourceType:         n.SourceType,
		UserAgent:          n.UserAgent,
		InsecureSkipVerify: n.InsecureSkipVerify,
		Status:             n.Status,
		HasCache:           n.CachedContent != "",
		FetchedAt:          fetchedAt,
		CreatedAt:          rfc3339(n.CreatedAt),
		UpdatedAt:          rfc3339(n.UpdatedAt),
	}
}

func toCustomNodeDTOs(nodes []repo.CustomNode) []customNodeDTO {
	out := make([]customNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, toCustomNodeDTO(n))
	}
	return out
}

type customNodeEntryDTO struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Server string `json:"server"`
	Port   int    `json:"port"`
	// ChainSupported reports whether this entry's type can be converted into a
	// sing-box chain outbound; the node form greys out the rest.
	ChainSupported bool `json:"chain_supported"`
}

// customNodeEntriesResponse is the parsed digest of a custom node's stored
// content. It never carries secrets or the raw upstream payload.
type customNodeEntriesResponse struct {
	SourceType string               `json:"source_type"`
	HasCache   bool                 `json:"has_cache"`
	FetchedAt  *string              `json:"fetched_at"`
	Entries    []customNodeEntryDTO `json:"entries"`
	Skipped    []string             `json:"skipped,omitempty"`
}

func toCustomNodeEntryDTOs(summaries []subscription.EntrySummary) []customNodeEntryDTO {
	out := make([]customNodeEntryDTO, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, customNodeEntryDTO{
			Key: s.Key, Name: s.Name, Type: s.Type, Server: s.Server, Port: s.Port,
			ChainSupported: singbox.OutboundSupported(s.Type),
		})
	}
	return out
}

// customNodeSourceContent decodes the content already stored for a custom node
// source without any network IO: links live in the encrypted column and
// subscription sources are read from the fetch cache only (an empty cache
// yields no content).
func (h *Handler) customNodeSourceContent(cn repo.CustomNode) (links []string, proxies []map[string]any, err error) {
	switch cn.SourceType {
	case repo.CustomNodeSourceLinks:
		plain, err := secrets.Decrypt(h.appKey, cn.ContentEnc)
		if err != nil {
			return nil, nil, err
		}
		return subscription.SplitLinkLines(string(plain)), nil, nil
	case repo.CustomNodeSourceSubscription:
		if cn.CachedContent == "" {
			return nil, nil, nil
		}
		links, proxies = subscription.NormalizeFetchedContent(cn.CachedContent)
		return links, proxies, nil
	default:
		return nil, nil, nil
	}
}

// resolveCustomNodeEntries parses a source's stored content into entries with
// their stable keys. It is the shared parse pass behind the entries digest and
// the external chain outbound.
func (h *Handler) resolveCustomNodeEntries(cn repo.CustomNode) ([]subscription.ResolvedEntry, error) {
	links, proxies, err := h.customNodeSourceContent(cn)
	if err != nil {
		return nil, err
	}
	return subscription.ResolveEntries(h.appKey, links, proxies)
}

// customNodeEntries parses a custom node's own stored content into display
// entries. It performs no network IO: subscription sources are read from the
// fetch cache only, and a cache miss yields an empty list.
func (h *Handler) customNodeEntries(cn repo.CustomNode) (customNodeEntriesResponse, error) {
	hasCache := cn.SourceType == repo.CustomNodeSourceSubscription && cn.CachedContent != ""
	resp := customNodeEntriesResponse{
		SourceType: cn.SourceType,
		HasCache:   hasCache,
		Entries:    []customNodeEntryDTO{},
	}
	if hasCache && !cn.FetchedAt.IsZero() {
		resp.FetchedAt = rfc3339Ptr(&cn.FetchedAt)
	}
	links, proxies, err := h.customNodeSourceContent(cn)
	if err != nil {
		return customNodeEntriesResponse{}, err
	}
	entries, skipped := subscription.SummarizeEntries(h.appKey, links, proxies)
	resp.Entries = toCustomNodeEntryDTOs(entries)
	resp.Skipped = skipped
	return resp, nil
}

func (h *Handler) handleCustomNodeNodesGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	cn, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	resp, err := h.customNodeEntries(cn)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

// customNodeShareDTO is the admin share/export payload for one custom node: a
// Clash `proxies:` fragment plus the plaintext link lines and the items that
// could not be converted.
type customNodeShareDTO struct {
	SourceType string   `json:"source_type"`
	HasCache   bool     `json:"has_cache"`
	FetchedAt  *string  `json:"fetched_at"`
	Clash      string   `json:"clash"`
	Links      []string `json:"links"`
	Skipped    []string `json:"skipped,omitempty"`
}

// handleCustomNodeShare renders one custom node's content for admin copy
// without a user. links sources decrypt content_enc; subscription sources use
// the render-path lazy fetch (TTL cache with stale fallback).
func (h *Handler) handleCustomNodeShare(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	cn, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	links := []string{}
	var proxies []map[string]any
	switch cn.SourceType {
	case repo.CustomNodeSourceLinks:
		plain, err := secrets.Decrypt(h.appKey, cn.ContentEnc)
		if err != nil {
			writeErr(w, errInternal("failed to decrypt custom node content"))
			return
		}
		links = subscription.SplitLinkLines(string(plain))
	case repo.CustomNodeSourceSubscription:
		upstream, err := secrets.Decrypt(h.appKey, cn.ContentEnc)
		if err != nil {
			writeErr(w, errInternal("failed to decrypt custom node content"))
			return
		}
		content := h.fetchCustomNodeContent(r.Context(), cn, strings.TrimSpace(string(upstream)))
		if content != "" {
			links, proxies = subscription.NormalizeFetchedContent(content)
		}
		if links == nil {
			links = []string{}
		}
		if updated, err := h.repo.GetCustomNode(r.Context(), cn.ID); err == nil {
			cn = updated
		}
	}
	skipped := []string{}
	source := subscription.CustomSource{ID: cn.ID, Name: cn.Name, Links: links, Proxies: proxies}
	rendered := subscription.RenderCustomProxies(source, func(_ int64, item string, _ error) {
		skipped = append(skipped, item)
	})
	clash, err := subscription.RenderProxiesFragment(rendered)
	if err != nil {
		writeErr(w, errInternal("failed to render clash proxies: "+err.Error()))
		return
	}
	resp := customNodeShareDTO{
		SourceType: cn.SourceType,
		HasCache:   cn.SourceType == repo.CustomNodeSourceSubscription && cn.CachedContent != "",
		Clash:      string(clash),
		Links:      links,
		Skipped:    skipped,
	}
	if resp.HasCache && !cn.FetchedAt.IsZero() {
		resp.FetchedAt = rfc3339Ptr(&cn.FetchedAt)
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
}

type customNodeContentDTO struct {
	Content string `json:"content"`
}

// handleCustomNodeContentGet echoes the decrypted stored content of one custom
// node so the edit form can prefill it. The list endpoint never returns it.
func (h *Handler) handleCustomNodeContentGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	cn, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	plain, err := secrets.Decrypt(h.appKey, cn.ContentEnc)
	if err != nil {
		writeErr(w, errInternal("failed to decrypt custom node content"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, customNodeContentDTO{Content: string(plain)})
}

// handleCustomNodeRefresh force-fetches a subscription source, ignoring the
// render-path cache TTL. On upstream failure the previous cache and
// fetched_at are left untouched so later renders keep serving stale data.
func (h *Handler) handleCustomNodeRefresh(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	cn, err := h.repo.GetCustomNode(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if cn.SourceType != repo.CustomNodeSourceSubscription {
		writeErr(w, errValidation("refresh is only supported for subscription sources"))
		return
	}
	upstream, err := secrets.Decrypt(h.appKey, cn.ContentEnc)
	if err != nil {
		writeErr(w, err)
		return
	}
	content, err := subscription.FetchSubscription(r.Context(), strings.TrimSpace(string(upstream)), cn.UserAgent, cn.InsecureSkipVerify)
	if err != nil {
		writeErr(w, errInternal("failed to fetch upstream subscription: "+err.Error()))
		return
	}
	if err := h.repo.UpdateCustomNodeCache(r.Context(), cn.ID, content, time.Now().Unix()); err != nil {
		writeErr(w, err)
		return
	}
	if err := h.bumpServersChainingCustomNode(r.Context(), cn.ID); err != nil {
		writeErr(w, err)
		return
	}
	updated, err := h.repo.GetCustomNode(r.Context(), cn.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	resp, err := h.customNodeEntries(updated)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, resp)
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
	Name               string  `json:"name"`
	Content            *string `json:"content"`
	Status             string  `json:"status"`
	UserAgent          *string `json:"user_agent"`
	InsecureSkipVerify *bool   `json:"insecure_skip_verify"`
}

type createCustomNodeRequest struct {
	Name               string `json:"name"`
	SourceType         string `json:"source_type"`
	Content            string `json:"content"`
	UserAgent          string `json:"user_agent"`
	InsecureSkipVerify bool   `json:"insecure_skip_verify"`
}

func validateCustomNodeName(name string) error {
	if name == "" || len(name) > 128 {
		return errValidation("name must be 1-128 characters")
	}
	return nil
}

// validateUserAgent rejects values that could inject extra HTTP headers and
// caps the length. An empty value is valid and means "use the default".
func validateUserAgent(ua string) error {
	if len(ua) > 255 {
		return errValidation("user_agent must be at most 255 characters")
	}
	for _, r := range ua {
		if r < 0x20 || r == 0x7f {
			return errValidation("user_agent must not contain control characters")
		}
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
	userAgent := ""
	insecureSkipVerify := false
	if req.SourceType == repo.CustomNodeSourceSubscription {
		userAgent = strings.TrimSpace(req.UserAgent)
		if err := validateUserAgent(userAgent); err != nil {
			writeErr(w, err)
			return
		}
		insecureSkipVerify = req.InsecureSkipVerify
	}
	id, err := h.repo.CreateCustomNode(r.Context(), repo.NewCustomNode{
		Name:               req.Name,
		SourceType:         req.SourceType,
		UserAgent:          userAgent,
		InsecureSkipVerify: insecureSkipVerify,
		ContentEnc:         enc,
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
	// links sources never carry a User-Agent, so their stored value stays empty.
	userAgent := existing.UserAgent
	if existing.SourceType == repo.CustomNodeSourceSubscription && req.UserAgent != nil {
		userAgent = strings.TrimSpace(*req.UserAgent)
		if err := validateUserAgent(userAgent); err != nil {
			writeErr(w, err)
			return
		}
	}
	insecureSkipVerify := existing.InsecureSkipVerify
	if existing.SourceType == repo.CustomNodeSourceSubscription {
		if req.InsecureSkipVerify != nil {
			insecureSkipVerify = *req.InsecureSkipVerify
		}
	} else {
		insecureSkipVerify = false
	}
	invalidateCache := contentEnc != nil || userAgent != existing.UserAgent || insecureSkipVerify != existing.InsecureSkipVerify
	if err := h.repo.UpdateCustomNode(r.Context(), id, name, contentEnc, status, userAgent, insecureSkipVerify, invalidateCache); err != nil {
		if err == repo.ErrConflict {
			writeErr(w, errConflict("a custom node with this name already exists"))
			return
		}
		writeErr(w, err)
		return
	}
	if err := h.bumpServersChainingCustomNode(r.Context(), id); err != nil {
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
	referencing, err := h.repo.ListNodesByCustomChainTarget(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	if len(referencing) > 0 {
		names := make([]string, 0, len(referencing))
		for _, ref := range referencing {
			names = append(names, ref.Name)
		}
		writeErr(w, errConflict(fmt.Sprintf("custom node is the chain exit of %s; unlink those nodes first", strings.Join(names, ", "))))
		return
	}
	if err := h.repo.DeleteCustomNode(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// bumpServersChainingCustomNode refreshes every server whose entry nodes use
// this source as their external chain exit, so their agent configs re-render
// after the source content, status, or cache changes.
func (h *Handler) bumpServersChainingCustomNode(ctx context.Context, customNodeID int64) error {
	serverIDs, err := h.repo.ListServersChainingCustomNode(ctx, customNodeID)
	if err != nil {
		return err
	}
	for _, serverID := range serverIDs {
		if _, err := h.repo.BumpServerRevision(ctx, serverID); err != nil {
			return err
		}
	}
	return nil
}

// customNodeEntrySelectionDTO is one source's entry-key whitelist. It is
// returned (and accepted) only for sources whose whitelist is non-empty; a
// source with no entry is authorized for every entry it contains.
type customNodeEntrySelectionDTO struct {
	CustomNodeID int64    `json:"custom_node_id"`
	EntryKeys    []string `json:"entry_keys"`
}

type userCustomNodesResponse struct {
	CustomNodeIDs     []int64                       `json:"custom_node_ids"`
	CustomNodes       []customNodeDTO               `json:"custom_nodes"`
	CustomNodeEntries []customNodeEntrySelectionDTO `json:"custom_node_entries"`
}

// toEntrySelections renders the per-source whitelists in a stable order,
// omitting sources with no whitelist.
func toEntrySelections(entries map[int64]map[string]struct{}) []customNodeEntrySelectionDTO {
	ids := make([]int64, 0, len(entries))
	for id, keys := range entries {
		if len(keys) > 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	out := make([]customNodeEntrySelectionDTO, 0, len(ids))
	for _, id := range ids {
		keys := make([]string, 0, len(entries[id]))
		for key := range entries[id] {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out = append(out, customNodeEntrySelectionDTO{CustomNodeID: id, EntryKeys: keys})
	}
	return out
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
	entryKeys, err := h.repo.ListCustomNodeEntryKeysByUser(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userCustomNodesResponse{
		CustomNodeIDs:     customNodeIDs,
		CustomNodes:       toCustomNodeDTOs(nodes),
		CustomNodeEntries: toEntrySelections(entryKeys),
	})
}

func (h *Handler) handleUserCustomNodesPut(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req struct {
		CustomNodeIDs     []int64                       `json:"custom_node_ids"`
		CustomNodeEntries []customNodeEntrySelectionDTO `json:"custom_node_entries"`
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
	entries, err := validateCustomNodeEntries(req.CustomNodeEntries, customNodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	// Custom nodes never reach the agent config, so no server revision bump.
	if err := h.repo.SetUserCustomNodesAndEntries(r.Context(), id, customNodeIDs, entries); err != nil {
		writeErr(w, err)
		return
	}
	nodes, err := h.repo.ListCustomNodesByIDs(r.Context(), customNodeIDs)
	if err != nil {
		writeErr(w, err)
		return
	}
	entryKeys, err := h.repo.ListCustomNodeEntryKeysByUser(r.Context(), id)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, userCustomNodesResponse{
		CustomNodeIDs:     customNodeIDs,
		CustomNodes:       toCustomNodeDTOs(nodes),
		CustomNodeEntries: toEntrySelections(entryKeys),
	})
}

// validateCustomNodeEntries normalizes the optional per-source entry
// whitelists: every referenced source must itself be authorized, every key must
// be a 64-character lowercase hex digest, and duplicates/blanks are dropped.
// An empty resulting whitelist is omitted, which means "all entries".
func validateCustomNodeEntries(selections []customNodeEntrySelectionDTO, customNodeIDs []int64) (map[int64][]string, error) {
	authorized := make(map[int64]bool, len(customNodeIDs))
	for _, id := range customNodeIDs {
		authorized[id] = true
	}
	out := map[int64][]string{}
	seen := map[int64]map[string]bool{}
	for _, selection := range selections {
		if !authorized[selection.CustomNodeID] {
			return nil, errValidation("custom_node_entries custom_node_id must also appear in custom_node_ids")
		}
		if seen[selection.CustomNodeID] == nil {
			seen[selection.CustomNodeID] = map[string]bool{}
		}
		for _, key := range selection.EntryKeys {
			if key == "" {
				continue
			}
			if !isEntryKey(key) {
				return nil, errValidation("entry_key must be a 64-character lowercase hexadecimal string")
			}
			if seen[selection.CustomNodeID][key] {
				continue
			}
			seen[selection.CustomNodeID][key] = true
			// Repeated selections for one source merge instead of replacing,
			// so duplicates can never reach the composite primary key.
			out[selection.CustomNodeID] = append(out[selection.CustomNodeID], key)
		}
	}
	return out, nil
}

func isEntryKey(key string) bool {
	if len(key) != 64 {
		return false
	}
	for _, r := range key {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
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
