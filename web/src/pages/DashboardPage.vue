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
    <h1 class="page-title">
      仪表盘
    </h1>
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

.stat-card {
  padding: var(--spacing-md);
}

.stat-label {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 600;
  margin-top: var(--spacing-xs);
}

.refresh-tip {
  font-size: var(--font-size-sm);
  margin-top: var(--spacing-md);
}
</style>
