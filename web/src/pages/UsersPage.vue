<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { deleteUser, listUsers, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { Paged, User, UserExpiryFilter, UserStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import CopyText from '@/components/CopyText.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import UserCreateDialog from '@/components/UserCreateDialog.vue'
import { formatBytes, formatDate, formatDateTime, formatRemaining } from '@/utils/format'
import { displayUserStatus } from '@/utils/labels'

const items = ref<User[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const query = ref('')
const statusFilter = ref<UserStatus | ''>('')
const expiryFilter = ref<UserExpiryFilter | ''>('')
const showCreate = ref(false)

const deleteTarget = ref<User | null>(null)
const deleting = ref(false)
const statusUpdatingId = ref<number | null>(null)

const columns: Column[] = [
  { key: 'id', label: 'ID', width: '70px' },
  { key: 'username', label: '用户名', width: '150px' },
  { key: 'uuid', label: 'UUID' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'traffic', label: '流量', width: '200px' },
  { key: 'expires_at', label: '到期时间', width: '170px' },
  { key: 'node_count', label: '节点数', align: 'right', width: '70px' },
  { key: 'session_count', label: '在线', align: 'right', width: '60px' },
  { key: 'created_at', label: '创建时间', width: '160px' },
  { key: 'actions', label: '操作', width: '180px' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result: Paged<User> = await listUsers({
      query: query.value.trim() || undefined,
      status: statusFilter.value || undefined,
      expiry: expiryFilter.value || undefined,
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
  query.value = ''
  statusFilter.value = ''
  expiryFilter.value = ''
  applyFilters()
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  void load()
}

function trafficPercent(user: User): number {
  if (user.quota_bytes <= 0) return 0
  return Math.min(100, (user.used_bytes / user.quota_bytes) * 100)
}

function trafficText(user: User): string {
  if (user.quota_bytes <= 0) return `${formatBytes(user.used_bytes)} / 不限`
  return `${formatBytes(user.used_bytes)} / ${formatBytes(user.quota_bytes)}`
}

async function toggleStatus(user: User) {
  const next: UserStatus = user.status === 'active' ? 'disabled' : 'active'
  statusUpdatingId.value = user.id
  error.value = ''
  try {
    await updateUser(user.id, { status: next })
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
    await deleteUser(target.id)
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
          ACCESS MANAGEMENT
        </p>
        <h1 class="page-title">
          用户
        </h1>
      </div>
      <button
        type="button"
        class="btn"
        @click="showCreate = true"
      >
        创建用户
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="card">
      <div class="toolbar-label">
        用户目录 <span>{{ total }} 个用户</span>
      </div>
      <div class="filters">
        <input
          v-model="query"
          class="search-input"
          type="text"
          placeholder="按 用户名 / UUID / Token 精确搜索"
          @keyup.enter="applyFilters"
        >
        <select
          v-model="statusFilter"
          @change="applyFilters"
        >
          <option value="">
            全部状态
          </option>
          <option value="active">
            正常
          </option>
          <option value="disabled">
            已禁用
          </option>
          <option value="expired">
            已过期
          </option>
        </select>
        <select
          v-model="expiryFilter"
          @change="applyFilters"
        >
          <option value="">
            全部到期状态
          </option>
          <option value="valid">
            未到期
          </option>
          <option value="expired">
            已到期
          </option>
        </select>
        <button
          type="button"
          class="btn"
          @click="applyFilters"
        >
          搜索
        </button>
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
        :loading="loading"
      >
        <template #cell-uuid="{ row }">
          <CopyText
            class="mono"
            :text="row.uuid"
          />
        </template>
        <template #cell-status="{ row }">
          <StatusBadge v-bind="displayUserStatus(row)" />
        </template>
        <template #cell-traffic="{ row }">
          <div class="traffic-cell">
            <span class="traffic-text">{{ trafficText(row) }}</span>
            <ProgressBar
              :percent="trafficPercent(row)"
              compact
            />
          </div>
        </template>
        <template #cell-expires_at="{ row }">
          <div class="expire-cell">
            <span>{{ row.expires_at ? formatDate(row.expires_at) : '永不过期' }}</span>
            <span class="text-secondary remaining">{{ formatRemaining(row.expires_at) }}</span>
          </div>
        </template>
        <template #cell-created_at="{ row }">
          {{ formatDateTime(row.created_at) }}
        </template>
        <template #cell-actions="{ row }">
          <span class="actions">
            <RouterLink :to="`/users/${row.id}`">详情</RouterLink>
            <button
              type="button"
              class="btn link"
              :disabled="statusUpdatingId === row.id"
              @click="toggleStatus(row)"
            >
              {{ row.status === 'active' ? '禁用' : '启用' }}
            </button>
            <button
              type="button"
              class="btn link"
              @click="deleteTarget = row"
            >删除</button>
          </span>
        </template>
        <template #empty>
          {{ loading ? '加载中…' : '没有符合条件的用户' }}
        </template>
      </DataTable>

      <TablePaginator
        :page="page"
        :page-size="pageSize"
        :total="total"
        @change="onPageChange"
      />
    </div>

    <UserCreateDialog
      :open="showCreate"
      @close="showCreate = false"
      @created="load"
    />

    <ConfirmDialog
      :open="deleteTarget !== null"
      title="删除用户"
      :message="`确定删除用户 #${deleteTarget?.id ?? ''} 吗？\n将同时移除其节点授权和当前连接，Agent 下一次同步后删除本地配置。`"
      danger
      confirm-text="删除"
      :loading="deleting"
      @cancel="deleteTarget = null"
      @confirm="confirmDelete"
    />
  </section>
</template>

<style scoped>
.traffic-cell {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 150px;
}

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

.traffic-text {
  font-size: var(--font-size-sm);
}

.expire-cell {
  display: flex;
  flex-direction: column;
}

.remaining {
  font-size: var(--font-size-sm);
}

.actions {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.search-input {
  width: min(300px, 100%);
}

@media (max-width: 700px) {
  .search-input {
    width: 100%;
  }
}
</style>
