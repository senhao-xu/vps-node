<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { getUserTraffic } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { TrafficBucket, TrafficSeries } from '@/api/types'
import TrafficChart from '@/components/TrafficChart.vue'
import { formatBytes } from '@/utils/format'

const props = defineProps<{
  userId: number
}>()

const range = ref<'7d' | '30d' | '90d'>('30d')
const bucket = ref<TrafficBucket>('day')
const data = ref<TrafficSeries | null>(null)
const loading = ref(false)
const error = ref('')

function toRfc3339(date: Date): string {
  return date.toISOString().replace(/\.\d+Z$/, 'Z')
}

function rangeBounds(): { from: string; to: string } {
  const to = new Date()
  const from = new Date()
  const days = range.value === '7d' ? 7 : range.value === '30d' ? 30 : 90
  from.setDate(from.getDate() - days)
  return { from: toRfc3339(from), to: toRfc3339(to) }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const { from, to } = rangeBounds()
    data.value = await getUserTraffic(props.userId, { from, to, bucket: bucket.value })
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

watch([range, bucket], () => {
  void load()
})

watch(
  () => props.userId,
  () => {
    void load()
  },
)

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="card">
    <div class="card-head">
      <h2 class="card-title">
        流量统计
      </h2>
      <div class="controls">
        <select v-model="range">
          <option value="7d">
            近 7 天
          </option>
          <option value="30d">
            近 30 天
          </option>
          <option value="90d">
            近 90 天
          </option>
        </select>
        <select v-model="bucket">
          <option value="day">
            按天
          </option>
          <option value="hour">
            按小时
          </option>
        </select>
      </div>
    </div>
    <p
      v-if="error"
      class="text-danger form-error"
    >
      {{ error }}
    </p>
    <div
      v-if="data"
      class="totals text-secondary"
    >
      <span>上行合计 {{ formatBytes(data.total_upload_bytes) }}</span>
      <span>下行合计 {{ formatBytes(data.total_download_bytes) }}</span>
      <span>总计 {{ formatBytes(data.total_upload_bytes + data.total_download_bytes) }}</span>
    </div>
    <TrafficChart
      v-if="data"
      :series="data.series"
    />
    <div
      v-else-if="loading"
      class="empty-tip"
    >
      加载中…
    </div>
  </div>
</template>

<style scoped>
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.card-head .card-title {
  margin-bottom: 0;
}

.controls {
  display: flex;
  gap: var(--spacing-sm);
}

.form-error {
  margin: var(--spacing-sm) 0;
  font-size: var(--font-size-sm);
}

.totals {
  display: flex;
  gap: var(--spacing-lg);
  font-size: var(--font-size-sm);
  margin: var(--spacing-sm) 0;
}

@media (max-width: 560px) {
  .card-head,
  .controls,
  .totals {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
