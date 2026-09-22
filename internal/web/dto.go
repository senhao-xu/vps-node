package web

import (
	"encoding/json"
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
	ID             int64   `json:"id"`
	UUID           string  `json:"uuid"`
	Username       string  `json:"username"`
	Status         string  `json:"status"`
	TransferEnable int64   `json:"transfer_enable"`
	U              int64   `json:"u"`
	D              int64   `json:"d"`
	UsedBytes      int64   `json:"used_bytes"`
	SpeedLimit     int64   `json:"speed_limit"`
	DeviceLimit    int64   `json:"device_limit"`
	OnlineCount    int64   `json:"online_count"`
	LastOnlineAt   *string `json:"last_online_at"`
	StartedAt      *string `json:"started_at"`
	ExpiresAt      *string `json:"expires_at"`
	NodeCount      int64   `json:"node_count"`
	CreatedAt      string  `json:"created_at"`
}

type userDetailDTO struct {
	userDTO
	RemainingBytes int64   `json:"remaining_bytes"`
	UsedPercent    float64 `json:"used_percent"`
}

func toUserDTO(u repo.User, nodeCount int64) userDTO {
	return userDTO{
		ID:             u.ID,
		UUID:           u.UUID,
		Username:       u.Username,
		Status:         u.Status,
		TransferEnable: u.TransferEnable,
		U:              u.U,
		D:              u.D,
		UsedBytes:      u.UsedBytes(),
		SpeedLimit:     u.SpeedLimit,
		DeviceLimit:    u.DeviceLimit,
		OnlineCount:    u.OnlineCount,
		LastOnlineAt:   rfc3339Ptr(u.LastOnlineAt),
		StartedAt:      rfc3339Ptr(u.StartedAt),
		ExpiresAt:      rfc3339Ptr(u.ExpiresAt),
		NodeCount:      nodeCount,
		CreatedAt:      rfc3339(u.CreatedAt),
	}
}

func toUserDetailDTO(u repo.User, nodeCount int64) userDetailDTO {
	used := u.UsedBytes()
	var remaining int64
	if u.TransferEnable > 0 {
		remaining = u.TransferEnable - used
		if remaining < 0 {
			remaining = 0
		}
	}
	return userDetailDTO{
		userDTO:        toUserDTO(u, nodeCount),
		RemainingBytes: remaining,
		UsedPercent:    roundPercent(used, u.TransferEnable),
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
	Rate      float64    `json:"rate"`
	Tags      []string   `json:"tags"`
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
		Rate:      n.Rate,
		Tags:      nodeTags(n.Tags),
		Status:    n.Status,
		Server:    nodeRefDTO{ID: n.ServerID, Name: n.ServerName},
		CreatedAt: rfc3339(n.CreatedAt),
	}
}

func nodeTags(raw string) []string {
	tags := []string{}
	if raw == "" {
		return tags
	}
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}
	}
	return tags
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

type deviceDTO struct {
	NodeID     int64  `json:"node_id"`
	ServerID   int64  `json:"server_id"`
	IP         string `json:"ip"`
	Online     int    `json:"online"`
	LastSeenAt string `json:"last_seen_at"`
}

func toDeviceDTO(d repo.OnlineDevice) deviceDTO {
	return deviceDTO{
		NodeID:     d.NodeID,
		ServerID:   d.ServerID,
		IP:         d.IP,
		Online:     d.Online,
		LastSeenAt: rfc3339(d.LastSeenAt),
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
	DevicesCurrent    int64 `json:"devices_current"`
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
	UserID         int64                         `json:"user_id"`
	Username       string                        `json:"username"`
	Status         string                        `json:"status"`
	TransferEnable int64                         `json:"transfer_enable"`
	UploadBytes    int64                         `json:"upload_bytes"`
	DownloadBytes  int64                         `json:"download_bytes"`
	TotalBytes     int64                         `json:"total_bytes"`
	Nodes          []dashboardUserNodeTrafficDTO `json:"nodes"`
}

type dashboardUserTrafficDTO struct {
	Range string                        `json:"range"`
	Items []dashboardUserTrafficItemDTO `json:"items"`
}

type visitDTO struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"user_id"`
	Username   string `json:"username"`
	NodeID     int64  `json:"node_id"`
	NodeName   string `json:"node_name"`
	ServerID   int64  `json:"server_id"`
	ServerName string `json:"server_name"`
	DestHost   string `json:"dest_host"`
	DestPort   int    `json:"dest_port"`
	Network    string `json:"network"`
	ClientIP   string `json:"client_ip"`
	CreatedAt  string `json:"created_at"`
}

func toVisitDTO(v repo.VisitRecord) visitDTO {
	return visitDTO{
		ID:         v.ID,
		UserID:     v.UserID,
		Username:   v.Username,
		NodeID:     v.NodeID,
		NodeName:   v.NodeName,
		ServerID:   v.ServerID,
		ServerName: v.ServerName,
		DestHost:   v.DestHost,
		DestPort:   v.DestPort,
		Network:    v.Network,
		ClientIP:   v.ClientIP,
		CreatedAt:  rfc3339(v.CreatedAt),
	}
}

type topHostDTO struct {
	DestHost string `json:"dest_host"`
	Hits     int64  `json:"hits"`
}

type settingsDTO struct {
	RetentionAggregateDays      int    `json:"retention_aggregate_days"`
	RetentionVisitDays          int    `json:"retention_visit_days"`
	RetentionVisitAggregateDays int    `json:"retention_visit_aggregate_days"`
	CollectionVisits            bool   `json:"collection_visits"`
	ServerOfflineAfterSeconds   int    `json:"server_offline_after_seconds"`
	SubscribeURLs               string `json:"subscribe_urls"`
	SubscribePath               string `json:"subscribe_path"`
	SubscribeName               string `json:"subscribe_name"`
	ClashMetaTemplate           string `json:"clash_meta_template"`
}
