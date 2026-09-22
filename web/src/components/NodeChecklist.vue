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
        <span class="group-count">{{ group.nodes.length }} 个节点</span>
      </div>
      <label
        v-for="node in group.nodes"
        :key="node.id"
        class="node-item"
        :class="{ disabled: !selectable(node), checked: isSelected(node) }"
      >
        <input
          type="checkbox"
          :checked="isSelected(node)"
          :disabled="!selectable(node)"
          @change="toggle(node, ($event.target as HTMLInputElement).checked)"
        >
        <span class="name">{{ node.name }}</span>
        <span class="meta chip">
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
  border-radius: var(--radius-md);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-surface-muted);
}

.group {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: var(--spacing-sm);
}

.group-title {
  grid-column: 1 / -1;
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
  font-size: var(--font-size-sm);
  font-weight: 500;
  margin: var(--spacing-xs) 0 2px;
}

.group-count {
  font-weight: 400;
  color: var(--color-text-secondary);
}

.node-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.node-item:hover:not(.disabled) {
  border-color: var(--color-border-strong);
}

.node-item.checked {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.node-item.checked .meta {
  background: var(--color-surface);
}

.node-item.disabled {
  cursor: not-allowed;
  color: var(--color-text-secondary);
}

.node-item .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.node-item .meta {
  white-space: nowrap;
}

@media (max-width: 560px) {
  .group {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
