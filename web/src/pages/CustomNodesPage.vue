<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Plus, X } from 'lucide-vue-next'
import { deleteCustomNode, listCustomNodes, updateCustomNode } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, NodeStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import CustomNodeFormDialog from '@/components/CustomNodeFormDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import FilterChip from '@/components/ui/FilterChip.vue'
import OverflowMenu, { type OverflowMenuItem } from '@/components/ui/OverflowMenu.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import { formatDateTime } from '@/utils/format'
import { customNodeSourceLabel, nodeStatusInfo } from '@/utils/labels'

const items = ref<CustomNode[]>([])
const loading = ref(false)
const error = ref('')

const query = ref('')
const statusFilter = ref<NodeStatus | ''>('')

const showCreate = ref(false)
const editTarget = ref<CustomNode | null>(null)
const deleteTarget = ref<CustomNode | null>(null)
const deleting = ref(false)
const statusUpdatingId = ref<number | null>(null)

const columns: Column[] = [
  { key: 'name', label: '名称', width: '200px' },
  { key: 'source_type', label: '来源', width: '100px' },
  { key: 'cache', label: '上游缓存', width: '170px' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'enabled', label: '启用', width: '64px' },
  { key: 'created_at', label: '创建时间', width: '140px' },
  { key: 'actions', label: '', width: '48px', divider: true },
]

const statusOptions: Array<{ value: NodeStatus | ''; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' },
]

const statusChipLabel = computed(
  () => statusOptions.find((option) => option.value === statusFilter.value)?.label ?? '全部状态',
)

const filteredItems = computed(() => {
  const q = query.value.trim().toLowerCase()
  return items.value.filter((item) => {
    if (statusFilter.value && item.status !== statusFilter.value) return false
    if (q && !item.name.toLowerCase().includes(q)) return false
    return true
  })
})

function rowActions(row: CustomNode): OverflowMenuItem[] {
  return [
    { label: '编辑', onSelect: () => (editTarget.value = row) },
    {
      label: row.status === 'active' ? '禁用' : '启用',
      onSelect: () => void toggleStatus(row),
    },
    { label: '删除', danger: true, onSelect: () => (deleteTarget.value = row) },
  ]
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listCustomNodes()
    items.value = result.items
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function toggleStatus(node: CustomNode) {
  const next: NodeStatus = node.status === 'active' ? 'disabled' : 'active'
  statusUpdatingId.value = node.id
  error.value = ''
  try {
    await updateCustomNode(node.id, { status: next })
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    statusUpdatingId.value = null
  }
}

async function confirmDelete() {
  const target = deleteTarget.value
  if (!target) return
  deleting.value = true
  error.value = ''
  try {
    await deleteCustomNode(target.id)
    deleteTarget.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    deleting.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="自定义节点"
      :subtitle="`共 ${items.length} 个自定义节点 · 按用户授权后并入订阅输出`"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          @click="showCreate = true"
        >
          <Plus :size="15" />
          新建自定义节点
        </button>
      </template>
    </PageHeader>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="table-toolbar">
      <SearchInput
        v-model="query"
        placeholder="按名称搜索"
      />
      <FilterChip
        :label="statusChipLabel"
        :active="statusFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              v-for="option in statusOptions"
              :key="option.value"
              type="button"
              class="menu-list-item"
              :class="{ selected: statusFilter === option.value }"
              @click="statusFilter = option.value; close()"
            >
              {{ option.label }}
            </button>
          </div>
        </template>
      </FilterChip>
      <button
        v-if="statusFilter || query"
        type="button"
        class="btn ghost small"
        @click="statusFilter = ''; query = ''"
      >
        <X :size="13" />
        重置
      </button>
    </div>

    <div class="table-card">
      <DataTable
        :columns="columns"
        :rows="filteredItems"
        :row-key="(row) => row.id"
        :loading="loading"
        :total-count="filteredItems.length"
        aria-label="自定义节点列表"
      >
        <template #cell-name="{ row }">
          <span class="name-cell">
            <i
              class="status-dot"
              :class="row.status === 'active' ? 'success' : 'muted'"
            />
            {{ row.name }}
          </span>
        </template>
        <template #cell-source_type="{ row }">
          {{ customNodeSourceLabel(row.source_type) }}
        </template>
        <template #cell-cache="{ row }">
          <template v-if="row.source_type === 'subscription'">
            <span v-if="row.has_cache">已缓存 · {{ formatDateTime(row.fetched_at) }}</span>
            <span
              v-else
              class="text-secondary"
            >未拉取</span>
          </template>
          <span
            v-else
            class="text-secondary"
          >—</span>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="nodeStatusInfo(row.status)" />
        </template>
        <template #cell-enabled="{ row }">
          <ToggleSwitch
            :model-value="row.status === 'active'"
            :disabled="statusUpdatingId === row.id"
            :label="`${row.status === 'active' ? '禁用' : '启用'}自定义节点 ${row.name}`"
            @update:model-value="toggleStatus(row)"
          />
        </template>
        <template #cell-created_at="{ row }">
          {{ formatDateTime(row.created_at) }}
        </template>
        <template #cell-actions="{ row }">
          <OverflowMenu
            :items="rowActions(row)"
            :label="`自定义节点 ${row.name} 的操作`"
          />
        </template>
        <template #empty>
          还没有自定义节点，点击右上角新建
        </template>
      </DataTable>
    </div>

    <CustomNodeFormDialog
      :open="showCreate"
      @close="showCreate = false"
      @saved="load"
    />
    <CustomNodeFormDialog
      :open="editTarget !== null"
      :node="editTarget"
      @close="editTarget = null"
      @saved="load"
    />

    <ConfirmDialog
      :open="deleteTarget !== null"
      title="删除自定义节点"
      :message="`确定删除自定义节点「${deleteTarget?.name ?? ''}」吗？\n将同时移除所有用户对该节点的授权，订阅立即不再包含它。`"
      danger
      confirm-text="删除"
      :loading="deleting"
      @cancel="deleteTarget = null"
      @confirm="confirmDelete"
    />
  </section>
</template>

<style scoped>
.name-cell {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
  font-weight: 500;
}
</style>
