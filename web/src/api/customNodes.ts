import { request } from './http'
import type {
  CreateCustomNodeInput,
  CustomNode,
  CustomNodeResult,
  UpdateCustomNodeInput,
  UserCustomNodes,
} from './types'

export function listCustomNodes(): Promise<{ items: CustomNode[] }> {
  return request<{ items: CustomNode[] }>('/api/custom-nodes')
}

export function createCustomNode(input: CreateCustomNodeInput): Promise<CustomNodeResult> {
  return request<CustomNodeResult>('/api/custom-nodes', { method: 'POST', body: input })
}

export function updateCustomNode(
  customNodeId: number,
  input: UpdateCustomNodeInput,
): Promise<CustomNodeResult> {
  return request<CustomNodeResult>(`/api/custom-nodes/${customNodeId}`, {
    method: 'PUT',
    body: input,
  })
}

export async function deleteCustomNode(customNodeId: number): Promise<void> {
  await request<unknown>(`/api/custom-nodes/${customNodeId}`, { method: 'DELETE' })
}

export function getUserCustomNodes(userId: number): Promise<UserCustomNodes> {
  return request<UserCustomNodes>(`/api/users/${userId}/custom-nodes`)
}

export function putUserCustomNodes(
  userId: number,
  customNodeIds: number[],
): Promise<UserCustomNodes> {
  return request<UserCustomNodes>(`/api/users/${userId}/custom-nodes`, {
    method: 'PUT',
    body: { custom_node_ids: customNodeIds },
  })
}
