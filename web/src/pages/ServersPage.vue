<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Activity, Cpu, Plus, Server, ServerOff, Waypoints } from 'lucide-vue-next'
import { deleteServer, listServers, updateServer } from '@/api/servers'
import { getDashboard } from '@/api/dashboard'
import { errorMessage } from '@/api/http'
import type { Dashboard, Paged, Server as ServerItem, ServerStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import ServerFormDialog from '@/components/ServerFormDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import OverflowMenu, { type OverflowMenuItem } from '@/components/ui/OverflowMenu.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import StatCard from '@/components/ui/StatCard.vue'
import { formatDateTime, formatRelative } from '@/utils/format'
import { serverStatusInfo } from '@/utils/labels'

const router = useRouter()

const items = ref<ServerItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')
const overview = ref<Dashboard | null>(null)

const showCreate = ref(false)
const editTarget = ref<ServerItem | null>(null)
const deleteTarget = ref<ServerItem | null>(null)
const deleting = ref(false)
const statusUpdatingId = ref<number | null>(null)

const columns: Column[] = [
  { key: 'name', label: '名称', width: '140px' },
  { key: 'address', label: '地址', width: '200px' },
  { key: 'status', label: '状态', width: '72px' },
  { key: 'agent_version', label: 'Agent 版本', width: '84px' },
  { key: 'last_seen_at', label: '最后心跳', width: '110px' },
  { key: 'node_count', label: '节点数', align: 'right', width: '56px' },
  { key: 'online_users', label: '在线用户', align: 'right', width: '64px' },
  { key: 'actions', label: '', width: '48px' },
]

const offlineCount = computed(() =>
  overview.value ? Math.max(0, overview.value.servers_total - overview.value.servers_online) : 0,
)

const hotCount = computed(
  () => items.value.filter((server) => server.cpu_percent >= 80).length,
)

const nodeTotal = computed(() =>
  items.value.reduce((sum, server) => sum + server.node_count, 0),
)

function rowActions(row: ServerItem): OverflowMenuItem[] {
  return [
    { label: '详情', onSelect: () => void router.push(`/servers/${row.id}`) },
    { label: '编辑', onSelect: () => (editTarget.value = row) },
    {
      label: row.status === 'disabled' ? '启用' : '禁用',
      onSelect: () => void toggleStatus(row),
    },
    { label: '删除', danger: true, onSelect: () => (deleteTarget.value = row) },
  ]
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result: Paged<ServerItem> = await listServers({ page: page.value, pageSize: pageSize.value })
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

async function loadOverview() {
  try {
    overview.value = await getDashboard()
  } catch {
    overview.value = null
  }
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  void load()
}

async function toggleStatus(server: ServerItem) {
  const next: ServerStatus = server.status === 'disabled' ? 'active' : 'disabled'
  statusUpdatingId.value = server.id
  error.value = ''
  try {
    await updateServer(server.id, { status: next })
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
    await deleteServer(target.id)
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
  void loadOverview()
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="服务器"
      :subtitle="`共 ${total} 台服务器`"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          @click="showCreate = true"
        >
          <Plus :size="15" />
          创建服务器
        </button>
      </template>
    </PageHeader>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="overview"
      class="stat-grid"
    >
      <StatCard
        label="服务器总数"
        :value="overview.servers_total"
        :icon="Server"
      />
      <StatCard
        label="在线"
        :value="overview.servers_online"
        :icon="Activity"
        tone="success"
      />
      <StatCard
        label="离线"
        :value="offlineCount"
        :icon="ServerOff"
        :tone="offlineCount > 0 ? 'danger' : 'default'"
      />
      <StatCard
        label="高负载（CPU ≥ 80%）"
        :value="hotCount"
        :icon="Cpu"
        :tone="hotCount > 0 ? 'warning' : 'default'"
      />
      <StatCard
        label="节点数"
        :value="nodeTotal"
        :icon="Waypoints"
      />
    </div>
    <div
      v-else
      class="stat-grid"
    >
      <div
        v-for="n in 5"
        :key="n"
        class="card stat-skeleton"
      >
        <span class="skeleton skeleton-line" />
        <span class="skeleton skeleton-value" />
      </div>
    </div>

    <div class="table-card">
      <DataTable
        :columns="columns"
        :rows="items"
        :row-key="(row) => row.id"
        :loading="loading"
        :total-count="total"
      >
        <template #cell-name="{ row }">
          <RouterLink :to="`/servers/${row.id}`">
            {{ row.name }}
          </RouterLink>
        </template>
        <template #cell-address="{ row }">
          <span class="mono">{{ row.address }}</span>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="serverStatusInfo(row.status)" />
        </template>
        <template #cell-agent_version="{ row }">
          {{ row.agent_version || '—' }}
        </template>
        <template #cell-last_seen_at="{ row }">
          <span v-if="row.last_seen_at">
            {{ formatRelative(row.last_seen_at) }}
            <span class="text-secondary heartbeat">{{ formatDateTime(row.last_seen_at) }}</span>
          </span>
          <span
            v-else
            class="text-secondary"
          >从未</span>
        </template>
        <template #cell-actions="{ row }">
          <OverflowMenu
            :items="rowActions(row)"
            :label="`服务器 ${row.name} 的操作`"
          />
        </template>
        <template #empty>
          还没有服务器，点击右上角创建
        </template>
      </DataTable>

      <TablePaginator
        :page="page"
        :page-size="pageSize"
        :total="total"
        @change="onPageChange"
      />
    </div>

    <ServerFormDialog
      :open="showCreate"
      @close="showCreate = false"
      @saved="load"
    />
    <ServerFormDialog
      :open="editTarget !== null"
      :server="editTarget"
      @close="editTarget = null"
      @saved="load"
    />

    <ConfirmDialog
      :open="deleteTarget !== null"
      title="删除服务器"
      :message="`确定删除服务器「${deleteTarget?.name ?? ''}」吗？\n将同时删除其 Agent、节点、节点授权和当前连接；历史流量与连接日志按保留策略清理。`"
      danger
      confirm-text="删除"
      :loading="deleting"
      @cancel="deleteTarget = null"
      @confirm="confirmDelete"
    />
  </section>
</template>

<style scoped>
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: var(--spacing-md);
}

.table-card {
  margin-top: var(--spacing-md);
}

.heartbeat {
  display: block;
  font-size: var(--font-size-sm);
}
</style>
