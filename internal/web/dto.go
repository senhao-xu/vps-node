package web

import (
	"time"

	"vps-node/internal/repo"
)

func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := rfc3339(*t)
	return &s
}

type userDTO struct {
	ID           int64   `json:"id"`
	UUID         string  `json:"uuid"`
	Username     string  `json:"username"`
	Status       string  `json:"status"`
	QuotaBytes   int64   `json:"quota_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	StartedAt    *string `json:"started_at"`
	ExpiresAt    *string `json:"expires_at"`
	NodeCount    int64   `json:"node_count"`
	SessionCount int64   `json:"session_count"`
	CreatedAt    string  `json:"created_at"`
}

type userDetailDTO struct {
	userDTO
	RemainingBytes int64   `json:"remaining_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

func toUserDTO(u repo.User, nodeCount, sessionCount int64) userDTO {
	return userDTO{
		ID:           u.ID,
		UUID:         u.UUID,
		Username:     u.Username,
		Status:       u.Status,
		QuotaBytes:   u.QuotaBytes,
		UsedBytes:    u.UsedBytes,
		StartedAt:    rfc3339Ptr(u.StartedAt),
		ExpiresAt:    rfc3339Ptr(u.ExpiresAt),
		NodeCount:    nodeCount,
		SessionCount: sessionCount,
		CreatedAt:    rfc3339(u.CreatedAt),
	}
}

func toUserDetailDTO(u repo.User, nodeCount, sessionCount int64) userDetailDTO {
	var remaining int64
	if u.QuotaBytes > 0 {
		remaining = u.QuotaBytes - u.UsedBytes
		if remaining < 0 {
			remaining = 0
		}
	}
	return userDetailDTO{
		userDTO:        toUserDTO(u, nodeCount, sessionCount),
		RemainingBytes: remaining,
		UsedPercent:    roundPercent(u.UsedBytes, u.QuotaBytes),
	}
}

type nodeRefDTO struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type nodeDTO struct {
	ID        int64      `json:"id"`
	ServerID  int64      `json:"server_id"`
	Name      string     `json:"name"`
	Protocol  string     `json:"protocol"`
	Port      int        `json:"port"`
	Status    string     `json:"status"`
	Server    nodeRefDTO `json:"server"`
	CreatedAt string     `json:"created_at"`
}

type nodeDetailDTO struct {
	nodeDTO
	UserCount   int64      `json:"user_count"`
	OnlineUsers int64      `json:"online_users"`
	Server      nodeRefDTO `json:"server"`
}

func toNodeDTO(n repo.Node) nodeDTO {
	return nodeDTO{
		ID:        n.ID,
		ServerID:  n.ServerID,
		Name:      n.Name,
		Protocol:  n.Protocol,
		Port:      n.Port,
		Status:    n.Status,
		Server:    nodeRefDTO{ID: n.ServerID, Name: n.ServerName},
		CreatedAt: rfc3339(n.CreatedAt),
	}
}

type agentInfoDTO struct {
	ID          int64   `json:"id"`
	Version     string  `json:"version"`
	LastSeenAt  *string `json:"last_seen_at"`
	ConnectedAt string  `json:"connected_at"`
}

type serverDTO struct {
	ID            int64   `json:"id"`
	Name          string  `json:"name"`
	Address       string  `json:"address"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	DiskPercent   float64 `json:"disk_percent"`
	UptimeSeconds int64   `json:"uptime_seconds"`
	AgentVersion  string  `json:"agent_version"`
	LastSeenAt    *string `json:"last_seen_at"`
	NodeCount     int64   `json:"node_count"`
	OnlineUsers   int64   `json:"online_users"`
	CreatedAt     string  `json:"created_at"`
}

type serverDetailDTO struct {
	serverDTO
	Revision int64         `json:"revision"`
	Agent    *agentInfoDTO `json:"agent"`
	Nodes    []nodeDTO     `json:"nodes"`
}

func toServerDTO(s repo.Server, nodeCount, onlineUsers int64) serverDTO {
	return serverDTO{
		ID:            s.ID,
		Name:          s.Name,
		Address:       s.Address,
		Status:        s.Status,
		CPUPercent:    s.CPUPercent,
		MemoryPercent: s.MemoryPercent,
		DiskPercent:   s.DiskPercent,
		UptimeSeconds: s.UptimeSeconds,
		AgentVersion:  s.AgentVersion,
		LastSeenAt:    rfc3339Ptr(s.LastSeenAt),
		NodeCount:     nodeCount,
		OnlineUsers:   onlineUsers,
		CreatedAt:     rfc3339(s.CreatedAt),
	}
}

type sessionDTO struct {
	NodeID        int64  `json:"node_id"`
	ServerID      int64  `json:"server_id"`
	IP            string `json:"ip"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	ConnectedAt   string `json:"connected_at"`
	LastSeenAt    string `json:"last_seen_at"`
}

func toSessionDTO(s repo.Session) sessionDTO {
	return sessionDTO{
		NodeID:        s.NodeID,
		ServerID:      s.ServerID,
		IP:            s.IP,
		UploadBytes:   s.UploadBytes,
		DownloadBytes: s.DownloadBytes,
		ConnectedAt:   rfc3339(s.ConnectedAt),
		LastSeenAt:    rfc3339(s.LastSeenAt),
	}
}

type connectionLogDTO struct {
	ID            int64   `json:"id"`
	UserID        int64   `json:"user_id"`
	NodeID        int64   `json:"node_id"`
	ServerID      int64   `json:"server_id"`
	IP            string  `json:"ip"`
	Protocol      string  `json:"protocol"`
	UploadBytes   int64   `json:"upload_bytes"`
	DownloadBytes int64   `json:"download_bytes"`
	ConnectedAt   string  `json:"connected_at"`
	ClosedAt      *string `json:"closed_at"`
	Status        string  `json:"status"`
}

func toConnectionLogDTO(l repo.ConnectionLog) connectionLogDTO {
	return connectionLogDTO{
		ID:            l.ID,
		UserID:        l.UserID,
		NodeID:        l.NodeID,
		ServerID:      l.ServerID,
		IP:            l.IP,
		Protocol:      l.Protocol,
		UploadBytes:   l.UploadBytes,
		DownloadBytes: l.DownloadBytes,
		ConnectedAt:   rfc3339(l.ConnectedAt),
		ClosedAt:      rfc3339Ptr(l.ClosedAt),
		Status:        l.Status,
	}
}

type trafficBucketDTO struct {
	BucketStart   string `json:"bucket_start"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
}

type trafficSeriesDTO struct {
	TotalUploadBytes   int64              `json:"total_upload_bytes"`
	TotalDownloadBytes int64              `json:"total_download_bytes"`
	Series             []trafficBucketDTO `json:"series"`
}

type dashboardDTO struct {
	UsersTotal        int64 `json:"users_total"`
	UsersOnline       int64 `json:"users_online"`
	ServersTotal      int64 `json:"servers_total"`
	ServersOnline     int64 `json:"servers_online"`
	TrafficTodayBytes int64 `json:"traffic_today_bytes"`
	SessionsCurrent   int64 `json:"sessions_current"`
}

type dashboardUserNodeTrafficDTO struct {
	NodeID        int64  `json:"node_id"`
	NodeName      string `json:"node_name"`
	ServerID      int64  `json:"server_id"`
	ServerName    string `json:"server_name"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
}

type dashboardUserTrafficItemDTO struct {
	UserID        int64                         `json:"user_id"`
	Username      string                        `json:"username"`
	Status        string                        `json:"status"`
	QuotaBytes    int64                         `json:"quota_bytes"`
	UploadBytes   int64                         `json:"upload_bytes"`
	DownloadBytes int64                         `json:"download_bytes"`
	TotalBytes    int64                         `json:"total_bytes"`
	Nodes         []dashboardUserNodeTrafficDTO `json:"nodes"`
}

type dashboardUserTrafficDTO struct {
	Range string                        `json:"range"`
	Items []dashboardUserTrafficItemDTO `json:"items"`
}

type settingsDTO struct {
	RetentionRawLogDays       int    `json:"retention_raw_log_days"`
	RetentionAggregateDays    int    `json:"retention_aggregate_days"`
	CollectionConnectionLogs  bool   `json:"collection_connection_logs"`
	SessionFreshnessSeconds   int    `json:"session_freshness_seconds"`
	ServerOfflineAfterSeconds int    `json:"server_offline_after_seconds"`
	SubscribeURLs             string `json:"subscribe_urls"`
	SubscribePath             string `json:"subscribe_path"`
	SubscribeName             string `json:"subscribe_name"`
	ClashMetaTemplate         string `json:"clash_meta_template"`
}
