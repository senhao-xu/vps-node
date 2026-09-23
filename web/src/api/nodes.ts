import { request } from './http'
import type {
  CreateNodeInput,
  NodeBrief,
  NodeDetail,
  NodeStatus,
  Paged,
  Protocol,
  RealityKeypair,
  UpdateNodeInput,
} from './types'

export function listNodes(
  params: {
    serverId?: number
    protocol?: Protocol
    status?: NodeStatus
    q?: string
    page?: number
    pageSize?: number
  } = {},
): Promise<Paged<NodeBrief>> {
  return request<Paged<NodeBrief>>('/api/nodes', {
    query: {
      server_id: params.serverId,
      protocol: params.protocol,
      status: params.status,
      q: params.q,
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

export function generateRealityKeypair(): Promise<RealityKeypair> {
  return request<RealityKeypair>('/api/nodes/reality-keypair', { method: 'POST' })
}

export function updateNode(nodeId: number, input: UpdateNodeInput): Promise<NodeBrief> {
  return request<NodeBrief>(`/api/nodes/${nodeId}`, { method: 'PUT', body: input })
}

export async function deleteNode(nodeId: number): Promise<void> {
  await request<unknown>(`/api/nodes/${nodeId}`, { method: 'DELETE' })
}

export function copyNode(nodeId: number): Promise<NodeBrief> {
  return request<NodeBrief>(`/api/nodes/${nodeId}/copy`, { method: 'POST' })
}
