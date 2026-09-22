import { request } from './http'
import type { Paged, TopHost, Visit } from './types'

export interface VisitListParams {
  userId?: number
  serverId?: number
  nodeId?: number
  host?: string
  from?: string | null
  to?: string | null
  page?: number
  pageSize?: number
}

export interface TopHostParams {
  userId?: number
  serverId?: number
  nodeId?: number
  host?: string
  from?: string | null
  to?: string | null
  days?: number
  limit?: number
}

function listQuery(params: VisitListParams) {
  return {
    user_id: params.userId,
    server_id: params.serverId,
    node_id: params.nodeId,
    host: params.host,
    from: params.from,
    to: params.to,
    page: params.page,
    page_size: params.pageSize,
  }
}

export function listVisits(params: VisitListParams = {}): Promise<Paged<Visit>> {
  return request<Paged<Visit>>('/api/visits', { query: listQuery(params) })
}

export function getUserVisits(userId: number, params: VisitListParams = {}): Promise<Paged<Visit>> {
  return request<Paged<Visit>>(`/api/users/${userId}/visits`, { query: listQuery(params) })
}

export function getServerVisits(
  serverId: number,
  params: VisitListParams = {},
): Promise<Paged<Visit>> {
  return request<Paged<Visit>>(`/api/servers/${serverId}/visits`, { query: listQuery(params) })
}

export function getTopHosts(params: TopHostParams = {}): Promise<Paged<TopHost>> {
  return request<Paged<TopHost>>('/api/visits/top', {
    query: {
      user_id: params.userId,
      server_id: params.serverId,
      node_id: params.nodeId,
      host: params.host,
      from: params.from,
      to: params.to,
      days: params.days,
      limit: params.limit,
    },
  })
}
