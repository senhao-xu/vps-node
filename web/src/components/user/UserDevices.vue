<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { MonitorSmartphone } from 'lucide-vue-next'
import { getUserDevices } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { NodeBrief, OnlineDevice, Server } from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { formatDateTime, formatRelative } from '@/utils/format'
import { buildNameMap, lookupName } from '@/utils/names'

const props = defineProps<{
  userId: number
  nodes: NodeBrief[]
  servers: Server[]
}>()

const items = ref<OnlineDevice[]>([])
const loading = ref(false)
const error = ref('')

const nodeNames = computed(() => buildNameMap(props.nodes))
const serverNames = computed(() => buildNameMap(props.servers))

const columns: Column[] = [
  { key: 'ip', label: '来源 IP' },
  { key: 'online', label: '连接数', width: '96px' },
  { key: 'node', label: '节点' },
  { key: 'server', label: '服务器' },
  { key: 'last_seen_at', label: '最后活动', width: '220px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await getUserDevices(props.userId)
    items.value = result.items
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
        在线设备 <span class="device-count">{{ items.length }}</span>
      </h2>
      <button
        type="button"
        class="btn secondary small"
        :disabled="loading"
        @click="load"
      >
        刷新
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div
      v-if="loading && items.length === 0"
      class="devices-loading"
      role="status"
    >
      <LoadingSpinner size="sm" />
      正在读取在线设备…
    </div>
    <EmptyState
      v-else-if="!error && items.length === 0"
      :icon="MonitorSmartphone"
      title="暂无在线设备"
      hint="用户连接节点后，设备会显示在这里。"
    />
    <DataTable
      v-else-if="items.length > 0"
      :columns="columns"
      :rows="items"
      :row-key="(row) => `${row.node_id}-${row.ip}`"
      :loading="loading"
      :bordered="false"
      aria-label="用户在线设备"
    >
      <template #cell-ip="{ row }">
        <span class="mono">{{ row.ip }}</span>
      </template>
      <template #cell-online="{ row }">
        {{ row.online }}
      </template>
      <template #cell-node="{ row }">
        {{ lookupName(nodeNames, row.node_id) }}
      </template>
      <template #cell-server="{ row }">
        {{ lookupName(serverNames, row.server_id) }}
      </template>
      <template #cell-last_seen_at="{ row }">
        {{ formatDateTime(row.last_seen_at) }}
        <span class="text-secondary">（{{ formatRelative(row.last_seen_at) }}）</span>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.device-count {
  margin-left: var(--spacing-xs);
  padding: 2px 7px;
  border-radius: var(--radius-full);
  background: var(--color-surface-muted);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 600;
  vertical-align: middle;
}

.devices-loading {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  min-height: 96px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}
</style>
