<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  ArrowDownUp,
  ChevronRight,
  Cpu,
  MonitorSmartphone,
  Plus,
  Server,
  ServerOff,
  Users,
  Waypoints,
} from 'lucide-vue-next'
import { getDashboardUserTraffic } from '@/api/dashboard'
import { errorMessage } from '@/api/http'
import { listNodes } from '@/api/nodes'
import { listServers } from '@/api/servers'
import type {
  DashboardUserTraffic,
  DashboardUserTrafficItem,
  DashboardUserTrafficRange,
  Server as ServerItem,
} from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import UserCreateDialog from '@/components/UserCreateDialog.vue'
import MetricStrip, { type MetricStripItem } from '@/components/ui/MetricStrip.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import { useOverviewStore } from '@/stores/overview'
import { formatBytes, formatRelative } from '@/utils/format'
import { userStatusInfo } from '@/utils/labels'

type AttentionItem = {
  server: ServerItem
  kind: 'offline' | 'load'
  detail: string
  severity: number
}

const router = useRouter()
const overview = useOverviewStore()
const showCreate = ref(false)
const overviewErrorVisible = ref(true)

const traffic = ref<DashboardUserTraffic | null>(null)
const trafficRange = ref<DashboardUserTrafficRange>('today')
const trafficRangeOptions: Array<{ value: DashboardUserTrafficRange; label: string }> = [
  { value: 'today', label: '今日' },
  { value: 'total', label: '累计' },
]
const trafficLoading = ref(false)
const trafficError = ref('')
const trafficQuery = ref('')
const expandedUsers = ref<Set<number>>(new Set())
let trafficRequestId = 0

const attentionServers = ref<ServerItem[]>([])
const attentionLoading = ref(false)
const attentionError = ref('')
const attentionComplete = ref(false)
const activeNodeTotal = ref<number | null>(null)
const activeNodeLoading = ref(false)
const activeNodeError = ref('')

const trafficColumns: Column[] = [
  { key: 'username', label: '用户' },
  { key: 'status', label: '状态', width: '82px' },
  { key: 'upload_bytes', label: '上传', align: 'right', width: '110px' },
  { key: 'download_bytes', label: '下载', align: 'right', width: '110px' },
  { key: 'total_bytes', label: '合计', align: 'right', width: '110px' },
  { key: 'quota', label: '配额使用', width: '190px' },
  { key: 'expand', label: '', width: '40px' },
]

const metrics = computed<MetricStripItem[]>(() => {
  const stats = overview.stats
  return [
    {
      key: 'users',
      label: '用户总数',
      value: stats?.users_total ?? '—',
      hint: stats ? '在线 ' + stats.users_online : undefined,
      icon: Users,
    },
    {
      key: 'traffic',
      label: '今日流量',
      value: stats ? formatBytes(stats.traffic_today_bytes) : '—',
      hint: 'UTC 0 点起',
      icon: ArrowDownUp,
    },
    {
      key: 'devices',
      label: '在线设备',
      value: stats?.devices_current ?? '—',
      hint: '按 IP 去重',
      icon: MonitorSmartphone,
    },
    {
      key: 'servers',
      label: '服务器',
      value: stats ? stats.servers_online + ' / ' + stats.servers_total : '—',
      hint: '在线',
      icon: Server,
      tone: !stats ? 'default' : stats.servers_online < stats.servers_total ? 'warning' : 'success',
    },
    {
      key: 'nodes',
      label: '运行节点',
      value: activeNodeLoading.value ? '…' : (activeNodeTotal.value ?? '—'),
      icon: Waypoints,
      tone: activeNodeError.value ? 'warning' : 'default',
    },
  ]
})

const filteredTraffic = computed(() => {
  const items = traffic.value?.items ?? []
  const query = trafficQuery.value.trim().toLowerCase()
  if (!query) return items
  return items.filter((item) => item.username.toLowerCase().includes(query))
})

const attentionItems = computed<AttentionItem[]>(() => {
  const result: AttentionItem[] = []
  for (const server of attentionServers.value) {
    if (server.status === 'offline') {
      result.push({
        server,
        kind: 'offline',
        detail:
          '最后心跳' +
          (server.last_seen_at ? '于 ' + formatRelative(server.last_seen_at) : '从未上报') +
          '，关联 ' +
          server.node_count +
          ' 个节点',
        severity: 200,
      })
      continue
    }
    if (server.status !== 'active') continue
    const load = Math.max(server.cpu_percent, server.memory_percent, server.disk_percent)
    if (load < 80) continue
    const parts = [
      ['CPU', server.cpu_percent],
      ['内存', server.memory_percent],
      ['磁盘', server.disk_percent],
    ]
      .filter((entry) => Number(entry[1]) >= 80)
      .map((entry) => entry[0] + ' ' + Number(entry[1]).toFixed(0) + '%')
    result.push({
      server,
      kind: 'load',
      detail: '当前' + parts.join('、'),
      severity: load,
    })
  }
  return result.sort((a, b) => b.severity - a.severity)
})

