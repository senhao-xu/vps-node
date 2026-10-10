<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Activity, CalendarClock, Download, GripVertical, Pencil, Plus, Server, ServerOff, Trash2 } from 'lucide-vue-next'
import { deleteServer, listServers, reorderServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { Paged, Server as ServerItem } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ServerBillingDialog from '@/components/ServerBillingDialog.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import ServerFormDialog from '@/components/ServerFormDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import MetricStrip, { type MetricStripItem } from '@/components/ui/MetricStrip.vue'
import { formatBytes, formatDate } from '@/utils/format'
import { serverStatusInfo } from '@/utils/labels'
import { useOverviewStore } from '@/stores/overview'

const router = useRouter()
const overview = useOverviewStore()

const items = ref<ServerItem[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const showCreate = ref(false)
const editTarget = ref<ServerItem | null>(null)
const billingTarget = ref<ServerItem | null>(null)
const deleteTarget = ref<ServerItem | null>(null)
const deleting = ref(false)
const ordering = ref(false)

const columns: Column[] = [
  { key: 'name', label: '名称', width: '30%' },
  { key: 'ip', label: 'IP', width: '15%' },
  { key: 'status', label: '状态', width: '9%' },
  { key: 'traffic', label: '流量', width: '16%' },
  { key: 'price', label: '价格', width: '9%' },
  { key: 'expiry', label: '到期', width: '11%' },
  { key: 'actions', label: '操作', width: '10%', align: 'right' },
]

const metrics = computed<MetricStripItem[]>(() => {
  const stats = overview.stats
  const offline = stats ? Math.max(0, stats.servers_total - stats.servers_online) : null
  return [
    {
      key: 'total',
      label: '服务器总数',
      value: stats?.servers_total ?? '—',
      icon: Server,
    },
    {
      key: 'online',
      label: '在线',
      value: stats?.servers_online ?? '—',
      icon: Activity,
      tone: stats ? 'success' : 'default',
    },
    {
      key: 'offline',
      label: '离线',
      value: offline ?? '—',
      icon: ServerOff,
      tone: offline && offline > 0 ? 'danger' : 'default',
    },
  ]
})

function editServer(row: ServerItem) {
  editTarget.value = row
}

function priceLabel(row: ServerItem): string {
  if (row.price_cents === 0) return '—'
  try { return new Intl.NumberFormat('en-US', { style: 'currency', currency: row.price_currency }).format(row.price_cents / 100) }
  catch { return `${row.price_currency} ${(row.price_cents / 100).toFixed(2)}` }
}

function exportServer(row: ServerItem) {
  const headers = ['名称', 'IP', '地区', '状态', '已用流量(字节)', '流量额度(字节)', '价格(分)', '货币', '计费周期', '到期']
  const values = [row.name, row.ip, row.region, row.status, row.monthly_used_bytes, row.traffic_limit_bytes, row.price_cents, row.price_currency, row.billing_cycle, row.expires_at ?? '']
  const csv = [headers, values].map((cells) => cells.map((cell) => `"${String(cell).replaceAll('"', '""')}"`).join(',')).join('\r\n')
  const url = globalThis.URL.createObjectURL(new globalThis.Blob(['\uFEFF', csv], { type: 'text/csv;charset=utf-8' }))
  const link = document.createElement('a')
  link.href = url
  link.download = `server-${row.id}.csv`
  link.click()
  globalThis.URL.revokeObjectURL(url)
}

async function onRowDrop(from: ServerItem, to: ServerItem) {
  if (ordering.value) return
  ordering.value = true
  error.value = ''
  try {
    const first = await listServers({ page: 1, pageSize: 100 })
    const all = [...first.items]
    for (let nextPage = 2; all.length < first.total; nextPage += 1) {
      const next = await listServers({ page: nextPage, pageSize: 100 })
      if (next.items.length === 0) throw new Error('服务器列表已变化，请刷新后重试')
      all.push(...next.items)
    }
    const fromIndex = all.findIndex((row) => row.id === from.id)
    const toIndex = all.findIndex((row) => row.id === to.id)
    if (fromIndex < 0 || toIndex < 0) return
    const [moved] = all.splice(fromIndex, 1)
    if (!moved) return
    all.splice(toIndex, 0, moved)
    await reorderServers(all.map((row) => row.id))
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    ordering.value = false
  }
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

async function reload() {
  await load()
  void overview.refresh().catch(() => undefined)
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  void load()
}

function onServerCreated(id: number) {
  void router.push(`/servers/${id}`)
}


async function confirmDelete() {
  const target = deleteTarget.value
  if (!target) return
  deleting.value = true
  error.value = ''
  try {
    await deleteServer(target.id)
    deleteTarget.value = null
    await reload()
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

    <MetricStrip
      :items="metrics"
      :loading="overview.loading && !overview.stats"
      aria-label="服务器状态汇总"
    />

    <div class="table-card">
      <DataTable
        :columns="columns"
        :rows="items"
        :row-key="(row) => row.id"
        :loading="loading"
        :total-count="total"
        draggable
        aria-label="服务器列表"
        @row-drop="onRowDrop"
      >
        <template #cell-name="{ row }">
          <div class="server-identity">
            <GripVertical
              :size="17"
              class="drag-handle"
              aria-hidden="true"
            />
            <RouterLink
              :to="'/servers/' + row.id"
              class="server-name"
            >
              {{ row.name }}
            </RouterLink>
            <span
              v-if="row.region"
              class="region-chip"
            >{{ row.region }}</span>
          </div>
        </template>
        <template #cell-ip="{ row }">
          <div class="ip-cell">
            <span class="mono">{{ row.ip || row.observed_ip || '—' }}</span>
            <span
              v-if="row.ipv6 || row.observed_ipv6"
              class="mono ip-v6"
              :title="row.ipv6 || row.observed_ipv6"
            >{{ row.ipv6 || row.observed_ipv6 }}</span>
          </div>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="serverStatusInfo(row.status)" />
        </template>
        <template #cell-traffic="{ row }">
          <span class="mono">{{ formatBytes(row.monthly_used_bytes) }}</span>
          <span class="server-meta"> / {{ row.traffic_limit_bytes ? formatBytes(row.traffic_limit_bytes) : '不限' }}</span>
        </template>
        <template #cell-price="{ row }">
          <span class="mono">{{ priceLabel(row) }}</span>
        </template>
        <template #cell-expiry="{ row }">
          <span class="mono">{{ row.expires_at ? formatDate(row.expires_at) : '—' }}</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              type="button"
              class="icon-action"
              :aria-label="`导出 ${row.name}`"
              title="导出服务器资料"
              @click="exportServer(row)"
            >
              <Download :size="17" />
            </button>
            <button
              type="button"
              class="icon-action"
              :aria-label="`编辑 ${row.name}`"
              title="编辑"
              @click="editServer(row)"
            >
              <Pencil :size="17" />
            </button>
            <button
              type="button"
              class="icon-action"
              :aria-label="`设置 ${row.name} 的到期时间`"
              title="设置到期时间"
              @click="billingTarget = row"
            >
              <CalendarClock :size="17" />
            </button>
            <button
              type="button"
              class="icon-action danger"
              :aria-label="`删除 ${row.name}`"
              title="删除"
              @click="deleteTarget = row"
            >
              <Trash2 :size="17" />
            </button>
          </div>
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
      @saved="reload"
      @created="onServerCreated"
    />
    <ServerFormDialog
      :open="editTarget !== null"
      :server="editTarget"
      @close="editTarget = null"
      @saved="reload"
    />
    <ServerBillingDialog
      :open="billingTarget !== null"
      :server="billingTarget"
      @close="billingTarget = null"
      @saved="reload"
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
.table-card { min-width: 0; }
.server-identity { display: flex; align-items: center; gap: 10px; min-width: 0; }
.drag-handle { flex: none; color: var(--color-text-secondary); cursor: grab; }
.server-name { color: var(--color-text); font-weight: 650; overflow-wrap: anywhere; }
.region-chip { flex: none; padding: 2px 8px; border: 1px solid var(--color-border); border-radius: var(--radius-full); color: var(--color-text-secondary); font-size: var(--font-size-xs); }
.server-meta { color: var(--color-text-secondary); }
.mono { font-variant-numeric: tabular-nums; white-space: nowrap; }
.ip-cell { display: flex; min-width: 0; flex-direction: column; gap: 2px; }
.ip-v6 { overflow: hidden; max-width: 100%; color: var(--color-text-secondary); text-overflow: ellipsis; }
.row-actions { display: flex; justify-content: flex-end; gap: 2px; }
.icon-action { display: inline-grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--color-text); cursor: pointer; }
.icon-action:hover { background: var(--color-surface-muted); }
.icon-action.danger { color: var(--color-danger); }
.icon-action:focus-visible { outline: 2px solid var(--color-focus-ring); }
:deep(.data-table) { min-width: 1000px; }
:deep(.badge.success) { background: var(--color-primary); color: var(--color-on-primary); }
:deep(.data-table th) { height: 38px; padding: 0 12px; font-size: var(--font-size-sm); color: var(--color-text); }
:deep(.data-table td) { height: 52px; padding: 8px 12px; }
</style>
