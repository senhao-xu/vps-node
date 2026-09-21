<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { getDashboard, getDashboardUserTraffic } from '@/api/dashboard'
import { errorMessage } from '@/api/http'
import type {
  Dashboard,
  DashboardUserTraffic,
  DashboardUserTrafficItem,
  DashboardUserTrafficRange,
} from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { formatBytes } from '@/utils/format'
import { userStatusInfo } from '@/utils/labels'

const stats = ref<Dashboard | null>(null)
const loading = ref(false)
const error = ref('')

const traffic = ref<DashboardUserTraffic | null>(null)
const trafficRange = ref<DashboardUserTrafficRange>('today')
const trafficLoading = ref(false)
const expandedUsers = ref<Set<number>>(new Set())

let timer: ReturnType<typeof setInterval> | null = null

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

function quotaPercent(item: DashboardUserTrafficItem): number {
  if (item.quota_bytes <= 0) return 0
  return Math.min(100, (item.total_bytes / item.quota_bytes) * 100)
}

function quotaText(item: DashboardUserTrafficItem): string {
  if (item.quota_bytes <= 0) return `${formatBytes(item.total_bytes)} / 不限`
  return `${formatBytes(item.total_bytes)} / ${formatBytes(item.quota_bytes)}`
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

interface StatCard {
  label: string
  value: string
}

function toCards(data: Dashboard): StatCard[] {
  return [
    { label: '用户总数', value: String(data.users_total) },
    { label: '在线用户', value: String(data.users_online) },
    { label: 'Server 总数', value: String(data.servers_total) },
    { label: '在线 Server', value: String(data.servers_online) },
    { label: '今日流量', value: formatBytes(data.traffic_today_bytes) },
    { label: '当前连接数', value: String(data.sessions_current) },
  ]
}
</script>

<template>
  <section class="page">
    <div class="page-header dashboard-header">
      <div>
        <p class="eyebrow">
          OPERATIONS OVERVIEW
        </p>
        <h1 class="page-title">
          仪表盘
        </h1>
        <p class="page-intro">
          实时掌握用户、节点与系统连接健康状态。
        </p>
      </div>
      <span
        v-if="stats"
        class="health-pill"
        :class="{ attention: stats.servers_online < stats.servers_total }"
      >
        <i />
        {{ stats.servers_online === stats.servers_total ? '系统运行正常' : '有服务器需要关注' }}
      </span>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div
      v-if="stats"
      class="stat-grid"
    >
      <div
        v-for="card in toCards(stats)"
        :key="card.label"
        class="card stat-card"
        :class="{ priority: card.label === '在线 Server' || card.label === '当前连接数' }"
      >
        <div class="stat-label">
          {{ card.label }}
        </div>
        <div class="stat-value">
          {{ card.value }}
        </div>
      </div>
    </div>
    <div
      v-else-if="loading"
      class="empty-tip"
    >
      加载中…
    </div>
    <p
      v-else-if="!error"
      class="empty-tip"
    >
      暂无数据
    </p>

    <div class="card traffic-card">
      <div class="traffic-header">
        <div class="toolbar-label">
          用户流量 <span>{{ trafficRange === 'today' ? '今日（UTC 0 点起）' : '累计' }}</span>
        </div>
        <div
          class="range-switch"
          role="group"
          aria-label="时间范围"
        >
          <button
            type="button"
            class="range-btn"
            :class="{ active: trafficRange === 'today' }"
            @click="setRange('today')"
          >
            今日
          </button>
          <button
            type="button"
            class="range-btn"
            :class="{ active: trafficRange === 'total' }"
            @click="setRange('total')"
          >
            累计
          </button>
        </div>
      </div>

      <div class="traffic-table-wrap">
        <table class="traffic-table">
          <thead>
            <tr>
              <th>用户</th>
              <th>状态</th>
              <th class="num">
                上传
              </th>
              <th class="num">
                下载
              </th>
              <th class="num">
                合计
              </th>
              <th>配额使用</th>
              <th class="expand-col" />
            </tr>
          </thead>
          <tbody>
            <tr v-if="trafficLoading && !traffic">
              <td
                colspan="7"
                class="empty-tip"
              >
                加载中…
              </td>
            </tr>
            <tr v-else-if="!traffic || traffic.items.length === 0">
              <td
                colspan="7"
                class="empty-tip"
              >
                暂无用户
              </td>
            </tr>
            <template
              v-for="item in traffic?.items ?? []"
              :key="item.user_id"
            >
              <tr
                class="user-row"
                @click="toggleExpand(item.user_id)"
              >
                <td>
                  <span class="username">{{ item.username }}</span>
                  <span class="text-secondary user-id">#{{ item.user_id }}</span>
                </td>
                <td>
                  <StatusBadge v-bind="userStatusInfo(item.status)" />
                </td>
                <td class="num">
                  {{ formatBytes(item.upload_bytes) }}
                </td>
                <td class="num">
                  {{ formatBytes(item.download_bytes) }}
                </td>
                <td class="num">
                  {{ formatBytes(item.total_bytes) }}
                </td>
                <td>
                  <div class="quota-cell">
                    <span class="quota-text">{{ quotaText(item) }}</span>
                    <ProgressBar
                      :percent="quotaPercent(item)"
                      compact
                    />
                  </div>
                </td>
                <td class="expand-col">
                  <button
                    type="button"
                    class="expand-btn"
                    :aria-expanded="expandedUsers.has(item.user_id)"
                    :aria-label="`展开 ${item.username} 的节点明细`"
                    @click.stop="toggleExpand(item.user_id)"
                  >
                    <span
                      class="chevron"
                      :class="{ open: expandedUsers.has(item.user_id) }"
                    >▸</span>
                  </button>
                </td>
              </tr>
              <tr
                v-if="expandedUsers.has(item.user_id)"
                class="detail-row"
              >
                <td colspan="7">
                  <p
                    v-if="item.nodes.length === 0"
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
                        v-for="node in item.nodes"
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
          </tbody>
        </table>
      </div>
    </div>

    <p class="text-secondary refresh-tip">
      每 30 秒自动刷新
    </p>
  </section>
</template>

<style scoped>
.stat-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(180px, 1fr));
  gap: var(--spacing-md);
}

