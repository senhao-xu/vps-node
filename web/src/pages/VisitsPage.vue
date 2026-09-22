<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { X } from 'lucide-vue-next'
import { getTopHosts, listVisits } from '@/api/visits'
import { listUsers } from '@/api/users'
import { listServers } from '@/api/servers'
import { listNodes } from '@/api/nodes'
import { errorMessage } from '@/api/http'
import type { NodeBrief, Server, TopHost, User, Visit } from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import { formatDateTime, localInputToIso } from '@/utils/format'

const items = ref<Visit[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const users = ref<User[]>([])
const servers = ref<Server[]>([])
const nodes = ref<NodeBrief[]>([])

const userFilter = ref('')
const serverFilter = ref('')
const nodeFilter = ref('')
const hostFilter = ref('')
const fromInput = ref('')
const toInput = ref('')

const topHosts = ref<TopHost[]>([])
const topLoading = ref(false)
const topError = ref('')

type TopRange = '7' | '30' | '90'

const topRange = ref<TopRange>('7')
const topRangeItems: Array<{ value: TopRange; label: string }> = [
  { value: '7', label: '近 7 天' },
  { value: '30', label: '近 30 天' },
  { value: '90', label: '近 90 天' },
]

const columns: Column[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'username', label: '用户' },
  { key: 'server_name', label: '服务器' },
  { key: 'node_name', label: '节点' },
  { key: 'target', label: '目标' },
  { key: 'network', label: '网络', width: '76px' },
  { key: 'client_ip', label: '来源 IP', width: '150px' },
]

const topColumns: Column[] = [
  { key: 'dest_host', label: '站点' },
  { key: 'hits', label: '访问次数', align: 'right', width: '120px' },
]

const userOptions = computed(() => [
  { value: '', label: '全部用户' },
  ...users.value.map((user) => ({ value: String(user.id), label: user.username })),
])

const serverOptions = computed(() => [
  { value: '', label: '全部服务器' },
  ...servers.value.map((server) => ({ value: String(server.id), label: server.name })),
])

const nodeOptions = computed(() => [
  { value: '', label: '全部节点' },
  ...nodes.value.map((node) => ({
    value: String(node.id),
    label: `${node.server.name} / ${node.name}`,
  })),
])

const hasFilters = computed(() =>
  Boolean(
    userFilter.value ||
      serverFilter.value ||
      nodeFilter.value ||
      hostFilter.value ||
      fromInput.value ||
      toInput.value,
  ),
)

function numeric(value: string): number | undefined {
  const id = Number(value)
  return Number.isInteger(id) && id > 0 ? id : undefined
}

function formatTarget(row: Visit): string {
  if (row.dest_port <= 0) return row.dest_host
  return row.dest_host.includes(':')
    ? `[${row.dest_host}]:${row.dest_port}`
    : `${row.dest_host}:${row.dest_port}`
}

