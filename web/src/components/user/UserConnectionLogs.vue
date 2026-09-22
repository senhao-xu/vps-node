<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { getUserConnectionLogs } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { ConnectionLog, NodeBrief, Paged, Server } from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { formatBytes, formatDateTime, localInputToIso } from '@/utils/format'
import { logStatusInfo, protocolLabel } from '@/utils/labels'
import { buildNameMap, lookupName } from '@/utils/names'

const props = defineProps<{
  userId: number
  nodes: NodeBrief[]
  servers: Server[]
}>()

const items = ref<ConnectionLog[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const fromInput = ref('')
const toInput = ref('')

const nodeNames = computed(() => buildNameMap(props.nodes))
const serverNames = computed(() => buildNameMap(props.servers))

const columns: Column[] = [
  { key: 'connected_at', label: '连接时间', width: '160px' },
  { key: 'node', label: '节点' },
  { key: 'server', label: '服务器' },
  { key: 'ip', label: '来源 IP' },
  { key: 'protocol', label: '协议', width: '100px' },
  { key: 'upload', label: '上行', align: 'right', width: '90px' },
  { key: 'download', label: '下行', align: 'right', width: '90px' },
  { key: 'closed_at', label: '结束时间', width: '160px' },
  { key: 'status', label: '状态', width: '80px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result: Paged<ConnectionLog> = await getUserConnectionLogs(props.userId, {
      from: localInputToIso(fromInput.value),
      to: localInputToIso(toInput.value),
      page: page.value,
      pageSize: pageSize.value,
    })
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

function applyFilters() {
  page.value = 1
  void load()
}

function resetFilters() {
  fromInput.value = ''
  toInput.value = ''
  applyFilters()
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  void load()
}

watch(
  () => props.userId,
  () => {
    page.value = 1
    void load()
  },
)

onMounted(() => {
  void load()
})
</script>

<template>
  <div class="card">
    <h2 class="card-title">
      连接日志
    </h2>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="filters">
      <label>从</label>
      <input
        v-model="fromInput"
        type="datetime-local"
        @change="applyFilters"
      >
      <label>至</label>
      <input
        v-model="toInput"
        type="datetime-local"
        @change="applyFilters"
      >
      <button
        type="button"
        class="btn secondary"
        @click="resetFilters"
      >
        重置
      </button>
    </div>
    <DataTable
      :columns="columns"
      :rows="items"
      :row-key="(row) => row.id"
      :loading="loading"
    >
      <template #cell-connected_at="{ row }">
        {{ formatDateTime(row.connected_at) }}
      </template>
      <template #cell-node="{ row }">
        {{ lookupName(nodeNames, row.node_id) }}
      </template>
      <template #cell-server="{ row }">
        {{ lookupName(serverNames, row.server_id) }}
      </template>
      <template #cell-ip="{ row }">
        <span class="mono">{{ row.ip }}</span>
      </template>
      <template #cell-protocol="{ row }">
        {{ protocolLabel(row.protocol) }}
      </template>
      <template #cell-upload="{ row }">
        {{ formatBytes(row.upload_bytes) }}
      </template>
      <template #cell-download="{ row }">
        {{ formatBytes(row.download_bytes) }}
      </template>
      <template #cell-closed_at="{ row }">
        {{ row.closed_at ? formatDateTime(row.closed_at) : '—' }}
      </template>
      <template #cell-status="{ row }">
        <StatusBadge v-bind="logStatusInfo(row.status)" />
      </template>
      <template #empty>
        所选时间范围内暂无连接日志
      </template>
    </DataTable>
    <TablePaginator
      :page="page"
      :page-size="pageSize"
      :total="total"
      @change="onPageChange"
    />
  </div>
</template>
