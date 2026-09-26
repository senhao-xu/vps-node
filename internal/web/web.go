package web

import (
	"context"
	"database/sql"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/httpx"
	"vps-node/internal/repo"
	"vps-node/internal/subscription"
)

const (
	settingRetentionAggregate      = "retention.aggregate_days"
	settingServerOfflineAfter      = "server.offline_after_seconds"
	settingCollectionVisits        = "collection.visits"
	settingRetentionVisit          = "retention.visit_days"
	settingRetentionVisitAggregate = "retention.visit_aggregate_days"
	settingSubscribeURLs           = "subscribe_urls"
	settingSubscribePath           = "subscribe_path"
	settingSubscribeName           = "subscribe_name"
	settingClashTemplate           = "clash_meta_template"
)

var weakPasswords = map[string]bool{
	"admin": true, "password": true, "123456": true, "12345678": true,
	"123456789": true, "root": true, "changeme": true, "letmein": true, "panel": true,
}

type Options struct {
	DB            *sql.DB
	Repo          *repo.Repo
	Sessions      *adminauth.Sessions
	Limiter       *adminauth.Limiter
	AppKey        []byte
	Logger        *slog.Logger
	AdminUsername string
	AdminPassword string

	AgentHeartbeatInterval time.Duration
	AgentSyncInterval      time.Duration
	AgentTrafficInterval   time.Duration
}

type Handler struct {
	db            *sql.DB
	repo          *repo.Repo
	sessions      *adminauth.Sessions
	limiter       *adminauth.Limiter
	appKey        []byte
	logger        *slog.Logger
	adminUsername string
	adminPassword string

	agentHeartbeatIntervalSeconds int
	agentSyncIntervalSeconds      int
	agentTrafficIntervalSeconds   int
}

type ctxKey int

const ctxKeyAdminID ctxKey = 1

func New(o Options) (http.Handler, error) {
	logger := o.Logger
	if logger == nil {
		logger = slog.Default()
	}
	h := &Handler{
		db:            o.DB,
		repo:          o.Repo,
		sessions:      o.Sessions,
		limiter:       o.Limiter,
		appKey:        o.AppKey,
		logger:        logger,
		adminUsername: o.AdminUsername,
		adminPassword: o.AdminPassword,

		agentHeartbeatIntervalSeconds: intervalSeconds(o.AgentHeartbeatInterval, defaultHeartbeatIntervalSeconds),
		agentSyncIntervalSeconds:      intervalSeconds(o.AgentSyncInterval, defaultSyncIntervalSeconds),
		agentTrafficIntervalSeconds:   intervalSeconds(o.AgentTrafficInterval, defaultTrafficIntervalSeconds),
	}
	if err := h.ensureAdmin(context.Background()); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", httpx.Healthz())
	mux.HandleFunc("GET /", h.handlePublicSubscriptionDispatcher)
	h.registerAdminRoutes(mux)
	h.registerAgentRoutes(mux)
	return mux, nil
}

func (h *Handler) handlePublicSubscriptionDispatcher(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	settings, err := h.settingsDTO(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != settings.SubscribePath || parts[1] == "" {
		http.NotFound(w, r)
		return
	}
	h.handlePublicSubscription(w, r, parts[1])
}

func intervalSeconds(d time.Duration, fallback int) int {
	if d <= 0 {
		return fallback
	}
	return int(d / time.Second)
}

func (h *Handler) registerAgentRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/agent/heartbeat", h.requireAgent(h.handleAgentHeartbeat))
	mux.HandleFunc("GET /api/agent/config", h.requireAgent(h.handleAgentConfig))
	mux.HandleFunc("POST /api/agent/traffic", h.requireAgent(h.handleAgentTraffic))
	mux.HandleFunc("POST /api/agent/devices", h.requireAgent(h.handleAgentDevices))
	mux.HandleFunc("POST /api/agent/visits", h.requireAgent(h.handleAgentVisits))
}

