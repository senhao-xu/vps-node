<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, X } from 'lucide-vue-next'
import { deleteNode, listNodes, updateNode } from '@/api/nodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { NodeBrief, NodeStatus, Paged, Protocol, Server } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import NodeFormDialog from '@/components/NodeFormDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import FilterChip from '@/components/ui/FilterChip.vue'
import OverflowMenu, { type OverflowMenuItem } from '@/components/ui/OverflowMenu.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import { formatDateTime } from '@/utils/format'
import { nodeStatusInfo, protocolLabel } from '@/utils/labels'

const route = useRoute()
const router = useRouter()

const items = ref<NodeBrief[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const servers = ref<Server[]>([])
const query = ref('')
const serverFilter = ref('')
const protocolFilter = ref<Protocol | ''>('')
const statusFilter = ref<NodeStatus | ''>('')

const selectedIds = ref<number[]>([])
const batchBusy = ref(false)
const statusUpdatingId = ref<number | null>(null)

const sortKey = ref('')
const sortDir = ref<'asc' | 'desc'>('asc')

const showCreate = ref(false)
const editTarget = ref<NodeBrief | null>(null)
const deleteTarget = ref<NodeBrief | null>(null)
const deleting = ref(false)

const columns: Column[] = [
  { key: 'name', label: '名称', width: '180px', sortable: true },
  { key: 'protocol', label: '协议', width: '90px' },
  { key: 'port', label: '端口', align: 'right', width: '72px', sortable: true },
  { key: 'rate', label: '倍率', align: 'right', width: '70px' },
  { key: 'tags', label: '标签', width: '150px' },
  { key: 'server', label: '所属服务器', width: '140px' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'enabled', label: '启用', width: '64px' },
  { key: 'created_at', label: '创建时间', width: '140px', sortable: true },
  { key: 'actions', label: '', width: '48px' },
]

const protocolOptions: Array<{ value: Protocol | ''; label: string }> = [
  { value: '', label: '全部协议' },
  { value: 'shadowsocks', label: protocolLabel('shadowsocks') },
  { value: 'vless', label: protocolLabel('vless') },
  { value: 'hysteria2', label: protocolLabel('hysteria2') },
  { value: 'anytls', label: protocolLabel('anytls') },
]

const statusOptions: Array<{ value: NodeStatus | ''; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' },
]

const serverChipLabel = computed(() => {
  if (!serverFilter.value) return '全部服务器'
  const hit = servers.value.find((server) => String(server.id) === serverFilter.value)
  return hit ? hit.name : '全部服务器'
})

const protocolChipLabel = computed(
  () => protocolOptions.find((option) => option.value === protocolFilter.value)?.label ?? '全部协议',
)

const statusChipLabel = computed(
  () => statusOptions.find((option) => option.value === statusFilter.value)?.label ?? '全部状态',
)

const sortedItems = computed(() => {
  if (!sortKey.value) return items.value
  const dir = sortDir.value === 'asc' ? 1 : -1
  return [...items.value].sort((a, b) => {
    if (sortKey.value === 'name') return a.name.localeCompare(b.name) * dir
    if (sortKey.value === 'port') return (a.port - b.port) * dir
    if (sortKey.value === 'created_at') return a.created_at.localeCompare(b.created_at) * dir
    return 0
  })
})

function onSort(key: string) {
  if (sortKey.value === key) {
    if (sortDir.value === 'asc') sortDir.value = 'desc'
    else {
      sortKey.value = ''
      sortDir.value = 'asc'
    }
  } else {
    sortKey.value = key
    sortDir.value = 'asc'
  }
}

function rowActions(row: NodeBrief): OverflowMenuItem[] {
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
    const serverId = Number(serverFilter.value)
    const result: Paged<NodeBrief> = await listNodes({
      serverId: Number.isInteger(serverId) && serverId > 0 ? serverId : undefined,
      protocol: protocolFilter.value || undefined,
      status: statusFilter.value || undefined,
      q: query.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
    })
    items.value = result.items
    total.value = result.total
    selectedIds.value = []
    if (result.items.length === 0 && result.total > 0 && page.value > 1) {
      page.value = Math.ceil(result.total / pageSize.value)
      syncQuery()
      void load()
    }
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

function syncQuery() {
  const next: Record<string, string> = {}
  if (serverFilter.value) next['server_id'] = serverFilter.value
  if (protocolFilter.value) next['protocol'] = protocolFilter.value
  if (statusFilter.value) next['status'] = statusFilter.value
  if (query.value.trim()) next['q'] = query.value.trim()
  if (page.value > 1) next['page'] = String(page.value)
  void router.replace({ query: next })
}

function applyFilters() {
  page.value = 1
  syncQuery()
  void load()
}

function resetFilters() {
  query.value = ''
  serverFilter.value = ''
  protocolFilter.value = ''
  statusFilter.value = ''
  applyFilters()
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  syncQuery()
  void load()
}

async function toggleStatus(node: NodeBrief) {
  const next: NodeStatus = node.status === 'active' ? 'disabled' : 'active'
  statusUpdatingId.value = node.id
  error.value = ''
  try {
    await updateNode(node.id, { status: next })
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    statusUpdatingId.value = null
  }
}

async function batchSetStatus(status: NodeStatus) {
  if (selectedIds.value.length === 0 || batchBusy.value) return
  batchBusy.value = true
  error.value = ''
  const results = await Promise.allSettled(
    selectedIds.value.map((id) => updateNode(id, { status })),
  )
  const failed = results.filter((result) => result.status === 'rejected').length
  if (failed > 0) error.value = `${failed} 个节点操作失败，请重试`
  batchBusy.value = false
  await load()
}

async function confirmDelete() {
  const target = deleteTarget.value
  if (!target) return
  deleting.value = true
  error.value = ''
  try {
    await deleteNode(target.id)
    deleteTarget.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    deleting.value = false
  }
}

async function loadServers() {
  try {
    const result = await listServers({ page: 1, pageSize: 100 })
    servers.value = result.items
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function restoreFromQuery() {
  const raw = route.query
  const serverId = typeof raw['server_id'] === 'string' ? Number(raw['server_id']) : NaN
  if (Number.isInteger(serverId) && serverId > 0) serverFilter.value = String(serverId)
  const protocol = raw['protocol']
  if (protocol === 'shadowsocks' || protocol === 'vless' || protocol === 'hysteria2' || protocol === 'anytls') {
    protocolFilter.value = protocol
  }
  const status = raw['status']
  if (status === 'active' || status === 'disabled') statusFilter.value = status
  if (typeof raw['q'] === 'string') {
    query.value = raw['q']
  }
  const rawPage = typeof raw['page'] === 'string' ? Number(raw['page']) : NaN
  if (Number.isInteger(rawPage) && rawPage > 0) page.value = rawPage
}

onMounted(() => {
  restoreFromQuery()
  void load()
  void loadServers()
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="节点"
      :subtitle="`共 ${total} 个节点`"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          @click="showCreate = true"
        >
          <Plus :size="15" />
          新建节点
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
        @search="applyFilters"
      />
      <FilterChip
        :label="serverChipLabel"
        :active="serverFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              type="button"
              class="menu-list-item"
              :class="{ selected: serverFilter === '' }"
              @click="serverFilter = ''; applyFilters(); close()"
            >
              全部服务器
            </button>
            <button
              v-for="server in servers"
              :key="server.id"
              type="button"
              class="menu-list-item"
              :class="{ selected: serverFilter === String(server.id) }"
              @click="serverFilter = String(server.id); applyFilters(); close()"
            >
              {{ server.name }}
            </button>
          </div>
        </template>
      </FilterChip>
      <FilterChip
        :label="protocolChipLabel"
        :active="protocolFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              v-for="option in protocolOptions"
              :key="option.value"
              type="button"
              class="menu-list-item"
              :class="{ selected: protocolFilter === option.value }"
              @click="protocolFilter = option.value; applyFilters(); close()"
            >
              {{ option.label }}
            </button>
          </div>
        </template>
      </FilterChip>
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
              @click="statusFilter = option.value; applyFilters(); close()"
            >
              {{ option.label }}
            </button>
          </div>
        </template>
      </FilterChip>
      <button
        v-if="serverFilter || protocolFilter || statusFilter || query"
        type="button"
        class="btn ghost small"
        @click="resetFilters"
      >
        <X :size="13" />
        重置
      </button>
    </div>

    <div class="table-card">
      <DataTable
        :columns="columns"
        :rows="sortedItems"
        :row-key="(row) => row.id"
        :loading="loading"
        selectable
        :selected="selectedIds"
        :sort-key="sortKey"
        :sort-dir="sortDir"
        :total-count="total"
        @update:selected="selectedIds = $event.map(Number)"
        @sort="onSort"
      >
        <template #cell-name="{ row }">
          <span class="name-cell">
            <i
              class="status-dot"
              :class="row.status === 'active' ? 'on' : 'off'"
            />
            {{ row.name }}
          </span>
        </template>
        <template #cell-protocol="{ row }">
          {{ protocolLabel(row.protocol) }}
        </template>
        <template #cell-rate="{ row }">
          {{ row.rate }}×
        </template>
        <template #cell-tags="{ row }">
          <span
            v-if="row.tags.length === 0"
            class="text-secondary"
          >—</span>
          <span
            v-else
            class="tag-list"
          >
            <span
              v-for="tag in row.tags"
              :key="tag"
              class="chip"
            >{{ tag }}</span>
          </span>
        </template>
        <template #cell-server="{ row }">
          <RouterLink :to="`/servers/${row.server.id}`">
            {{ row.server.name }}
          </RouterLink>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="nodeStatusInfo(row.status)" />
        </template>
        <template #cell-enabled="{ row }">
          <ToggleSwitch
            :model-value="row.status === 'active'"
            :disabled="statusUpdatingId === row.id"
            :label="`${row.status === 'active' ? '禁用' : '启用'}节点 ${row.name}`"
            @update:model-value="toggleStatus(row)"
          />
        </template>
        <template #cell-created_at="{ row }">
          {{ formatDateTime(row.created_at) }}
        </template>
        <template #cell-actions="{ row }">
          <OverflowMenu
            :items="rowActions(row)"
            :label="`节点 ${row.name} 的操作`"
          />
        </template>
        <template #empty>
          没有符合条件的节点
        </template>
      </DataTable>

      <div
        v-if="selectedIds.length > 0"
        class="batch-bar"
      >
        <button
          type="button"
          class="btn secondary small"
          :disabled="batchBusy"
          @click="batchSetStatus('active')"
        >
          批量启用
        </button>
        <button
          type="button"
          class="btn secondary small"
          :disabled="batchBusy"
          @click="batchSetStatus('disabled')"
        >
          批量禁用
        </button>
        <button
          type="button"
          class="btn link small"
          :disabled="batchBusy"
          @click="selectedIds = []"
        >
          清除选择
        </button>
      </div>

      <TablePaginator
        :page="page"
        :page-size="pageSize"
        :total="total"
        @change="onPageChange"
      />
    </div>

    <NodeFormDialog
      :open="showCreate"
      @close="showCreate = false"
      @saved="load"
    />
    <NodeFormDialog
      :open="editTarget !== null"
      :node="editTarget"
      @close="editTarget = null"
      @saved="load"
    />

    <ConfirmDialog
      :open="deleteTarget !== null"
      title="删除节点"
      :message="`确定删除节点「${deleteTarget?.name ?? ''}」吗？\n将移除该节点的用户授权，Agent 下次同步后停止该服务。`"
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

.status-dot {
  width: 8px;
  height: 8px;
  flex: none;
  border-radius: 50%;
  background: var(--color-offline);
}

.status-dot.on {
  background: var(--color-success);
}

.batch-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
  padding-top: var(--spacing-md);
}

.tag-list {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 4px;
}


</style>