watch(
  () => overview.error,
  (message) => {
    if (message) overviewErrorVisible.value = true
  },
)

async function loadTraffic() {
  const requestId = ++trafficRequestId
  trafficLoading.value = true
  trafficError.value = ''
  try {
    const result = await getDashboardUserTraffic(trafficRange.value)
    if (requestId === trafficRequestId) traffic.value = result
  } catch (cause) {
    if (requestId === trafficRequestId) trafficError.value = errorMessage(cause)
  } finally {
    if (requestId === trafficRequestId) trafficLoading.value = false
  }
}

async function loadAttentionServers() {
  attentionLoading.value = true
  attentionError.value = ''
  attentionComplete.value = false
  const loaded: ServerItem[] = []
  try {
    let page = 1
    let total = 0
    do {
      const result = await listServers({ page, pageSize: 100 })
      loaded.push(...result.items)
      total = result.total
      page += 1
      if (result.items.length === 0) break
    } while (loaded.length < total)
    attentionServers.value = loaded
    attentionComplete.value = true
  } catch (cause) {
    if (loaded.length > 0) attentionServers.value = loaded
    attentionError.value = errorMessage(cause)
  } finally {
    attentionLoading.value = false
  }
}

async function loadActiveNodeTotal() {
  activeNodeLoading.value = true
  activeNodeError.value = ''
  try {
    const result = await listNodes({ status: 'active', page: 1, pageSize: 1 })
    activeNodeTotal.value = result.total
  } catch (cause) {
    activeNodeError.value = errorMessage(cause)
  } finally {
    activeNodeLoading.value = false
  }
}

function setRange(range: DashboardUserTrafficRange) {
  if (range === trafficRange.value) return
  trafficRange.value = range
  expandedUsers.value = new Set()
  void loadTraffic()
}

function toggleExpand(userId: number) {
  const next = new Set(expandedUsers.value)
  if (next.has(userId)) next.delete(userId)
  else next.add(userId)
  expandedUsers.value = next
}

function onRowClick(item: DashboardUserTrafficItem) {
  toggleExpand(item.user_id)
}

function quotaPercent(item: DashboardUserTrafficItem): number {
  if (item.transfer_enable <= 0) return 0
  return Math.min(100, (item.total_bytes / item.transfer_enable) * 100)
}

function quotaText(item: DashboardUserTrafficItem): string {
  if (item.transfer_enable <= 0) return formatBytes(item.total_bytes) + ' / 不限'
  return formatBytes(item.total_bytes) + ' / ' + formatBytes(item.transfer_enable)
}

function handleUserCreated() {
  void overview.refresh().catch(() => undefined)
  void loadTraffic()
}

onMounted(() => {
  void loadTraffic()
  void loadAttentionServers()
  void loadActiveNodeTotal()
})