func (h *Handler) registerAdminRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/admin/login", h.handleLogin)
	mux.HandleFunc("POST /api/admin/logout", h.requireAdmin(h.handleLogout))
	mux.HandleFunc("GET /api/admin/me", h.requireAdmin(h.handleMe))

	mux.HandleFunc("GET /api/users", h.requireAdmin(h.handleUserList))
	mux.HandleFunc("POST /api/users", h.requireAdmin(h.handleUserCreate))
	mux.HandleFunc("GET /api/users/{id}", h.requireAdmin(h.handleUserGet))
	mux.HandleFunc("PUT /api/users/{id}", h.requireAdmin(h.handleUserUpdate))
	mux.HandleFunc("DELETE /api/users/{id}", h.requireAdmin(h.handleUserDelete))
	mux.HandleFunc("POST /api/users/{id}/reset-token", h.requireAdmin(h.handleUserResetToken))
	mux.HandleFunc("POST /api/users/{id}/reset-traffic", h.requireAdmin(h.handleUserResetTraffic))
	mux.HandleFunc("POST /api/users/{id}/expire-now", h.requireAdmin(h.handleUserExpireNow))
	mux.HandleFunc("GET /api/users/{id}/nodes", h.requireAdmin(h.handleUserNodesGet))
	mux.HandleFunc("PUT /api/users/{id}/nodes", h.requireAdmin(h.handleUserNodesPut))
	mux.HandleFunc("GET /api/users/{id}/devices", h.requireAdmin(h.handleUserDevices))
	mux.HandleFunc("GET /api/users/{id}/traffic", h.requireAdmin(h.handleUserTraffic))
	mux.HandleFunc("GET /api/users/{id}/visits", h.requireAdmin(h.handleUserVisits))
	mux.HandleFunc("GET /api/users/{id}/subscription", h.requireAdmin(h.handleSubscriptionGet))
	mux.HandleFunc("POST /api/users/{id}/subscription", h.requireAdmin(h.handleSubscriptionCreate))
	mux.HandleFunc("POST /api/users/{id}/subscription/rotate", h.requireAdmin(h.handleSubscriptionRotate))

	mux.HandleFunc("GET /api/servers", h.requireAdmin(h.handleServerList))
	mux.HandleFunc("POST /api/servers", h.requireAdmin(h.handleServerCreate))
	mux.HandleFunc("GET /api/servers/{id}", h.requireAdmin(h.handleServerGet))
	mux.HandleFunc("PUT /api/servers/{id}", h.requireAdmin(h.handleServerUpdate))
	mux.HandleFunc("DELETE /api/servers/{id}", h.requireAdmin(h.handleServerDelete))
	mux.HandleFunc("GET /api/servers/{id}/agent-key", h.requireAdmin(h.handleServerAgentKeyGet))
	mux.HandleFunc("POST /api/servers/{id}/agent-key", h.requireAdmin(h.handleServerAgentKeyGenerate))
	mux.HandleFunc("GET /api/servers/{id}/visits", h.requireAdmin(h.handleServerVisits))

	mux.HandleFunc("GET /api/visits", h.requireAdmin(h.handleVisitList))
	mux.HandleFunc("GET /api/visits/top", h.requireAdmin(h.handleVisitTop))

	mux.HandleFunc("GET /api/nodes", h.requireAdmin(h.handleNodeList))
	mux.HandleFunc("POST /api/nodes", h.requireAdmin(h.handleNodeCreate))
	mux.HandleFunc("POST /api/nodes/reality-keypair", h.requireAdmin(h.handleRealityKeypairGenerate))
	mux.HandleFunc("GET /api/nodes/{id}", h.requireAdmin(h.handleNodeGet))
	mux.HandleFunc("POST /api/nodes/{id}/copy", h.requireAdmin(h.handleNodeCopy))
	mux.HandleFunc("PUT /api/nodes/{id}", h.requireAdmin(h.handleNodeUpdate))
	mux.HandleFunc("DELETE /api/nodes/{id}", h.requireAdmin(h.handleNodeDelete))

	mux.HandleFunc("GET /api/custom-nodes", h.requireAdmin(h.handleCustomNodeList))
	mux.HandleFunc("POST /api/custom-nodes", h.requireAdmin(h.handleCustomNodeCreate))
	mux.HandleFunc("PUT /api/custom-nodes/{id}", h.requireAdmin(h.handleCustomNodeUpdate))
	mux.HandleFunc("DELETE /api/custom-nodes/{id}", h.requireAdmin(h.handleCustomNodeDelete))
	mux.HandleFunc("GET /api/users/{id}/custom-nodes", h.requireAdmin(h.handleUserCustomNodesGet))
	mux.HandleFunc("PUT /api/users/{id}/custom-nodes", h.requireAdmin(h.handleUserCustomNodesPut))

	mux.HandleFunc("GET /api/dashboard", h.requireAdmin(h.handleDashboard))
	mux.HandleFunc("GET /api/dashboard/user-traffic", h.requireAdmin(h.handleDashboardUserTraffic))
	mux.HandleFunc("GET /api/settings", h.requireAdmin(h.handleSettingsGet))
	mux.HandleFunc("PUT /api/settings", h.requireAdmin(h.handleSettingsPut))
}

