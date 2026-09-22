<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { Activity, ArrowDownUp, ChevronRight, Server, UserCheck, Users } from 'lucide-vue-next'
import { getDashboard, getDashboardUserTraffic } from '@/api/dashboard'
import { errorMessage } from '@/api/http'
import type {
  Dashboard,
  DashboardUserTraffic,
  DashboardUserTrafficItem,
  DashboardUserTrafficRange,
} from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import StatCard from '@/components/ui/StatCard.vue'
import { formatBytes } from '@/utils/format'
import { userStatusInfo } from '@/utils/labels'

const stats = ref<Dashboard | null>(null)
const loading = ref(false)
const error = ref('')

const traffic = ref<DashboardUserTraffic | null>(null)
const trafficRange = ref<DashboardUserTrafficRange>('today')
const trafficRangeOptions: Array<{ value: DashboardUserTrafficRange; label: string }> = [
  { value: 'today', label: '今日' },
  { value: 'total', label: '累计' },
]
const trafficLoading = ref(false)
const expandedUsers = ref<Set<number>>(new Set())

let timer: ReturnType<typeof setInterval> | null = null

const trafficColumns: Column[] = [
  { key: 'username', label: '用户' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'upload_bytes', label: '上传', align: 'right', width: '110px' },
  { key: 'download_bytes', label: '下载', align: 'right', width: '110px' },
  { key: 'total_bytes', label: '合计', align: 'right', width: '110px' },
  { key: 'quota', label: '配额使用', width: '200px' },
  { key: 'expand', label: '', width: '40px' },
]

