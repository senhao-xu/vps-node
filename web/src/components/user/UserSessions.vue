<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { getUserSessions } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { NodeBrief, Server, Session } from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import { formatBytes, formatDateTime, formatRelative } from '@/utils/format'
import { buildNameMap, lookupName } from '@/utils/names'

const props = defineProps<{
  userId: number
  nodes: NodeBrief[]
  servers: Server[]
}>()

const items = ref<Session[]>([])
const loading = ref(false)
const error = ref('')

const nodeNames = computed(() => buildNameMap(props.nodes))
const serverNames = computed(() => buildNameMap(props.servers))

const columns: Column[] = [
  { key: 'node', label: '节点' },
  { key: 'server', label: '服务器' },
  { key: 'ip', label: 'IP' },
  { key: 'upload', label: '上行', align: 'right', width: '100px' },
  { key: 'download', label: '下行', align: 'right', width: '100px' },
  { key: 'connected_at', label: '连接时间', width: '160px' },
  { key: 'last_seen_at', label: '最后活动', width: '160px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await getUserSessions(props.userId)
    items.value = result.items
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="card">
    <div class="card-head">
      <h2 class="card-title">
        当前连接
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
    <p
      v-if="error"
      class="text-danger form-error"
    >
      {{ error }}
    </p>
    <DataTable
      :columns="columns"
      :rows="items"
      :loading="loading"
    >
      <template #cell-node="{ row }">
        {{ lookupName(nodeNames, row.node_id) }}
      </template>
      <template #cell-server="{ row }">
        {{ lookupName(serverNames, row.server_id) }}
      </template>
      <template #cell-ip="{ row }">
        <span class="mono">{{ row.ip }}</span>
      </template>
      <template #cell-upload="{ row }">
        {{ formatBytes(row.upload_bytes) }}
      </template>
      <template #cell-download="{ row }">
        {{ formatBytes(row.download_bytes) }}
      </template>
      <template #cell-connected_at="{ row }">
        {{ formatDateTime(row.connected_at) }}
      </template>
      <template #cell-last_seen_at="{ row }">
        {{ formatDateTime(row.last_seen_at) }}
        <span class="text-secondary">（{{ formatRelative(row.last_seen_at) }}）</span>
      </template>
      <template #empty>
        当前没有在线连接
      </template>
    </DataTable>
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

.form-error {
  margin: 0 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
}

@media (max-width: 560px) {
  .card-head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
