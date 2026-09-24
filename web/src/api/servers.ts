import { request } from './http'
import type {
  AgentKeyResult,
  CreateServerInput,
  CreateServerResult,
  Paged,
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

export function createServer(input: CreateServerInput): Promise<CreateServerResult> {
  return request<CreateServerResult>('/api/servers', { method: 'POST', body: input })
}

export function updateServer(serverId: number, input: UpdateServerInput): Promise<Server> {
  return request<Server>(`/api/servers/${serverId}`, { method: 'PUT', body: input })
}

export async function deleteServer(serverId: number): Promise<void> {
  await request<unknown>(`/api/servers/${serverId}`, { method: 'DELETE' })
}

export function getAgentKey(serverId: number): Promise<AgentKeyResult> {
  return request<AgentKeyResult>(`/api/servers/${serverId}/agent-key`)
}

export function generateAgentKey(serverId: number): Promise<AgentKeyResult> {
  return request<AgentKeyResult>(`/api/servers/${serverId}/agent-key`, { method: 'POST' })
}