onUnmounted(() => {
  trafficRequestId += 1
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="运营数据"
      subtitle="快速筛选、对比并处理用户与资源状态"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          @click="showCreate = true"
        >
          <Plus :size="15" />
          创建用户
        </button>
      </template>
    </PageHeader>

    <ErrorBanner
      v-if="overview.unavailable && overviewErrorVisible"
      :message="overview.error"
      @dismiss="overviewErrorVisible = false"
    />

    <MetricStrip
      :items="metrics"
      :loading="overview.loading && !overview.stats"
      aria-label="运营关键指标"
    />

    <div class="data-workspace">
      <section
        class="workspace-panel data-workspace-main"
        aria-labelledby="traffic-workspace-title"
      >
        <div class="workspace-toolbar">
          <div>
            <h2 id="traffic-workspace-title">
              用户流量
            </h2>
            <p>{{ trafficRange === 'today' ? '今日，UTC 0 点起' : '累计使用量' }}</p>
          </div>
          <div class="workspace-controls">
            <SearchInput
              v-model="trafficQuery"
              placeholder="搜索用户"
            />
            <SegmentedControl
              :items="trafficRangeOptions"
              :model-value="trafficRange"
              aria-label="流量时间范围"
              @update:model-value="setRange"
            />
          </div>
        </div>

        <ErrorBanner
          :message="trafficError"
          @dismiss="trafficError = ''"
        />

        <DataTable
          :columns="trafficColumns"
          :rows="filteredTraffic"
          :row-key="(row) => row.user_id"
          :loading="trafficLoading"
          :bordered="false"
          aria-label="用户流量表格"
          @row-click="onRowClick"
        >
          <template #cell-username="{ row }">
            <span class="username">{{ row.username }}</span>
            <span class="text-secondary user-id">#{{ row.user_id }}</span>
          </template>
          <template #cell-status="{ row }">
            <StatusBadge v-bind="userStatusInfo(row.status)" />
          </template>
          <template #cell-upload_bytes="{ row }">
            {{ formatBytes(row.upload_bytes) }}
          </template>
          <template #cell-download_bytes="{ row }">
            {{ formatBytes(row.download_bytes) }}
          </template>
          <template #cell-total_bytes="{ row }">
            {{ formatBytes(row.total_bytes) }}
          </template>
          <template #cell-quota="{ row }">
            <div class="quota-cell">
              <span class="quota-text">{{ quotaText(row) }}</span>
              <ProgressBar
                :percent="quotaPercent(row)"
                compact
              />
            </div>
          </template>
          <template #cell-expand="{ row }">
            <button
              type="button"
              class="expand-btn"
              :aria-expanded="expandedUsers.has(row.user_id)"
              :aria-label="'展开 ' + row.username + ' 的节点明细'"
              @click.stop="toggleExpand(row.user_id)"
            >
              <ChevronRight
                :size="15"
                class="chevron"
                :class="{ open: expandedUsers.has(row.user_id) }"
              />
            </button>
          </template>
          <template #row-extra="{ row, colspan }">
            <tr
              v-if="expandedUsers.has(row.user_id)"
              class="detail-row"
            >
              <td :colspan="colspan">
                <p
                  v-if="row.nodes.length === 0"
                  class="empty-tip"
                >
                  该范围内无流量记录
                </p>
                <div
                  v-else
                  class="node-table-wrap"
                  role="region"
                  tabindex="0"
                  :aria-label="row.username + ' 的节点流量明细'"
                >
                  <table class="node-table">
                    <thead>
                      <tr>
                        <th>节点</th>
                        <th>所属服务器</th>
                        <th class="num">
                          上传
                        </th>
                        <th class="num">
                          下载
                        </th>
                        <th class="num">
                          合计
                        </th>
                      </tr>
                    </thead>
                    <tbody>
                      <tr
                        v-for="node in row.nodes"
                        :key="node.node_id"
                      >
                        <td>{{ node.node_name }}</td>
                        <td>{{ node.server_name }}</td>
                        <td class="num">
                          {{ formatBytes(node.upload_bytes) }}
                        </td>
                        <td class="num">
                          {{ formatBytes(node.download_bytes) }}
                        </td>
                        <td class="num">
                          {{ formatBytes(node.total_bytes) }}
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </td>
            </tr>
          </template>
          <template #empty>
            {{ trafficQuery ? '没有匹配的用户' : '暂无用户流量数据' }}
          </template>
        </DataTable>
        <div class="workspace-footer">
          <span>共 {{ filteredTraffic.length }} 个用户</span>
          <span>展开行可查看节点明细</span>
        </div>
      </section>

      <aside class="attention-panel data-workspace-aside">
        <div class="attention-head">
          <div>
            <h2>需要关注</h2>
            <p>离线优先，其次按当前负载排序</p>
          </div>
          <span class="attention-count">{{ attentionItems.length }}</span>
        </div>

        <ErrorBanner
          v-if="attentionError"
          :message="attentionError"
          @dismiss="attentionError = ''"
        />
        <div
          v-if="attentionLoading && attentionServers.length === 0"
          class="attention-loading"
          aria-label="正在加载服务器状态"
        >
          <span
            v-for="n in 3"
            :key="n"
            class="skeleton attention-skeleton"
          />
        </div>
        <p
          v-else-if="attentionItems.length === 0 && attentionComplete"
          class="attention-empty"
        >
          当前没有离线或高负载服务器
        </p>
        <article
          v-for="item in attentionItems"
          v-else
          :key="item.server.id"
          class="attention-item"
        >
          <div
            class="attention-icon"
            :class="item.kind"
          >
            <ServerOff
              v-if="item.kind === 'offline'"
              :size="15"
            />
            <Cpu
              v-else
              :size="15"
            />
          </div>
          <div class="attention-body">
            <strong>{{ item.server.name }}</strong>
            <p>{{ item.detail }}</p>
            <button
              type="button"
              class="btn link"
              @click="router.push('/servers/' + item.server.id)"
            >
              查看服务器
            </button>
          </div>
        </article>
      </aside>
    </div>

    <UserCreateDialog
      :open="showCreate"
      @close="showCreate = false"
      @created="handleUserCreated"
    />
  </section>
</template>

<style scoped>
.workspace-panel,
.attention-panel {
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
}

.workspace-toolbar {
  display: flex;
  min-height: 54px;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding: 9px 12px;
  border-bottom: 1px solid var(--color-border);
}

.workspace-toolbar h2,
.attention-head h2 {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: 750;
}

.workspace-toolbar p,
.attention-head p {
  margin: 2px 0 0;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.workspace-controls {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
}

.workspace-controls :deep(.search-input) {
  width: 180px;
}

.workspace-panel :deep(.error-banner) {
  margin: 10px 12px;
}

.workspace-footer {
  display: flex;
  min-height: 40px;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding: 0 12px;
  border-top: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.username {
  font-weight: 650;
}

.user-id {
  margin-left: var(--spacing-xs);
  font-size: var(--font-size-xs);
}

.quota-cell {
  display: flex;
  min-width: 150px;
  flex-direction: column;
  gap: 3px;
}

.quota-text {
  font-size: var(--font-size-xs);
}

.expand-btn {
  display: inline-flex;
  width: 26px;
  height: 26px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.expand-btn:hover {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.chevron {
  transition: transform 0.15s ease;
}

.chevron.open {
  transform: rotate(90deg);
}

.detail-row td {
  padding: 10px 12px;
  background: var(--color-surface-muted);
}

.node-table-wrap {
  overflow-x: auto;
}

.node-table {
  width: 100%;
  min-width: 560px;
  border-collapse: collapse;
  font-size: var(--font-size-sm);
}

.node-table th,
.node-table td {
  padding: 7px var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
  text-align: left;
}

.node-table th {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 700;
}

.node-table th.num,
.node-table td.num {
  text-align: right;
}

.node-table tbody tr:last-child td {
  border-bottom: none;
}

.attention-head {
  display: flex;
  min-height: 54px;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding: 10px 12px;
  border-bottom: 1px solid var(--color-border);
}

.attention-count {
  display: grid;
  min-width: 24px;
  height: 24px;
  place-items: center;
  border-radius: var(--radius-sm);
  background: var(--color-surface-muted);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-variant-numeric: tabular-nums;
}

.attention-item {
  display: grid;
  grid-template-columns: 30px minmax(0, 1fr);
  gap: 9px;
  padding: 12px;
  border-bottom: 1px solid var(--color-border);
}

.attention-item:last-child {
  border-bottom: 0;
}

.attention-icon {
  display: grid;
  width: 28px;
  height: 28px;
  place-items: center;
  border-radius: var(--radius-sm);
}

.attention-icon.offline {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.attention-icon.load {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.attention-body {
  min-width: 0;
}

.attention-body strong {
  display: block;
  overflow: hidden;
  font-size: var(--font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.attention-body p {
  margin: 4px 0 3px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  line-height: 1.55;
}

.attention-body .btn.link {
  font-size: var(--font-size-xs);
}

.attention-loading {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 12px;
}

.attention-skeleton {
  display: block;
  width: 100%;
  height: 58px;
}

.attention-panel :deep(.error-banner) {
  margin: 10px 12px;
}

.attention-empty {
  margin: 0;
  padding: var(--spacing-lg) 12px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-align: center;
}

@media (max-width: 1024px) {
  .attention-panel {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .attention-head,
  .attention-empty,
  .attention-loading {
    grid-column: 1 / -1;
  }

  .attention-panel :deep(.error-banner) {
    grid-column: 1 / -1;
  }

  .attention-item:nth-of-type(even) {
    border-left: 1px solid var(--color-border);
  }
}

@media (max-width: 700px) {
  .workspace-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .workspace-controls {
    width: 100%;
  }

  .workspace-controls :deep(.search-input) {
    flex: 1;
    width: auto;
  }

  .workspace-footer {
    align-items: flex-start;
    flex-direction: column;
    justify-content: center;
    padding-block: 8px;
  }
}

@media (max-width: 560px) {
  .workspace-controls {
    align-items: stretch;
    flex-direction: column;
  }

  .attention-panel {
    grid-template-columns: minmax(0, 1fr);
  }

  .attention-item:nth-of-type(even) {
    border-left: 0;
  }
}
</style>
