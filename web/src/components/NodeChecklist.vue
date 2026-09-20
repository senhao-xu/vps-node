<script setup lang="ts">
import { computed } from 'vue'
import type { NodeBrief, Server } from '@/api/types'
import { protocolLabel, nodeStatusInfo } from '@/utils/labels'

const props = defineProps<{
  nodes: NodeBrief[]
  servers: Server[]
  modelValue: number[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: number[]): void
}>()

const groups = computed(() => {
  const serverName = new Map(props.servers.map((s) => [s.id, s.name]))
  const map = new Map<number, { serverId: number; serverName: string; nodes: NodeBrief[] }>()
  for (const node of props.nodes) {
    let group = map.get(node.server_id)
    if (!group) {
      group = {
        serverId: node.server_id,
        serverName: serverName.get(node.server_id) ?? `Server #${node.server_id}`,
        nodes: [],
      }
      map.set(node.server_id, group)
    }
    group.nodes.push(node)
  }
  return [...map.values()]
})

function isSelected(node: NodeBrief): boolean {
  return props.modelValue.includes(node.id)
}

function selectable(node: NodeBrief): boolean {
  return node.status === 'active'
}

function toggle(node: NodeBrief, checked: boolean) {
  if (!selectable(node)) return
  const next = checked
    ? [...props.modelValue, node.id]
    : props.modelValue.filter((id) => id !== node.id)
  emit('update:modelValue', next)
}
</script>

<template>
  <div class="node-checklist">
    <div
      v-if="groups.length === 0"
      class="empty-tip"
    >
      暂无节点，请先创建节点
    </div>
    <div
      v-for="group in groups"
      :key="group.serverId"
      class="group"
    >
      <div class="group-title">
        {{ group.serverName }}
      </div>
      <label
        v-for="node in group.nodes"
        :key="node.id"
        class="node-item"
        :class="{ disabled: !selectable(node) }"
      >
        <input
          type="checkbox"
          :checked="isSelected(node)"
          :disabled="!selectable(node)"
          @change="toggle(node, ($event.target as HTMLInputElement).checked)"
        >
        <span class="name">{{ node.name }}</span>
        <span class="meta text-secondary">
          {{ protocolLabel(node.protocol) }} · {{ node.port }}
          <template v-if="!selectable(node)">（{{ nodeStatusInfo(node.status).label }}）</template>
        </span>
      </label>
    </div>
  </div>
</template>

<style scoped>
.node-checklist {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  max-height: 280px;
  overflow-y: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: var(--spacing-sm) var(--spacing-md);
}

.group-title {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  margin: var(--spacing-xs) 0;
}

.node-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: 2px 0;
  cursor: pointer;
}

.node-item.disabled {
  cursor: not-allowed;
  color: var(--color-text-secondary);
}

.node-item .name {
  min-width: 120px;
}

.node-item .meta {
  font-size: var(--font-size-sm);
}
</style>
