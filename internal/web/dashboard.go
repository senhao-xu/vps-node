package web

import (
	"net/http"
	"time"

	"vps-node/internal/httpx"
)

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	usersTotal, err := h.repo.CountUsersTotal(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	freshCutoff := now.Add(-h.sessionFreshness(ctx))
	usersOnline, err := h.repo.CountUsersWithFreshSession(ctx, freshCutoff)
	if err != nil {
		writeErr(w, err)
		return
	}
	serversTotal, err := h.repo.CountServersTotal(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	serversOnline, err := h.repo.CountServersHealthy(ctx, now.Add(-h.offlineAfter(ctx)))
	if err != nil {
		writeErr(w, err)
		return
	}
	todayStart := now.UTC().Truncate(24 * time.Hour)
	up, down, err := h.repo.SumTrafficSince(ctx, todayStart)
	if err != nil {
		writeErr(w, err)
		return
	}
	sessionsCurrent, err := h.repo.CountActiveSessions(ctx, freshCutoff)
	if err != nil {
		writeErr(w, err)
		return
	}

	httpx.WriteJSON(w, http.StatusOK, dashboardDTO{
		UsersTotal:        usersTotal,
		UsersOnline:       usersOnline,
		ServersTotal:      serversTotal,
		ServersOnline:     serversOnline,
		TrafficTodayBytes: up + down,
		SessionsCurrent:   sessionsCurrent,
	})
}
