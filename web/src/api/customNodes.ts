import { request } from './http'
import type {
  CreateCustomNodeInput,
  CustomNode,
  CustomNodeContent,
  CustomNodeEntries,
  CustomNodeEntrySelection,
  CustomNodeResult,
  CustomNodeShare,
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

export function getCustomNodeEntries(customNodeId: number): Promise<CustomNodeEntries> {
  return request<CustomNodeEntries>(`/api/custom-nodes/${customNodeId}/nodes`)
}

export function getCustomNodeShare(customNodeId: number): Promise<CustomNodeShare> {
  return request<CustomNodeShare>(`/api/custom-nodes/${customNodeId}/share`)
}

export function getCustomNodeContent(customNodeId: number): Promise<CustomNodeContent> {
  return request<CustomNodeContent>(`/api/custom-nodes/${customNodeId}/content`)
}

export function refreshCustomNode(customNodeId: number): Promise<CustomNodeEntries> {
  return request<CustomNodeEntries>(`/api/custom-nodes/${customNodeId}/refresh`, {
    method: 'POST',
  })
}

export function getUserCustomNodes(userId: number): Promise<UserCustomNodes> {
  return request<UserCustomNodes>(`/api/users/${userId}/custom-nodes`)
}

export function putUserCustomNodes(
  userId: number,
  customNodeIds: number[],
  entries: CustomNodeEntrySelection[],
): Promise<UserCustomNodes> {
  return request<UserCustomNodes>(`/api/users/${userId}/custom-nodes`, {
    method: 'PUT',
    body: { custom_node_ids: customNodeIds, custom_node_entries: entries },
  })
}
