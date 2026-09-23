<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { getUserVisits } from '@/api/visits'
import { errorMessage } from '@/api/http'
import type { Visit } from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import { formatDateTime, formatRelative } from '@/utils/format'

const props = defineProps<{
  userId: number
}>()

const items = ref<Visit[]>([])
const total = ref(0)
const loading = ref(false)
const error = ref('')

const columns: Column[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'node_name', label: '节点' },
  { key: 'target', label: '目标' },
  { key: 'network', label: '网络', width: '76px' },
  { key: 'client_ip', label: '来源 IP', width: '150px' },
]

function formatTarget(row: Visit): string {
  if (row.dest_port <= 0) return row.dest_host
  return row.dest_host.includes(':')
    ? `[${row.dest_host}]:${row.dest_port}`
    : `${row.dest_host}:${row.dest_port}`
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await getUserVisits(props.userId, { page: 1, pageSize: 20 })
    items.value = result.items
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

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
        访问站点
      </h2>
      <div class="card-head-actions">
        <span class="text-secondary count">共 {{ total }} 条</span>
        <button
          type="button"
          class="btn secondary small"
          :disabled="loading"
          @click="load"
        >
          刷新
        </button>
      </div>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <DataTable
      :columns="columns"
      :rows="items"
      :row-key="(row) => row.id"
      :loading="loading"
      :bordered="false"
    >
      <template #cell-created_at="{ row }">
        {{ formatDateTime(row.created_at) }}
        <span class="text-secondary">（{{ formatRelative(row.created_at) }}）</span>
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
        该用户暂无访问记录
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.count {
  font-size: var(--font-size-sm);
}
</style>
