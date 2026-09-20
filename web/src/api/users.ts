import { request } from './http'
import type {
  ConnectionLog,
  CreateUserInput,
  Paged,
  Session,
  TrafficBucket,
  TrafficSeries,
  UpdateUserInput,
  User,
  UserCreated,
  UserDetail,
  UserExpiryFilter,
  UserNodes,
  UserStatus,
} from './types'

export interface UserListParams {
  query?: string
  status?: UserStatus
  expiry?: UserExpiryFilter
  page?: number
  pageSize?: number
}

export interface ConnectionLogParams {
  from?: string | null
  to?: string | null
  page?: number
  pageSize?: number
}

export interface UserTrafficParams {
  from?: string | null
  to?: string | null
  bucket?: TrafficBucket
}

export function listUsers(params: UserListParams = {}): Promise<Paged<User>> {
  return request<Paged<User>>('/api/users', {
    query: {
      query: params.query,
      status: params.status,
      expiry: params.expiry,
      page: params.page,
      page_size: params.pageSize,
    },
  })
}

export function getUser(userId: number): Promise<UserDetail> {
  return request<UserDetail>(`/api/users/${userId}`)
}

export function createUser(input: CreateUserInput): Promise<UserCreated> {
  return request<UserCreated>('/api/users', { method: 'POST', body: input })
}

export function updateUser(userId: number, input: UpdateUserInput): Promise<UserDetail> {
  return request<UserDetail>(`/api/users/${userId}`, { method: 'PUT', body: input })
}

export async function deleteUser(userId: number): Promise<void> {
  await request<unknown>(`/api/users/${userId}`, { method: 'DELETE' })
}

export function resetUserToken(userId: number): Promise<{ token: string }> {
  return request<{ token: string }>(`/api/users/${userId}/reset-token`, { method: 'POST' })
}

export function resetUserTraffic(userId: number): Promise<UserDetail> {
  return request<UserDetail>(`/api/users/${userId}/reset-traffic`, { method: 'POST' })
}

export function expireUserNow(userId: number): Promise<UserDetail> {
  return request<UserDetail>(`/api/users/${userId}/expire-now`, { method: 'POST' })
}

export function getUserNodes(userId: number): Promise<UserNodes> {
  return request<UserNodes>(`/api/users/${userId}/nodes`)
}

export function putUserNodes(userId: number, nodeIds: number[]): Promise<UserNodes> {
  return request<UserNodes>(`/api/users/${userId}/nodes`, {
    method: 'PUT',
    body: { node_ids: nodeIds },
  })
}

export function getUserSessions(userId: number, includeStale = false): Promise<{ items: Session[] }> {
  return request<{ items: Session[] }>(`/api/users/${userId}/sessions`, {
    query: { include_stale: includeStale ? 'true' : undefined },
  })
}

export function getUserConnectionLogs(
  userId: number,
  params: ConnectionLogParams = {},
): Promise<Paged<ConnectionLog>> {
  return request<Paged<ConnectionLog>>(`/api/users/${userId}/connection-logs`, {
    query: {
      from: params.from,
      to: params.to,
      page: params.page,
      page_size: params.pageSize,
    },
  })
}

export function getUserTraffic(
  userId: number,
  params: UserTrafficParams = {},
): Promise<TrafficSeries> {
  return request<TrafficSeries>(`/api/users/${userId}/traffic`, {
    query: {
      from: params.from,
      to: params.to,
      bucket: params.bucket,
    },
  })
}