.dashboard-header {
  align-items: flex-end;
}

.eyebrow {
  margin: 0 0 var(--spacing-xs);
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.page-intro {
  margin: calc(var(--spacing-sm) * -1) 0 0;
  color: var(--color-text-secondary);
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

.stat-card {
  position: relative;
  min-height: 112px;
  padding: var(--spacing-lg);
  overflow: hidden;
}

.stat-card::after {
  position: absolute;
  top: var(--spacing-md);
  right: var(--spacing-md);
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-primary-border);
  box-shadow: 0 0 0 6px var(--color-primary-soft);
  content: '';
}

.stat-card.priority {
  border-top: 3px solid var(--color-primary);
}

.stat-label {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.025em;
  margin-top: var(--spacing-sm);
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

.toolbar-label {
  color: var(--color-text);
  font-weight: 600;
}

.toolbar-label span {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 400;
}

.range-switch {
  display: inline-flex;
  padding: 2px;
  border: 1px solid var(--color-border);
  border-radius: 8px;
  background: var(--color-muted-soft);
  gap: 2px;
}

.range-btn {
  padding: 4px 14px;
  border: none;
  border-radius: 6px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition:
    background 0.15s ease,
    color 0.15s ease;
}

.range-btn:hover {
  color: var(--color-text);
}

.range-btn.active {
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-weight: 600;
  box-shadow: none;
}

.traffic-table-wrap {
  overflow-x: auto;
  margin: 0 calc(var(--spacing-md) * -1);
  padding: 0 var(--spacing-md);
  scrollbar-color: var(--color-border-strong) transparent;
}

.traffic-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-md);
  min-width: 760px;
}

.traffic-table th {
  text-align: left;
  font-weight: 600;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  padding: 10px var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
  white-space: nowrap;
  background: var(--color-surface-muted);
  letter-spacing: 0.02em;
}

.traffic-table td {
  padding: 12px var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
  vertical-align: middle;
}

.traffic-table th.num,
.traffic-table td.num {
  text-align: right;
}

.traffic-table tbody .user-row:hover {
  background: var(--color-table-hover);
}

.traffic-table tbody tr:last-child td {
  border-bottom: none;
}

.user-row {
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

.expand-col {
  width: 40px;
  text-align: center;
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
  display: inline-block;
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
