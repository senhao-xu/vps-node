import type {
  CustomNodeSourceType,
  NodeStatus,
  Protocol,
  ServerStatus,
  User,
  UserStatus,
} from '@/api/types'

export type Tone = 'success' | 'warning' | 'danger' | 'muted' | 'primary'

export interface StatusInfo {
  label: string
  tone: Tone
}

export function userStatusInfo(status: UserStatus): StatusInfo {
  switch (status) {
    case 'active':
      return { label: '正常', tone: 'success' }
    case 'disabled':
      return { label: '已禁用', tone: 'danger' }
    case 'expired':
      return { label: '已过期', tone: 'warning' }
    default:
      return { label: status, tone: 'muted' }
  }
}

export function displayUserStatus(user: Pick<User, 'status' | 'expires_at'>): StatusInfo {
  if (user.status === 'disabled') return userStatusInfo('disabled')
  if (user.expires_at) {
    const time = new Date(user.expires_at).getTime()
    if (!Number.isNaN(time) && time <= Date.now()) return userStatusInfo('expired')
  }
  return userStatusInfo(user.status)
}

export function serverStatusInfo(status: ServerStatus): StatusInfo {
  switch (status) {
    case 'active':
      return { label: '在线', tone: 'success' }
    case 'offline':
      return { label: '离线', tone: 'muted' }
    case 'disabled':
      return { label: '已禁用', tone: 'warning' }
    default:
      return { label: status, tone: 'muted' }
  }
}

export function nodeStatusInfo(status: NodeStatus): StatusInfo {
  return status === 'active'
    ? { label: '启用', tone: 'success' }
    : { label: '停用', tone: 'warning' }
}

export function protocolLabel(protocol: Protocol): string {
  switch (protocol) {
    case 'shadowsocks':
      return 'Shadowsocks'
    case 'vless':
      return 'VLESS'
    case 'hysteria2':
      return 'Hysteria2'
    case 'anytls':
      return 'AnyTLS'
    default:
      return protocol
  }
}

export function customNodeSourceLabel(sourceType: CustomNodeSourceType): string {
  switch (sourceType) {
    case 'links':
      return '分享链接'
    case 'subscription':
      return '订阅链接'
    default:
      return sourceType
  }
}

export const SHADOWSOCKS_METHODS = [
  '2022-blake3-aes-128-gcm',
  '2022-blake3-aes-256-gcm',
  '2022-blake3-chacha20-poly1305',
] as const
