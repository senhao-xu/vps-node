export type UserStatus = 'active' | 'disabled' | 'expired'
export type ServerStatus = 'active' | 'disabled' | 'offline'
export type NodeStatus = 'active' | 'disabled'
export type Protocol = 'shadowsocks' | 'vless' | 'hysteria2'
export type ConnectionLogStatus = 'active' | 'closed'
export type TrafficBucket = 'hour' | 'day'
export type UserExpiryFilter = 'valid' | 'expired'

export type Admin = {
  id: number
  username: string
}

export type Paged<T> = {
  items: T[]
  total: number
  page: number
  page_size: number
}

export type User = {
  id: number
  uuid: string
  username: string
  status: UserStatus
  quota_bytes: number
  used_bytes: number
  started_at: string | null
  expires_at: string | null
  node_count: number
  session_count: number
  created_at: string
}

export type UserDetail = User & {
  remaining_bytes: number
  used_percent: number
}

export type UserCreated = UserDetail & {
  token: string
}

export type NodeBrief = {
  id: number
  server_id: number
  name: string
  protocol: Protocol
  port: number
  status: NodeStatus
  created_at: string
}

export type NodeRef = {
  id: number
  name: string
}

export type NodeDetail = NodeBrief & {
  user_count: number
  online_users: number
  server: NodeRef
}

export type UserNodes = {
  node_ids: number[]
  nodes: NodeBrief[]
}

export type Session = {
  node_id: number
  server_id: number
  ip: string
  upload_bytes: number
  download_bytes: number
  connected_at: string
  last_seen_at: string
}

export type ConnectionLog = {
  id: number
  user_id: number
  node_id: number
  server_id: number
  ip: string
  protocol: Protocol
  upload_bytes: number
  download_bytes: number
  connected_at: string
  closed_at: string | null
  status: ConnectionLogStatus
}

export type TrafficPoint = {
  bucket_start: string
  upload_bytes: number
  download_bytes: number
}

export type TrafficSeries = {
  total_upload_bytes: number
  total_download_bytes: number
  series: TrafficPoint[]
}

export type Server = {
  id: number
  name: string
  address: string
  status: ServerStatus
  cpu_percent: number
  memory_percent: number
  disk_percent: number
  uptime_seconds: number
  agent_version: string
  last_seen_at: string | null
  node_count: number
  online_users: number
  created_at: string
}

export type AgentInfo = {
  id: number
  version: string
  last_seen_at: string | null
  connected_at: string
}

export type ServerDetail = Server & {
  revision: number
  agent: AgentInfo | null
  nodes: NodeBrief[]
}

export type Dashboard = {
  users_total: number
  users_online: number
  servers_total: number
  servers_online: number
  traffic_today_bytes: number
  sessions_current: number
}

export type Settings = {
  retention_raw_log_days: number
  retention_aggregate_days: number
  collection_connection_logs: boolean
  session_freshness_seconds: number
  server_offline_after_seconds: number
}

export type RegisterTokenResult = {
  register_token: string
  expires_at: string
}

export type AgentTokenResult = {
  agent_token: string
  expires_hint: null
}

export type NodeSettingsInput = Record<string, unknown>

export type CreateUserInput = {
  username: string
  quota_bytes?: number
  started_at?: string | null
  expires_at?: string | null
  node_ids?: number[]
}

export type UpdateUserInput = {
  status?: UserStatus
  username?: string
  quota_bytes?: number
  started_at?: string | null
  expires_at?: string | null
}

export type CreateServerInput = {
  name: string
  address: string
}

export type UpdateServerInput = {
  name?: string
  address?: string
  status?: ServerStatus
}

export type CreateNodeInput = {
  server_id: number
  name: string
  protocol: Protocol
  port: number
  settings?: NodeSettingsInput
}

export type UpdateNodeInput = {
  name?: string
  port?: number
  settings?: NodeSettingsInput
  status?: NodeStatus
}
