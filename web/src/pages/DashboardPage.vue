<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { getDashboard } from '@/api/dashboard'
import { errorMessage } from '@/api/http'
import type { Dashboard } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import { formatBytes } from '@/utils/format'

const stats = ref<Dashboard | null>(null)
const loading = ref(false)
const error = ref('')

let timer: ReturnType<typeof setInterval> | null = null

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
</style>
