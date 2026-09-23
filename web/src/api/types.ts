export type UserStatus = 'active' | 'disabled' | 'expired'
export type ServerStatus = 'active' | 'disabled' | 'offline'
export type NodeStatus = 'active' | 'disabled'
export type Protocol = 'shadowsocks' | 'vless' | 'hysteria2' | 'anytls'
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
  transfer_enable: number
  u: number
  d: number
  used_bytes: number
  speed_limit: number
  device_limit: number
  online_count: number
  last_online_at: string | null
  started_at: string | null
  expires_at: string | null
  node_count: number
  created_at: string
}

export type UserDetail = User & {
  remaining_bytes: number
  used_percent: number
}

export type UserCreated = UserDetail & {
  token: string
  subscription_url: string
}

export type NodeBrief = {
  id: number
  server_id: number
  address: string
  name: string
  protocol: Protocol
  port: number
  rate: number
  tags: string[]
  status: NodeStatus
  created_at: string
  server: NodeRef
}

export type NodeRef = {
  id: number
  name: string
}

export type NodeDetail = NodeBrief & {
  user_count: number
  online_users: number
  server: NodeRef
  settings?: NodeSettings
}

export type UserNodes = {
  node_ids: number[]
  nodes: NodeBrief[]
}

export type OnlineDevice = {
  node_id: number
  server_id: number
  ip: string
  online: number
  last_seen_at: string
}

export type OnlineDevices = {
  items: OnlineDevice[]
}

export type Visit = {
  id: number
  user_id: number
  username: string
  node_id: number
  node_name: string
  server_id: number
  server_name: string
  dest_host: string
  dest_port: number
  network: string
  client_ip: string
  created_at: string
}

export type TopHost = {
  dest_host: string
  hits: number
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
  devices_current: number
}

export type DashboardUserTrafficRange = 'today' | 'total'

export type DashboardUserNodeTraffic = {
  node_id: number
  node_name: string
  server_id: number
  server_name: string
  upload_bytes: number
  download_bytes: number
  total_bytes: number
}

export type DashboardUserTrafficItem = {
  user_id: number
  username: string
  status: UserStatus
  transfer_enable: number
  upload_bytes: number
  download_bytes: number
  total_bytes: number
  nodes: DashboardUserNodeTraffic[]
}

export type DashboardUserTraffic = {
  range: DashboardUserTrafficRange
  items: DashboardUserTrafficItem[]
}

export type Settings = {
  retention_aggregate_days: number
  retention_visit_days: number
  retention_visit_aggregate_days: number
  collection_visits: boolean
  server_offline_after_seconds: number
  subscribe_urls: string
  subscribe_path: string
  subscribe_name: string
  clash_meta_template: string
}

export type Subscription = { configured: boolean; url: string | null }

export type RegisterTokenResult = {
  register_token: string
  expires_at: string
}

export type AgentTokenResult = {
  agent_token: string
  expires_hint: null
}

export type RealityKeypair = {
  private_key: string
  public_key: string
  short_id: string
}

export type RealitySettingsInput = {
  server_name?: string
  server_port?: number
  public_key?: string
  short_id?: string
  allow_insecure?: boolean
}

export type TLSSettingsInput = {
  server_name?: string
  allow_insecure?: boolean
}

export type Hysteria2BandwidthInput = {
  up?: number
  down?: number
}

export type Hysteria2ObfsInput = {
  open?: boolean
  type?: string
  password?: string
}

/**
 * Public `protocol_settings` shape echoed by `GET /api/nodes/{id}`. Secret material
 * (vless `private_key`, TLS `certificate`/`private_key`, server `password`) is
 * encrypted at rest and never echoed.
 */
export type NodeSettings = {
  // shadowsocks
  cipher?: string
  obfs?: string | Hysteria2ObfsInput
  obfs_settings?: Record<string, unknown>
  plugin?: string
  plugin_opts?: string
  // vless
  tls?: number | TLSSettingsInput
  tls_settings?: Record<string, unknown>
  reality_settings?: RealitySettingsInput
  flow?: string
  network?: string
  network_settings?: Record<string, unknown>
  multiplex?: Record<string, unknown>
  utls?: Record<string, unknown>
  // hysteria2
  version?: number
  bandwidth?: Hysteria2BandwidthInput
  hop_interval?: string
  // anytls
  padding_scheme?: string | string[]
}

/**
 * Xboard-style nested `protocol_settings` payload. The shape is a superset of the
 * per-protocol sections so the form can build one typed object; the panel validates
 * the resulting object against the protocol-specific allowlist.
 */
export type NodeSettingsInput = NodeSettings & {
  // shared encrypted material / optional password
  password?: string
  certificate?: string
  private_key?: string
}

export type CreateUserInput = {
  username: string
  transfer_enable?: number
  speed_limit?: number
  device_limit?: number
  started_at?: string | null
  expires_at?: string | null
  node_ids?: number[]
}

export type UpdateUserInput = {
  status?: UserStatus
  username?: string
  transfer_enable?: number
  speed_limit?: number
  device_limit?: number
  started_at?: string | null
  expires_at?: string | null
}

export type CreateServerInput = {
  name: string
}

export type UpdateServerInput = {
  name?: string
  status?: ServerStatus
}

export type CreateNodeInput = {
  server_id: number
  address: string
  name: string
  protocol: Protocol
  port: number
  rate?: number
  tags?: string[]
  settings?: NodeSettingsInput
}

export type UpdateNodeInput = {
  address?: string
  name?: string
  port?: number
  rate?: number
  tags?: string[]
  settings?: NodeSettingsInput
  status?: NodeStatus
}