func (h *Handler) ensureAdmin(ctx context.Context) error {
	count, err := h.repo.CountAdmins(ctx)
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	username := h.adminUsername
	if strings.TrimSpace(username) == "" {
		username = "admin"
	}
	if strings.TrimSpace(h.adminPassword) == "" {
		h.logger.Warn("admin bootstrap skipped: set admin.password in config or PANEL_ADMIN_PASSWORD to create the initial admin account")
		return nil
	}
	if len(h.adminPassword) < 8 || weakPasswords[strings.ToLower(h.adminPassword)] {
		h.logger.Warn("initial admin password is weak or a well-known default; change it immediately")
	}
	hash, err := adminauth.HashPassword(h.adminPassword)
	if err != nil {
		return err
	}
	if _, err := h.repo.CreateAdmin(ctx, username, hash); err != nil {
		return err
	}
	h.logger.Info("created initial admin account", "username", username)
	return nil
}

func (h *Handler) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(adminauth.CookieName)
		if err != nil || cookie.Value == "" {
			writeErr(w, errUnauthorized("admin session required"))
			return
		}
		adminID, err := h.sessions.Resolve(r.Context(), cookie.Value)
		if err != nil {
			writeErr(w, errUnauthorized("admin session required"))
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyAdminID, adminID)
		next(w, r.WithContext(ctx))
	}
}

func adminIDFrom(ctx context.Context) int64 {
	id, _ := ctx.Value(ctxKeyAdminID).(int64)
	return id
}

func (h *Handler) settingsDTO(ctx context.Context) (settingsDTO, error) {
	raw, err := h.repo.ListSettings(ctx)
	if err != nil {
		return settingsDTO{}, err
	}
	return settingsFromMap(raw), nil
}

func settingsFromMap(raw map[string]string) settingsDTO {
	return settingsDTO{
		RetentionAggregateDays:      settingInt(raw, settingRetentionAggregate, 90, 1),
		RetentionVisitDays:          settingInt(raw, settingRetentionVisit, 7, 1),
		RetentionVisitAggregateDays: settingInt(raw, settingRetentionVisitAggregate, 90, 1),
		CollectionVisits:            settingBool(raw, settingCollectionVisits, true),
		ServerOfflineAfterSeconds:   settingInt(raw, settingServerOfflineAfter, 60, 1),
		SubscribeURLs:               raw[settingSubscribeURLs],
		SubscribePath:               settingString(raw, settingSubscribePath, "s"),
		SubscribeName:               raw[settingSubscribeName],
		ClashMetaTemplate:           settingString(raw, settingClashTemplate, subscription.DefaultClashMetaTemplate),
	}
}

func settingString(raw map[string]string, key, fallback string) string {
	if value := raw[key]; value != "" {
		return value
	}
	return fallback
}

func settingInt(raw map[string]string, key string, fallback, min int) int {
	v, ok := raw[key]
	if !ok {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return fallback
	}
	return n
}

func settingBool(raw map[string]string, key string, fallback bool) bool {
	v, ok := raw[key]
	if !ok || v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func (h *Handler) offlineAfter(ctx context.Context) time.Duration {
	raw, err := h.repo.ListSettings(ctx)
	if err != nil {
		return 60 * time.Second
	}
	return time.Duration(settingInt(raw, settingServerOfflineAfter, 60, 1)) * time.Second
}

func effectiveServerStatus(s repo.Server, offlineAfter time.Duration, now time.Time) string {
	if s.Status == repo.ServerStatusDisabled {
		return repo.ServerStatusDisabled
	}
	if s.LastSeenAt == nil || now.Sub(*s.LastSeenAt) > offlineAfter {
		return repo.ServerStatusOffline
	}
	return repo.ServerStatusActive
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

func dedupeIDs(ids []int64) []int64 {
	seen := make(map[int64]bool, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
