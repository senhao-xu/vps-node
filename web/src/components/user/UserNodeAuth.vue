<script setup lang="ts">
import { ref, watch } from 'vue'
import { putUserNodes } from '@/api/users'
import { putUserCustomNodes } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, CustomNodeEntrySelection, NodeBrief, Server } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import NodeChecklist from '@/components/NodeChecklist.vue'
import CustomNodeEntryPickerDialog from '@/components/user/CustomNodeEntryPickerDialog.vue'
import { customNodeSourceLabel } from '@/utils/labels'

const props = defineProps<{
  userId: number
  nodes: NodeBrief[]
  servers: Server[]
  nodeIds: number[]
  customNodes: CustomNode[]
  customNodeIds: number[]
  customNodeEntries: CustomNodeEntrySelection[]
}>()

const emit = defineEmits<{
  (e: 'saved', nodeIds: number[], customNodeIds: number[], customNodeEntries: CustomNodeEntrySelection[]): void
}>()

const selected = ref<number[]>([])
const selectedCustom = ref<number[]>([])
const selectedCustomEntries = ref<Record<number, string[]>>({})
const dirty = ref(false)
const saving = ref(false)
const error = ref('')
const savedTip = ref(false)

const pickerOpen = ref(false)
const pickerSource = ref<CustomNode | null>(null)

function entryMap(entries: CustomNodeEntrySelection[]): Record<number, string[]> {
  const map: Record<number, string[]> = {}
  for (const selection of entries) {
    if (selection.entry_keys.length > 0) map[selection.custom_node_id] = [...selection.entry_keys]
  }
  return map
}

function sortedKeys(keys: string[]): string {
  return [...keys].sort().join(',')
}

function entriesEqual(
  local: Record<number, string[]>,
  remote: CustomNodeEntrySelection[],
): boolean {
  const remoteMap = new Map<number, string>()
  for (const selection of remote) {
    if (selection.entry_keys.length > 0) {
      remoteMap.set(selection.custom_node_id, sortedKeys(selection.entry_keys))
    }
  }
  const localIds = Object.keys(local).map(Number).filter((id) => (local[id]?.length ?? 0) > 0)
  if (localIds.length !== remoteMap.size) return false
  return localIds.every((id) => remoteMap.get(id) === sortedKeys(local[id] ?? []))
}

watch(
  () => [props.userId, props.nodeIds, props.customNodeIds, props.customNodeEntries] as const,
  () => {
    selected.value = [...props.nodeIds]
    selectedCustom.value = [...props.customNodeIds]
    selectedCustomEntries.value = entryMap(props.customNodeEntries)
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
    !sameIds(selected.value, props.nodeIds) ||
    !sameIds(selectedCustom.value, props.customNodeIds) ||
    !entriesEqual(selectedCustomEntries.value, props.customNodeEntries)
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
  if (!checked) {
    const next = { ...selectedCustomEntries.value }
    delete next[customNode.id]
    selectedCustomEntries.value = next
  }
  refreshDirty()
}

function entrySelectionCount(customNodeId: number): number {
  return selectedCustomEntries.value[customNodeId]?.length ?? 0
}

function entrySummary(customNodeId: number): string {
  const count = entrySelectionCount(customNodeId)
  return count > 0 ? `已选 ${count} 条` : '全部线路'
}

function openEntryPicker(customNode: CustomNode) {
  if (!selectedCustom.value.includes(customNode.id) || customNode.status !== 'active') return
  pickerSource.value = customNode
  pickerOpen.value = true
}

function onEntriesConfirmed(entryKeys: string[]) {
  const source = pickerSource.value
  pickerOpen.value = false
  if (!source) return
  const next = { ...selectedCustomEntries.value }
  if (entryKeys.length === 0) delete next[source.id]
  else next[source.id] = [...entryKeys]
  selectedCustomEntries.value = next
  refreshDirty()
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const entriesPayload: CustomNodeEntrySelection[] = selectedCustom.value
      .filter((id) => entrySelectionCount(id) > 0)
      .map((id) => ({ custom_node_id: id, entry_keys: [...(selectedCustomEntries.value[id] ?? [])] }))
    const [nodesResult, customResult] = await Promise.all([
      putUserNodes(props.userId, selected.value),
      putUserCustomNodes(props.userId, selectedCustom.value, entriesPayload),
    ])
    emit(
      'saved',
      nodesResult.node_ids,
      customResult.custom_node_ids,
      customResult.custom_node_entries,
    )
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
        <div
          v-for="customNode in customNodes"
          :key="customNode.id"
          class="custom-node-item"
          :class="{
            disabled: customNode.status !== 'active' && !selectedCustom.includes(customNode.id),
            checked: selectedCustom.includes(customNode.id),
          }"
        >
          <label class="custom-node-toggle">
            <input
              type="checkbox"
              :checked="selectedCustom.includes(customNode.id)"
              :disabled="customNode.status !== 'active' && !selectedCustom.includes(customNode.id)"
              @change="toggleCustom(customNode, ($event.target as HTMLInputElement).checked)"
            >
            <span class="name">{{ customNode.name }}</span>
          </label>
          <span class="meta chip">{{ customNodeSourceLabel(customNode.source_type) }}</span>
          <template v-if="selectedCustom.includes(customNode.id)">
            <span class="meta chip">{{ entrySummary(customNode.id) }}</span>
            <button
              type="button"
              class="btn small secondary"
              :disabled="customNode.status !== 'active'"
              @click="openEntryPicker(customNode)"
            >
              选择线路
            </button>
          </template>
        </div>
      </div>
    </template>

    <CustomNodeEntryPickerDialog
      :open="pickerOpen"
      :node="pickerSource"
      :selected-keys="pickerSource ? selectedCustomEntries[pickerSource.id] ?? [] : []"
      @close="pickerOpen = false"
      @confirm="onEntriesConfirmed"
    />
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
  flex-wrap: wrap;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
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
  color: var(--color-text-secondary);
}

.custom-node-toggle {
  display: flex;
  flex: 1;
  min-width: 0;
  align-items: center;
  gap: var(--spacing-sm);
  cursor: pointer;
}

.custom-node-item.disabled .custom-node-toggle {
  cursor: not-allowed;
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

.custom-node-item .btn.small {
  flex: none;
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
