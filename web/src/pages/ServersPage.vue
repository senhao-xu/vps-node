<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { deleteServer, listServers, updateServer } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { Paged, Server, ServerStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import ServerFormDialog from '@/components/ServerFormDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { formatDateTime, formatRelative } from '@/utils/format'
import { serverStatusInfo } from '@/utils/labels'

const items = ref<Server[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const showCreate = ref(false)
const editTarget = ref<Server | null>(null)
const deleteTarget = ref<Server | null>(null)
const deleting = ref(false)
const statusUpdatingId = ref<number | null>(null)

const columns: Column[] = [
  { key: 'name', label: '名称', width: '150px' },
  { key: 'address', label: '地址' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'agent_version', label: 'Agent 版本', width: '100px' },
  { key: 'last_seen_at', label: '最后心跳', width: '130px' },
  { key: 'node_count', label: '节点数', align: 'right', width: '70px' },
  { key: 'online_users', label: '在线用户', align: 'right', width: '80px' },
  { key: 'actions', label: '操作', width: '150px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result: Paged<Server> = await listServers({ page: page.value, pageSize: pageSize.value })
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

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  void load()
}

async function toggleStatus(server: Server) {
  const next: ServerStatus = server.status === 'disabled' ? 'active' : 'disabled'
  statusUpdatingId.value = server.id
  error.value = ''
  try {
    await updateServer(server.id, { status: next })
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    statusUpdatingId.value = null
  }
}

async function confirmDelete() {
  const target = deleteTarget.value
  if (!target) return
  deleting.value = true
  error.value = ''
  try {
    await deleteServer(target.id)
    deleteTarget.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    deleting.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <section class="page">
    <div class="page-header">
      <div>
        <p class="eyebrow">
          INFRASTRUCTURE
        </p>
        <h1 class="page-title">
          服务器
        </h1>
      </div>
      <button
        type="button"
        class="btn"
        @click="showCreate = true"
      >
        创建服务器
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="card">
      <div class="toolbar-label">
        服务器目录 <span>{{ total }} 台服务器</span>
      </div>
      <DataTable
        :columns="columns"
        :rows="items"
        :loading="loading"
      >
        <template #cell-name="{ row }">
          <RouterLink :to="`/servers/${row.id}`">
            {{ row.name }}
          </RouterLink>
        </template>
        <template #cell-address="{ row }">
          <span class="mono">{{ row.address }}</span>
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="serverStatusInfo(row.status)" />
        </template>
        <template #cell-agent_version="{ row }">
          {{ row.agent_version || '—' }}
        </template>
        <template #cell-last_seen_at="{ row }">
          <span v-if="row.last_seen_at">
            {{ formatRelative(row.last_seen_at) }}
            <span class="text-secondary heartbeat">{{ formatDateTime(row.last_seen_at) }}</span>
          </span>
          <span
            v-else
            class="text-secondary"
          >从未</span>
        </template>
        <template #cell-actions="{ row }">
          <span class="actions">
            <RouterLink :to="`/servers/${row.id}`">详情</RouterLink>
            <button
              type="button"
              class="btn link"
              @click="editTarget = row"
            >编辑</button>
            <button
              type="button"
              class="btn link"
              :disabled="statusUpdatingId === row.id"
              @click="toggleStatus(row)"
            >
              {{ row.status === 'disabled' ? '启用' : '禁用' }}
            </button>
            <button
              type="button"
              class="btn link"
              @click="deleteTarget = row"
            >删除</button>
          </span>
        </template>
        <template #empty>
          {{ loading ? '加载中…' : '还没有服务器，点击右上角创建' }}
        </template>
      </DataTable>

      <TablePaginator
        :page="page"
        :page-size="pageSize"
        :total="total"
        @change="onPageChange"
      />
    </div>

    <ServerFormDialog
      :open="showCreate"
      @close="showCreate = false"
      @saved="load"
    />
    <ServerFormDialog
      :open="editTarget !== null"
      :server="editTarget"
      @close="editTarget = null"
      @saved="load"
    />

    <ConfirmDialog
      :open="deleteTarget !== null"
      title="删除服务器"
      :message="`确定删除服务器「${deleteTarget?.name ?? ''}」吗？\n将同时删除其 Agent、节点、节点授权和当前连接；历史流量与连接日志按保留策略清理。`"
      danger
      confirm-text="删除"
      :loading="deleting"
      @cancel="deleteTarget = null"
      @confirm="confirmDelete"
    />
  </section>
</template>

<style scoped>
.eyebrow {
  margin: 0 0 var(--spacing-xs);
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.toolbar-label {
  display: flex;
  justify-content: space-between;
  margin-bottom: var(--spacing-md);
  color: var(--color-text);
  font-weight: 600;
}

.toolbar-label span {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 400;
}

.heartbeat {
  display: block;
  font-size: var(--font-size-sm);
}

.actions {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
}
</style>
