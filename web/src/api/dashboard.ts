import { request } from './http'
import type { Dashboard } from './types'

export function getDashboard(): Promise<Dashboard> {
  return request<Dashboard>('/api/dashboard')
}