async function loadTraffic() {
  trafficLoading.value = true
  error.value = ''
  try {
    traffic.value = await getDashboardUserTraffic(trafficRange.value)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    trafficLoading.value = false
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    stats.value = await getDashboard()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
  await loadTraffic()
}

function setRange(range: DashboardUserTrafficRange) {
  if (range === trafficRange.value) return
  trafficRange.value = range
  expandedUsers.value = new Set()
  void loadTraffic()
}

function toggleExpand(userId: number) {
  const next = new Set(expandedUsers.value)
  if (next.has(userId)) {
    next.delete(userId)
  } else {
    next.add(userId)
  }
  expandedUsers.value = next
}

function onRowClick(item: DashboardUserTrafficItem) {
  toggleExpand(item.user_id)
}

function ratioPercent(part: number, total: number): string {
  if (total <= 0) return '0%'
  return `${Math.round((part / total) * 100)}%`
}

function quotaPercent(item: DashboardUserTrafficItem): number {
  if (item.transfer_enable <= 0) return 0
  return Math.min(100, (item.total_bytes / item.transfer_enable) * 100)
}

function quotaText(item: DashboardUserTrafficItem): string {
  if (item.transfer_enable <= 0) return `${formatBytes(item.total_bytes)} / 不限`
  return `${formatBytes(item.total_bytes)} / ${formatBytes(item.transfer_enable)}`
}

onMounted(() => {
  void load()
  timer = setInterval(() => {
    void load()
  }, 30_000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="仪表盘"
      subtitle="实时掌握用户、节点与系统连接健康状态"
    >
      <template #actions>
        <span
          v-if="stats"
          class="health-pill"
          :class="{ attention: stats.servers_online < stats.servers_total }"
        >
          <i />
          {{ stats.servers_online === stats.servers_total ? '系统运行正常' : '有服务器需要关注' }}
        </span>
      </template>
    </PageHeader>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div
      v-if="stats"
      class="stat-grid"
    >
      <StatCard
        label="用户总数"
        :value="stats.users_total"
        :icon="Users"
        :hint="`在线 ${stats.users_online}`"
      />
      <StatCard
        label="在线用户"
        :value="stats.users_online"
        :icon="UserCheck"
        tone="success"
        :hint="`占比 ${ratioPercent(stats.users_online, stats.users_total)}`"
      />
      <StatCard
        label="Server 总数"
        :value="stats.servers_total"
        :icon="Server"
        :hint="`在线 ${stats.servers_online}`"
      />
      <StatCard
        label="在线 Server"
        :value="stats.servers_online"
        :icon="Activity"
        :tone="stats.servers_online < stats.servers_total ? 'warning' : 'success'"
        :hint="`占比 ${ratioPercent(stats.servers_online, stats.servers_total)}`"
      />
      <StatCard
        label="今日流量"
        :value="formatBytes(stats.traffic_today_bytes)"
        :icon="ArrowDownUp"
        hint="UTC 0 点起"
      />
      <StatCard
        label="在线设备"
        :value="stats.devices_current"
        :icon="Activity"
        hint="按 IP 去重"
      />
    </div>
    <div
      v-else-if="loading"
      class="stat-grid"
    >
      <div
        v-for="n in 6"
        :key="n"
        class="card stat-skeleton"
      >
        <span class="skeleton skeleton-line" />
        <span class="skeleton skeleton-value" />
      </div>
    </div>

    <div class="card traffic-card">
      <div class="traffic-header">
        <h2 class="card-title traffic-title">
          用户流量 <span>{{ trafficRange === 'today' ? '今日（UTC 0 点起）' : '累计' }}</span>
        </h2>
        <SegmentedControl
          :items="trafficRangeOptions"
          :model-value="trafficRange"
          aria-label="时间范围"
          @update:model-value="setRange"
        />
      </div>

      <DataTable
        :columns="trafficColumns"
        :rows="traffic?.items ?? []"
        :row-key="(row) => row.user_id"
        :loading="trafficLoading"
        :bordered="false"
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
            :aria-label="`展开 ${row.username} 的节点明细`"
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
              <table
                v-else
                class="node-table"
              >
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
            </td>
          </tr>
        </template>
        <template #empty>
          暂无用户流量数据
        </template>
      </DataTable>
    </div>

    <p class="text-secondary refresh-tip">
      每 30 秒自动刷新
    </p>
  </section>
</template>

<style scoped>
.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: var(--spacing-md);
}

@media (max-width: 1200px) {
  .stat-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 640px) {
  .stat-grid {
    grid-template-columns: 1fr;
  }
}

.stat-skeleton {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  min-height: 96px;
}

.skeleton-line {
  display: block;
  height: 12px;
  width: 40%;
}

.skeleton-value {
  display: block;
  height: 24px;
  width: 60%;
}

.health-pill {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: 7px 11px;
  border: 1px solid var(--color-primary-border);
  border-radius: 999px;
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.health-pill i {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--color-success);
}

.health-pill.attention {
  border-color: var(--color-warning-border);
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.health-pill.attention i {
  background: var(--color-warning);
}

.refresh-tip {
  font-size: var(--font-size-sm);
  margin-top: var(--spacing-md);
}

.traffic-card {
  margin-top: var(--spacing-lg);
}

.traffic-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  flex-wrap: wrap;
  margin-bottom: var(--spacing-md);
}

.traffic-title {
  margin: 0;
}

.traffic-title span {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 400;
  margin-left: var(--spacing-xs);
}

.traffic-card :deep(.data-table tbody tr) {
  cursor: pointer;
}

.username {
  font-weight: 600;
}

.user-id {
  margin-left: var(--spacing-xs);
  font-size: var(--font-size-sm);
}

.quota-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 160px;
}

.quota-text {
  font-size: var(--font-size-sm);
}

.expand-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  padding: 0;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-secondary);
  cursor: pointer;
}

.expand-btn:hover {
  background: var(--color-muted-soft);
  color: var(--color-text);
}

.chevron {
  transition: transform 0.15s ease;
}

.chevron.open {
  transform: rotate(90deg);
}

.detail-row td {
  background: var(--color-surface-muted);
  padding: var(--spacing-md) var(--spacing-lg);
}

.node-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-sm);
}

.node-table th {
  text-align: left;
  font-weight: 600;
  color: var(--color-text-secondary);
  padding: 6px var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
  white-space: nowrap;
}

.node-table td {
  padding: 8px var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
}

.node-table th.num,
.node-table td.num {
  text-align: right;
}

.node-table tbody tr:last-child td {
  border-bottom: none;
}
</style>
