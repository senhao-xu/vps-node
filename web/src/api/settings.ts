import { request } from './http'
import type { Settings } from './types'

export function getSettings(): Promise<Settings> {
  return request<Settings>('/api/settings')
}

export function updateSettings(input: Partial<Settings>): Promise<Settings> {
  return request<Settings>('/api/settings', { method: 'PUT', body: input })
}
