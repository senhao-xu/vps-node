<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
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

const showCreate = ref(false)
const editTarget = ref<NodeBrief | null>(null)
const deleteTarget = ref<NodeBrief | null>(null)
const deleting = ref(false)

let searchTimer: ReturnType<typeof setTimeout> | null = null
let suppressNextSearch = false

const columns: Column[] = [
  { key: 'select', label: '', width: '40px' },
  { key: 'name', label: '名称' },
  { key: 'protocol', label: '协议', width: '120px' },
  { key: 'port', label: '端口', align: 'right', width: '80px' },
  { key: 'server', label: '所属服务器', width: '160px' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'created_at', label: '创建时间', width: '160px' },
  { key: 'actions', label: '操作', width: '150px' },
]

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

function toggleSelect(id: number, checked: boolean) {
  if (checked) {
    if (!selectedIds.value.includes(id)) selectedIds.value = [...selectedIds.value, id]
  } else {
    selectedIds.value = selectedIds.value.filter((item) => item !== id)
  }
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
  if (protocol === 'shadowsocks' || protocol === 'vless' || protocol === 'hysteria2') {
    protocolFilter.value = protocol
  }
  const status = raw['status']
  if (status === 'active' || status === 'disabled') statusFilter.value = status
  if (typeof raw['q'] === 'string' && raw['q'] !== query.value) {
    suppressNextSearch = true
    query.value = raw['q']
  }
  const rawPage = typeof raw['page'] === 'string' ? Number(raw['page']) : NaN
  if (Number.isInteger(rawPage) && rawPage > 0) page.value = rawPage
}

watch(query, () => {
  if (suppressNextSearch) {
    suppressNextSearch = false
    return
  }
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    applyFilters()
  }, 300)
})

onMounted(() => {
  restoreFromQuery()
  void load()
  void loadServers()
})
</script>

<template>
  <section class="page">
    <div class="page-header">
      <div>
        <p class="eyebrow">
          INFRASTRUCTURE
        </p>
        <h1 class="page-title">
          节点
        </h1>
      </div>
      <button
        type="button"
        class="btn"
        @click="showCreate = true"
      >
        新建节点
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="card">
      <div class="toolbar-label">
        节点目录 <span>{{ total }} 个节点</span>
      </div>
      <div class="filters">
        <input
          v-model="query"
          class="search-input"
          type="text"
          placeholder="按名称搜索"
        >
        <select
          v-model="serverFilter"
          @change="applyFilters"
        >
          <option value="">
            全部服务器
          </option>
          <option
            v-for="server in servers"
            :key="server.id"
            :value="String(server.id)"
          >
            {{ server.name }}
          </option>
        </select>
        <select
          v-model="protocolFilter"
          @change="applyFilters"
        >
          <option value="">
            全部协议
          </option>
          <option value="shadowsocks">
            {{ protocolLabel('shadowsocks') }}
          </option>
          <option value="vless">
            {{ protocolLabel('vless') }}
          </option>
          <option value="hysteria2">
            {{ protocolLabel('hysteria2') }}
          </option>
        </select>
        <select
          v-model="statusFilter"
          @change="applyFilters"
        >
          <option value="">
            全部状态
          </option>
          <option value="active">
            启用
          </option>
          <option value="disabled">
            停用
          </option>
        </select>
        <button
          type="button"
          class="btn secondary"
          @click="resetFilters"
        >
          重置
        </button>
      </div>

      <div
        v-if="selectedIds.length > 0"
        class="batch-bar"
      >
        <span class="text-secondary">已选 {{ selectedIds.length }} 个节点</span>
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

      <DataTable
        :columns="columns"
        :rows="items"
        :loading="loading"
      >
        <template #cell-select="{ row }">
          <input
            type="checkbox"
            :checked="selectedIds.includes(row.id)"
            :aria-label="`选择节点 ${row.name}`"
            @change="toggleSelect(row.id, ($event.target as HTMLInputElement).checked)"
          >
        </template>
        <template #cell-protocol="{ row }">
          {{ protocolLabel(row.protocol) }}
        </template>
        <template #cell-server="{ row }">
          <RouterLink :to="`/servers/${row.server.id}`">
            {{ row.server.name }}
          </RouterLink>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="nodeStatusInfo(row.status)" />
        </template>
        <template #cell-created_at="{ row }">
          {{ formatDateTime(row.created_at) }}
        </template>
        <template #cell-actions="{ row }">
          <span class="actions">
            <button
              type="button"
              class="btn link"
              @click="editTarget = row"
            >编辑</button>
            <button
              type="button"
              class="btn link"
              :disabled="statusUpdatingId === row.id"
              @click="toggleStatus(row)"
            >
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </button>
            <button
              type="button"
              class="btn link"
              @click="deleteTarget = row"
            >删除</button>
          </span>
        </template>
        <template #empty>
          {{ loading ? '加载中…' : '没有符合条件的节点' }}
        </template>
      </DataTable>

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
.eyebrow {
  margin: 0 0 var(--spacing-xs);
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.toolbar-label {
  display: flex;
  justify-content: space-between;
  margin-bottom: var(--spacing-md);
  color: var(--color-text);
  font-weight: 600;
}

.toolbar-label span {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 400;
}

.actions {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.batch-bar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
}

.search-input {
  width: min(260px, 100%);
}

@media (max-width: 700px) {
  .search-input {
    width: 100%;
  }
}
</style>
