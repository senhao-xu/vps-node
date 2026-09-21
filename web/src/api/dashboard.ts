import { request } from './http'
import type { Dashboard, DashboardUserTraffic, DashboardUserTrafficRange } from './types'

export function getDashboard(): Promise<Dashboard> {
  return request<Dashboard>('/api/dashboard')
}

export function getDashboardUserTraffic(
  range: DashboardUserTrafficRange,
): Promise<DashboardUserTraffic> {
  return request<DashboardUserTraffic>('/api/dashboard/user-traffic', { query: { range } })
}
