<script setup lang="ts">
import { ref, watch } from 'vue'
import { putUserNodes } from '@/api/users'
import { putUserCustomNodes } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, NodeBrief, Server } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import NodeChecklist from '@/components/NodeChecklist.vue'
import { customNodeSourceLabel } from '@/utils/labels'

const props = defineProps<{
  userId: number
  nodes: NodeBrief[]
  servers: Server[]
  nodeIds: number[]
  customNodes: CustomNode[]
  customNodeIds: number[]
}>()

const emit = defineEmits<{
  (e: 'saved', nodeIds: number[], customNodeIds: number[]): void
}>()

const selected = ref<number[]>([])
const selectedCustom = ref<number[]>([])
const dirty = ref(false)
const saving = ref(false)
const error = ref('')
const savedTip = ref(false)

watch(
  () => [props.userId, props.nodeIds, props.customNodeIds] as const,
  () => {
    selected.value = [...props.nodeIds]
    selectedCustom.value = [...props.customNodeIds]
    dirty.value = false
    savedTip.value = false
    error.value = ''
  },
  { immediate: true },
)

function sameIds(a: number[], b: number[]): boolean {
  return a.length === b.length && [...a].sort().join(',') === [...b].sort().join(',')
}

function refreshDirty() {
  dirty.value =
    !sameIds(selected.value, props.nodeIds) || !sameIds(selectedCustom.value, props.customNodeIds)
  savedTip.value = false
}

function onSelectionChange(value: number[]) {
  selected.value = value
  refreshDirty()
}

function toggleCustom(customNode: CustomNode, checked: boolean) {
  if (checked && customNode.status !== 'active') return
  selectedCustom.value = checked
    ? [...selectedCustom.value, customNode.id]
    : selectedCustom.value.filter((id) => id !== customNode.id)
  refreshDirty()
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const [nodesResult, customResult] = await Promise.all([
      putUserNodes(props.userId, selected.value),
      putUserCustomNodes(props.userId, selectedCustom.value),
    ])
    emit('saved', nodesResult.node_ids, customResult.custom_node_ids)
    dirty.value = false
    savedTip.value = true
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="card-head">
      <h2 class="card-title">
        节点授权
      </h2>
      <div class="card-head-actions">
        <span
          v-if="savedTip"
          class="saved-tip"
        >已保存</span>
        <button
          type="button"
          class="btn small"
          :disabled="!dirty || saving"
          :title="dirty ? '' : '勾选变化后可保存'"
          @click="save"
        >
          {{ saving ? '保存中…' : '保存授权' }}
        </button>
      </div>
    </div>
    <p class="text-secondary tip">
      用户只能使用被授权的节点；保存后立即生效，Agent 将在下次同步时应用。自定义节点仅影响订阅输出。
    </p>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <NodeChecklist
      :nodes="props.nodes"
      :servers="props.servers"
      :model-value="selected"
      @update:model-value="onSelectionChange"
    />
    <template v-if="customNodes.length > 0">
      <h3 class="custom-title">
        自定义节点
      </h3>
      <div class="custom-node-list">
        <label
          v-for="customNode in customNodes"
          :key="customNode.id"
          class="custom-node-item"
          :class="{
            disabled: customNode.status !== 'active' && !selectedCustom.includes(customNode.id),
            checked: selectedCustom.includes(customNode.id),
          }"
        >
          <input
            type="checkbox"
            :checked="selectedCustom.includes(customNode.id)"
            :disabled="customNode.status !== 'active' && !selectedCustom.includes(customNode.id)"
            @change="toggleCustom(customNode, ($event.target as HTMLInputElement).checked)"
          >
          <span class="name">{{ customNode.name }}</span>
          <span class="meta chip">{{ customNodeSourceLabel(customNode.source_type) }}</span>
        </label>
      </div>
    </template>
  </div>
</template>

<style scoped>
.saved-tip {
  color: var(--color-success);
  font-size: var(--font-size-sm);
}

.tip {
  margin: var(--spacing-xs) 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
}

.custom-title {
  margin: var(--spacing-md) 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.custom-node-list {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: var(--spacing-sm);
}

.custom-node-item {
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

.custom-node-item:hover:not(.disabled) {
  border-color: var(--color-border-strong);
}

.custom-node-item.checked {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.custom-node-item.disabled {
  cursor: not-allowed;
  color: var(--color-text-secondary);
}

.custom-node-item .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.custom-node-item .meta {
  white-space: nowrap;
}

@media (max-width: 560px) {
  .card-head-actions {
    margin-left: 0;
  }

  .custom-node-list {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
