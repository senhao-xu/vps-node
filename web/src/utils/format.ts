const BYTE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'] as const

export type QuotaUnit = 'MB' | 'GB' | 'TB'

const QUOTA_MULTIPLIERS: Record<QuotaUnit, number> = {
  MB: 1024 ** 2,
  GB: 1024 ** 3,
  TB: 1024 ** 4,
}

export function formatBytes(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '—'
  if (value < 1024) return `${Math.round(value)} B`
  let scaled = value
  let unitIndex = 0
  while (scaled >= 1024 && unitIndex < BYTE_UNITS.length - 1) {
    scaled /= 1024
    unitIndex += 1
  }
  const digits = scaled >= 100 ? 0 : scaled >= 10 ? 1 : 2
  return `${scaled.toFixed(digits)} ${BYTE_UNITS[unitIndex]}`
}

function pad(value: number): string {
  return String(value).padStart(2, '0')
}

export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  )
}

export function formatDateTimeShort(iso: string | null | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return `${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

export function formatDate(iso: string | null | undefined): string {
  const full = formatDateTime(iso)
  return full === '—' ? full : full.slice(0, 10)
}

export function formatRelative(iso: string | null | undefined): string {
  if (!iso) return '从未'
  const time = new Date(iso).getTime()
  if (Number.isNaN(time)) return '—'
  const diffSeconds = Math.floor((Date.now() - time) / 1000)
  if (diffSeconds < 0) return '刚刚'
  if (diffSeconds < 60) return `${diffSeconds} 秒前`
  return `${formatDuration(diffSeconds)}前`
}

export function formatDuration(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return '—'
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  if (days > 0) return `${days} 天 ${hours} 小时`
  if (hours > 0) return `${hours} 小时 ${minutes} 分钟`
  if (minutes > 0) return `${minutes} 分钟`
  return `${Math.floor(seconds)} 秒`
}

export function formatRemaining(expiresAt: string | null | undefined): string {
  if (!expiresAt) return '永不过期'
  const time = new Date(expiresAt).getTime()
  if (Number.isNaN(time)) return '—'
  const diffSeconds = Math.floor((time - Date.now()) / 1000)
  if (diffSeconds <= 0) return '已过期'
  return `剩余 ${formatDuration(diffSeconds)}`
}

export function formatPercent(value: number): string {
  if (!Number.isFinite(value) || value < 0) return '—'
  return value % 1 === 0 ? `${value.toFixed(0)}%` : `${value.toFixed(1)}%`
}

export function shortUuid(uuid: string, head = 8): string {
  return uuid.length <= head ? uuid : `${uuid.slice(0, head)}…`
}

export function isoToLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return ''
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

export function localInputToIso(value: string): string | null {
  if (!value) return null
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return null
  return date.toISOString().replace(/\.\d+Z$/, 'Z')
}

export function quotaFromInput(value: number, unit: QuotaUnit): number {
  if (!Number.isFinite(value) || value < 0) return 0
  return Math.round(value * QUOTA_MULTIPLIERS[unit])
}

export function splitQuota(bytes: number): { value: number; unit: QuotaUnit } {
  if (!Number.isFinite(bytes) || bytes <= 0) return { value: 0, unit: 'GB' }
  const { MB, GB, TB } = QUOTA_MULTIPLIERS
  if (bytes >= TB && bytes % GB === 0) return { value: bytes / TB, unit: 'TB' }
  if (bytes >= GB && bytes % GB === 0) return { value: bytes / GB, unit: 'GB' }
  if (bytes >= MB && bytes % MB === 0) return { value: bytes / MB, unit: 'MB' }
  if (bytes >= TB) return { value: Math.round((bytes / TB) * 100) / 100, unit: 'TB' }
  if (bytes >= GB) return { value: Math.round((bytes / GB) * 100) / 100, unit: 'GB' }
  return { value: Math.round((bytes / MB) * 100) / 100, unit: 'MB' }
}
