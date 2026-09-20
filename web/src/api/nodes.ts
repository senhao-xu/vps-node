import { request } from './http'
import type {
  CreateNodeInput,
  NodeBrief,
  NodeDetail,
  Paged,
  UpdateNodeInput,
} from './types'

export function listNodes(
  params: { serverId?: number; page?: number; pageSize?: number } = {},
): Promise<Paged<NodeBrief>> {
  return request<Paged<NodeBrief>>('/api/nodes', {
    query: {
      server_id: params.serverId,
      page: params.page,
      page_size: params.pageSize,
    },
  })
}

export function getNode(nodeId: number): Promise<NodeDetail> {
  return request<NodeDetail>(`/api/nodes/${nodeId}`)
}

export function createNode(input: CreateNodeInput): Promise<NodeBrief> {
  return request<NodeBrief>('/api/nodes', { method: 'POST', body: input })
}

export function updateNode(nodeId: number, input: UpdateNodeInput): Promise<NodeBrief> {
  return request<NodeBrief>(`/api/nodes/${nodeId}`, { method: 'PUT', body: input })
}

export async function deleteNode(nodeId: number): Promise<void> {
  await request<unknown>(`/api/nodes/${nodeId}`, { method: 'DELETE' })
}
