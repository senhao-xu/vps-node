package web

import (
	"net/http"
	"sort"
	"time"

	"vps-node/internal/httpx"
	"vps-node/internal/repo"
)

func (h *Handler) handleDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := time.Now()

	usersTotal, err := h.repo.CountUsersTotal(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	usersOnline, err := h.repo.CountUsersOnline(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	devicesCurrent, err := h.repo.CountOnlineDevices(ctx)
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

	httpx.WriteJSON(w, http.StatusOK, dashboardDTO{
		UsersTotal:        usersTotal,
		UsersOnline:       usersOnline,
		ServersTotal:      serversTotal,
		ServersOnline:     serversOnline,
		TrafficTodayBytes: up + down,
		DevicesCurrent:    devicesCurrent,
	})
}

func (h *Handler) handleDashboardUserTraffic(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "today"
	}
	if rangeParam != "today" && rangeParam != "total" {
		writeErr(w, errInvalid("invalid range, want today or total"))
		return
	}

	filter := repo.TrafficFilter{}
	if rangeParam == "today" {
		from := time.Now().UTC().Truncate(24 * time.Hour)
		filter.From = &from
	}

	users, err := h.repo.ListUsersAll(ctx)
	if err != nil {
		writeErr(w, err)
		return
	}
	userSums, err := h.repo.SumTrafficByUser(ctx, filter)
	if err != nil {
		writeErr(w, err)
		return
	}
	nodeSums, err := h.repo.SumTrafficByUserNode(ctx, filter)
	if err != nil {
		writeErr(w, err)
		return
	}

	sumByUser := make(map[int64]repo.UserTrafficSum, len(userSums))
	for _, s := range userSums {
		sumByUser[s.UserID] = s
	}
	nodesByUser := make(map[int64][]dashboardUserNodeTrafficDTO)
	for _, n := range nodeSums {
		nodesByUser[n.UserID] = append(nodesByUser[n.UserID], dashboardUserNodeTrafficDTO{
			NodeID:        n.NodeID,
			NodeName:      n.NodeName,
			ServerID:      n.ServerID,
			ServerName:    n.ServerName,
			UploadBytes:   n.U,
			DownloadBytes: n.D,
			TotalBytes:    n.U + n.D,
		})
	}

	items := make([]dashboardUserTrafficItemDTO, 0, len(users))
	for _, u := range users {
		item := dashboardUserTrafficItemDTO{
			UserID:         u.ID,
			Username:       u.Username,
			Status:         u.Status,
			TransferEnable: u.TransferEnable,
			Nodes:          nodesByUser[u.ID],
		}
		if item.Nodes == nil {
			item.Nodes = []dashboardUserNodeTrafficDTO{}
		}
		if rangeParam == "total" {
			// users.u/users.d 与用户列表口径一致；上传/下载拆分只有流量记录可查，
			// 受保留策略与流量重置影响，合计以 u+d 为准。
			s := sumByUser[u.ID]
			item.UploadBytes = s.U
			item.DownloadBytes = s.D
			item.TotalBytes = u.UsedBytes()
		} else {
			s := sumByUser[u.ID]
			item.UploadBytes = s.U
			item.DownloadBytes = s.D
			item.TotalBytes = s.U + s.D
		}
		items = append(items, item)
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].TotalBytes != items[j].TotalBytes {
			return items[i].TotalBytes > items[j].TotalBytes
		}
		return items[i].UserID < items[j].UserID
	})

	httpx.WriteJSON(w, http.StatusOK, dashboardUserTrafficDTO{
		Range: rangeParam,
		Items: items,
	})
}
