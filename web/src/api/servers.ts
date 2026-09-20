import { request } from './http'
import type {
  AgentTokenResult,
  CreateServerInput,
  Paged,
  RegisterTokenResult,
  Server,
  ServerDetail,
  UpdateServerInput,
} from './types'

export function listServers(params: { page?: number; pageSize?: number } = {}): Promise<Paged<Server>> {
  return request<Paged<Server>>('/api/servers', {
    query: { page: params.page, page_size: params.pageSize },
  })
}

export function getServer(serverId: number): Promise<ServerDetail> {
  return request<ServerDetail>(`/api/servers/${serverId}`)
}

export function createServer(input: CreateServerInput): Promise<Server> {
  return request<Server>('/api/servers', { method: 'POST', body: input })
}

export function updateServer(serverId: number, input: UpdateServerInput): Promise<Server> {
  return request<Server>(`/api/servers/${serverId}`, { method: 'PUT', body: input })
}

export async function deleteServer(serverId: number): Promise<void> {
  await request<unknown>(`/api/servers/${serverId}`, { method: 'DELETE' })
}

export function createRegisterToken(serverId: number): Promise<RegisterTokenResult> {
  return request<RegisterTokenResult>(`/api/servers/${serverId}/register-token`, { method: 'POST' })
}

export function rotateAgentToken(serverId: number): Promise<AgentTokenResult> {
  return request<AgentTokenResult>(`/api/servers/${serverId}/agent-token`, { method: 'POST' })
}
