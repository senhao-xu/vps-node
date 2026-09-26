package web

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
	"vps-node/internal/subscription"
)

type subscriptionDTO struct {
	Configured bool    `json:"configured"`
	URL        *string `json:"url"`
}

func (h *Handler) handleSubscriptionGet(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	s, err := h.repo.GetUserSubscription(r.Context(), id)
	if err == repo.ErrNotFound {
		httpx.WriteJSON(w, http.StatusOK, subscriptionDTO{})
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	value, err := secrets.Decrypt(h.appKey, s.TokenEnc)
	if err != nil {
		writeErr(w, err)
		return
	}
	link, err := h.subscriptionURL(r, string(value))
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, subscriptionDTO{Configured: true, URL: &link})
}

func (h *Handler) handleSubscriptionCreate(w http.ResponseWriter, r *http.Request) {
	h.saveSubscription(w, r, false)
}
func (h *Handler) handleSubscriptionRotate(w http.ResponseWriter, r *http.Request) {
	h.saveSubscription(w, r, true)
}

func (h *Handler) saveSubscription(w http.ResponseWriter, r *http.Request, rotate bool) {
	id, err := pathID(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if _, err := h.repo.GetUser(r.Context(), id); err != nil {
		writeErr(w, err)
		return
	}
	token, hash, enc, err := h.newSubscriptionCredential()
	if err != nil {
		writeErr(w, err)
		return
	}
	if rotate {
		err = h.repo.RotateUserSubscription(r.Context(), id, hash, enc)
	} else {
		err = h.repo.CreateUserSubscription(r.Context(), id, hash, enc)
	}
	if err == repo.ErrConflict {
		writeErr(w, errConflict("subscription is already configured"))
		return
	}
	if err != nil {
		writeErr(w, err)
		return
	}
	link, err := h.subscriptionURL(r, token)
	if err != nil {
		writeErr(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, subscriptionDTO{Configured: true, URL: &link})
}

func (h *Handler) newSubscriptionCredential() (string, string, []byte, error) {
	token, err := adminauth.NewURLToken(16)
	if err != nil {
		return "", "", nil, err
	}
	enc, err := secrets.Encrypt(h.appKey, []byte(token))
	if err != nil {
		return "", "", nil, err
	}
	return token, adminauth.HashToken(token), enc, nil
}

func (h *Handler) subscriptionURL(r *http.Request, token string) (string, error) {
	settings, err := h.settingsDTO(r.Context())
	if err != nil {
		return "", err
	}
	origins := []string{}
	for _, raw := range strings.Split(settings.SubscribeURLs, ",") {
		if value := strings.TrimSpace(raw); value != "" {
			origins = append(origins, strings.TrimSuffix(value, "/"))
		}
	}
	origin := ""
	if len(origins) > 0 {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(origins))))
		if err != nil {
			return "", err
		}
		origin = origins[n.Int64()]
	} else {
		scheme := "http"
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			scheme = "https"
		}
		origin = scheme + "://" + r.Host
	}
	return origin + "/" + settings.SubscribePath + "/" + url.PathEscape(token), nil
}