function filterParams() {
  return {
    userId: numeric(userFilter.value),
    serverId: numeric(serverFilter.value),
    nodeId: numeric(nodeFilter.value),
    host: hostFilter.value.trim() || undefined,
    from: localInputToIso(fromInput.value),
    to: localInputToIso(toInput.value),
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listVisits({ ...filterParams(), page: page.value, pageSize: pageSize.value })
    items.value = result.items
    total.value = result.total
    if (result.items.length === 0 && result.total > 0 && page.value > 1) {
      page.value = Math.ceil(result.total / pageSize.value)
      void load()
    }
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function loadTop() {
  topLoading.value = true
  topError.value = ''
  try {
    const result = await getTopHosts({ ...filterParams(), days: Number(topRange.value), limit: 20 })
    topHosts.value = result.items
  } catch (err) {
    topError.value = errorMessage(err)
  } finally {
    topLoading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void load()
  void loadTop()
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  void load()
}

function onUserFilter(value: string) {
  userFilter.value = value
  applyFilters()
}

function onServerFilter(value: string) {
  serverFilter.value = value
  nodeFilter.value = ''
  void loadNodes()
  applyFilters()
}

function onNodeFilter(value: string) {
  nodeFilter.value = value
  applyFilters()
}

function onTopRangeChange(value: TopRange) {
  topRange.value = value
  void loadTop()
}

function resetFilters() {
  userFilter.value = ''
  serverFilter.value = ''
  nodeFilter.value = ''
  hostFilter.value = ''
  fromInput.value = ''
  toInput.value = ''
  void loadNodes()
  applyFilters()
}

async function loadUsers() {
  try {
    const result = await listUsers({ page: 1, pageSize: 100 })
    users.value = result.items
  } catch (err) {
    error.value = errorMessage(err)
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

async function loadNodes() {
  try {
    const result = await listNodes({
      serverId: numeric(serverFilter.value),
      page: 1,
      pageSize: 100,
    })
    nodes.value = result.items
    if (nodeFilter.value && !result.items.some((node) => String(node.id) === nodeFilter.value)) {
      nodeFilter.value = ''
    }
  } catch (err) {
    error.value = errorMessage(err)
  }
}

onMounted(() => {
  void loadUsers()
  void loadServers()
  void loadNodes()
  void load()
  void loadTop()
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="访问记录"
      :subtitle="`共 ${total} 条访问记录`"
    />
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="table-toolbar">
      <div class="filter-select">
        <AppSelect
          :model-value="userFilter"
          :options="userOptions"
          label="筛选用户"
          @update:model-value="onUserFilter"
        />
      </div>
      <div class="filter-select">
        <AppSelect
          :model-value="serverFilter"
          :options="serverOptions"
          label="筛选服务器"
          @update:model-value="onServerFilter"
        />
      </div>
      <div class="filter-select">
        <AppSelect
          :model-value="nodeFilter"
          :options="nodeOptions"
          label="筛选节点"
          @update:model-value="onNodeFilter"
        />
      </div>
      <SearchInput
        v-model="hostFilter"
        placeholder="按目标站点搜索"
        @search="applyFilters"
      />
      <label class="range-field">
        <span class="range-label">起</span>
        <input
          v-model="fromInput"
          type="datetime-local"
          @change="applyFilters"
        >
      </label>
      <label class="range-field">
        <span class="range-label">止</span>
        <input
          v-model="toInput"
          type="datetime-local"
          @change="applyFilters"
        >
      </label>
      <button
        v-if="hasFilters"
        type="button"
        class="btn ghost small"
        @click="resetFilters"
      >
        <X :size="13" />
        重置
      </button>
    </div>

    <div class="card">
      <div class="card-head">
        <h2 class="card-title">
          访问最多站点
        </h2>
        <SegmentedControl
          :items="topRangeItems"
          :model-value="topRange"
          aria-label="Top 站点时间范围"
          @update:model-value="onTopRangeChange"
        />
      </div>
      <ErrorBanner
        :message="topError"
        @dismiss="topError = ''"
      />
      <DataTable
        :columns="topColumns"
        :rows="topHosts"
        :row-key="(row) => row.dest_host"
        :loading="topLoading"
        :skeleton-rows="4"
        :bordered="false"
      >
        <template #cell-dest_host="{ row }">
          <span class="mono">{{ row.dest_host }}</span>
        </template>
        <template #cell-hits="{ row }">
          {{ row.hits }}
        </template>
        <template #empty>
          暂无聚合数据
        </template>
      </DataTable>
    </div>

    <div class="card">
      <DataTable
        :columns="columns"
        :rows="items"
        :row-key="(row) => row.id"
        :loading="loading"
        :bordered="false"
      >
        <template #cell-created_at="{ row }">
          {{ formatDateTime(row.created_at) }}
        </template>
        <template #cell-username="{ row }">
          <RouterLink :to="`/users/${row.user_id}`">
            {{ row.username }}
          </RouterLink>
        </template>
        <template #cell-server_name="{ row }">
          <RouterLink :to="`/servers/${row.server_id}`">
            {{ row.server_name }}
          </RouterLink>
        </template>
        <template #cell-node_name="{ row }">
          {{ row.node_name || '—' }}
        </template>
        <template #cell-target="{ row }">
          <span class="mono">{{ formatTarget(row) }}</span>
        </template>
        <template #cell-network="{ row }">
          {{ row.network || '—' }}
        </template>
        <template #cell-client_ip="{ row }">
          <span
            v-if="row.client_ip"
            class="mono"
          >{{ row.client_ip }}</span>
          <span
            v-else
            class="text-secondary"
          >—</span>
        </template>
        <template #empty>
          没有符合条件的访问记录
        </template>
      </DataTable>
      <TablePaginator
        :page="page"
        :page-size="pageSize"
        :total="total"
        @change="onPageChange"
      />
    </div>
  </section>
</template>

<style scoped>
.filter-select {
  width: 170px;
}

.range-field {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}

.range-label {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

@media (max-width: 700px) {
  .filter-select {
    width: 100%;
  }

  .range-field {
    width: 100%;
  }

  .range-field input {
    flex: 1;
    min-width: 0;
  }
}
</style>