func (h *Handler) handlePublicSubscription(w http.ResponseWriter, r *http.Request, token string) {
	u, err := h.repo.ResolveSubscription(r.Context(), adminauth.HashToken(token))
	if err != nil {
		writeErr(w, errNotFound("subscription not found"))
		return
	}
	now := time.Now()
	if u.Status != repo.UserStatusActive || (u.StartedAt != nil && u.StartedAt.After(now)) || (u.ExpiresAt != nil && !u.ExpiresAt.After(now)) || (u.TransferEnable > 0 && u.UsedBytes() >= u.TransferEnable) {
		writeErr(w, errForbidden("subscription is unavailable"))
		return
	}
	repoNodes, err := h.repo.ListSubscriptionNodes(r.Context(), u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	nodes := make([]subscription.Node, 0, len(repoNodes))
	for _, n := range repoNodes {
		settings, secret := map[string]any{}, map[string]any{}
		if err := json.Unmarshal([]byte(n.ProtocolSettings), &settings); err != nil {
			writeErr(w, err)
			return
		}
		if len(n.SecretEnc) > 0 {
			plain, err := secrets.Decrypt(h.appKey, n.SecretEnc)
			if err != nil {
				writeErr(w, err)
				return
			}
			if err := json.Unmarshal(plain, &secret); err != nil {
				writeErr(w, err)
				return
			}
		}
		tlsSettings, _ := settings["tls"].(map[string]any)
		if (n.Protocol == repo.ProtocolHysteria2 || n.Protocol == repo.ProtocolAnyTLS) && (fmt.Sprint(tlsSettings["server_name"]) == "" || secret["certificate"] == nil || secret["private_key"] == nil) {
			continue
		}
		ipv6Address := ""
		if n.IPv6Enabled {
			ipv6Address = n.IPv6Address
		}
		nodes = append(nodes, subscription.Node{ID: n.ID, Name: n.Name, Protocol: n.Protocol, Address: n.Address, IPv6Address: ipv6Address, Port: n.Port, Settings: settings, Secret: secret})
	}
	flag := r.URL.Query().Get("flag")
	if flag == "" {
		ua := strings.ToLower(r.UserAgent())
		if strings.Contains(ua, "clash") || strings.Contains(ua, "mihomo") || strings.Contains(ua, "flclash") || strings.Contains(ua, "nekobox") {
			flag = "clash-meta"
		} else {
			flag = "general"
		}
	}
	if flag != "general" && flag != "clash-meta" {
		writeErr(w, errInvalid("unknown subscription format"))
		return
	}
	skipNode := func(n subscription.Node, err error) {
		h.logger.Warn("skipping unrenderable node in subscription", "user_id", u.ID, "node_id", n.ID, "error", err)
	}
	customSources, err := h.customSourcesForUser(r.Context(), u.ID)
	if err != nil {
		writeErr(w, err)
		return
	}
	skipCustom := func(sourceID int64, item string, err error) {
		h.logger.Warn("skipping unrenderable custom node entry in subscription", "user_id", u.ID, "custom_node_id", sourceID, "error", err)
	}
	w.Header().Set("Subscription-Userinfo", subscriptionUserinfo(u))
	name, err := h.repo.GetSettingOr(r.Context(), settingSubscribeName, "")
	if err != nil {
		writeErr(w, err)
		return
	}
	if name != "" {
		w.Header().Set("Profile-Title", "base64:"+base64.StdEncoding.EncodeToString([]byte(name)))
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(name))
	}
	if flag == "general" {
		body := subscription.RenderGeneralLinksMerged(h.appKey, u.UUID, nodes, customSources, skipNode)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(body))
		return
	}
	template, err := h.repo.GetSettingOr(r.Context(), settingClashTemplate, subscription.DefaultClashMetaTemplate)
	if err != nil {
		writeErr(w, err)
		return
	}
	body, err := subscription.RenderClashFilteredMerged(h.appKey, u.UUID, template, nodes, customSources, skipNode, skipCustom)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(body)
}

func subscriptionUserinfo(u repo.User) string {
	value := fmt.Sprintf("upload=%d; download=%d; total=%d", u.U, u.D, u.TransferEnable)
	if u.ExpiresAt != nil {
		value += fmt.Sprintf("; expire=%d", u.ExpiresAt.Unix())
	}
	return value
}

// customSourcesForUser resolves the active custom nodes authorized for the
// user into renderable sources. links-type content is decrypted and split
// into lines; subscription-type content is fetched upstream with a short TTL
// cache (stale cache is served when the fetch fails, and the source is
// skipped when no cache exists).
func (h *Handler) customSourcesForUser(ctx context.Context, userID int64) ([]subscription.CustomSource, error) {
	customNodes, err := h.repo.ListActiveCustomNodesByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(customNodes) == 0 {
		return nil, nil
	}
	sources := make([]subscription.CustomSource, 0, len(customNodes))
	for _, cn := range customNodes {
		plain, err := secrets.Decrypt(h.appKey, cn.ContentEnc)
		if err != nil {
			h.logger.Warn("skipping custom node with undecryptable content", "custom_node_id", cn.ID, "error", err)
			continue
		}
		source := subscription.CustomSource{ID: cn.ID, Name: cn.Name}
		switch cn.SourceType {
		case repo.CustomNodeSourceLinks:
			source.Links = subscription.SplitLinkLines(string(plain))
		case repo.CustomNodeSourceSubscription:
			content := h.fetchCustomNodeContent(ctx, cn, strings.TrimSpace(string(plain)))
			if content == "" {
				continue
			}
			source.Links, source.Proxies = subscription.NormalizeFetchedContent(content)
		default:
			continue
		}
		if len(source.Links) == 0 && len(source.Proxies) == 0 {
			continue
		}
		sources = append(sources, source)
	}
	return sources, nil
}

// fetchCustomNodeContent returns fresh upstream content when the cache is
// stale, falling back to the cached content when the fetch fails. An empty
// result means the source must be skipped for this render.
func (h *Handler) fetchCustomNodeContent(ctx context.Context, cn repo.CustomNode, upstreamURL string) string {
	if cn.CachedContent != "" && time.Since(cn.FetchedAt) < subscription.CacheTTL {
		return cn.CachedContent
	}
	content, err := subscription.FetchSubscription(ctx, upstreamURL)
	if err != nil {
		h.logger.Warn("custom node subscription fetch failed", "custom_node_id", cn.ID, "error", err)
		return cn.CachedContent
	}
	if err := h.repo.UpdateCustomNodeCache(ctx, cn.ID, content, time.Now().Unix()); err != nil {
		h.logger.Warn("custom node cache write failed", "custom_node_id", cn.ID, "error", err)
	}
	return content
}
