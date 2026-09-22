<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, X } from 'lucide-vue-next'
import { deleteUser, listUsers, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { Paged, User, UserExpiryFilter, UserStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import UserCreateDialog from '@/components/UserCreateDialog.vue'
import FilterChip from '@/components/ui/FilterChip.vue'
import OverflowMenu, { type OverflowMenuItem } from '@/components/ui/OverflowMenu.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import { formatBytes, formatDate, formatDateTime, formatRemaining } from '@/utils/format'
import { displayUserStatus } from '@/utils/labels'

const router = useRouter()

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
  { key: 'id', label: 'ID', width: '80px' },
  { key: 'username', label: '用户名', width: '150px' },
  { key: 'traffic', label: '流量', width: '200px' },
  { key: 'expires_at', label: '到期时间', width: '150px' },
  { key: 'node_count', label: '节点数', align: 'right', width: '70px' },
  { key: 'online_count', label: '在线设备', align: 'right', width: '80px' },
  { key: 'created_at', label: '创建时间', width: '150px' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'actions', label: '', width: '48px' },
]

const statusOptions: Array<{ value: UserStatus | ''; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '正常' },
  { value: 'disabled', label: '已禁用' },
  { value: 'expired', label: '已过期' },
]

const expiryOptions: Array<{ value: UserExpiryFilter | ''; label: string }> = [
  { value: '', label: '全部到期状态' },
  { value: 'valid', label: '未到期' },
  { value: 'expired', label: '已到期' },
]

function rowActions(row: User): OverflowMenuItem[] {
  return [
    { label: '详情', onSelect: () => void router.push(`/users/${row.id}`) },
    {
      label: row.status === 'active' ? '禁用' : '启用',
      onSelect: () => void toggleStatus(row),
    },
    { label: '删除', danger: true, onSelect: () => (deleteTarget.value = row) },
  ]
}

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
  if (user.transfer_enable <= 0) return 0
  return Math.min(100, (user.used_bytes / user.transfer_enable) * 100)
}

function trafficText(user: User): string {
  if (user.transfer_enable <= 0) return `${formatBytes(user.used_bytes)} / 不限`
  return `${formatBytes(user.used_bytes)} / ${formatBytes(user.transfer_enable)}`
}

function isExpired(user: User): boolean {
  if (!user.expires_at) return false
  const time = new Date(user.expires_at).getTime()
  return !Number.isNaN(time) && time <= Date.now()
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
    <PageHeader
      title="用户"
      :subtitle="`共 ${total} 个用户`"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          @click="showCreate = true"
        >
          <Plus :size="15" />
          创建用户
        </button>
      </template>
    </PageHeader>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="table-toolbar">
      <SearchInput
        v-model="query"
        placeholder="按 用户名 / UUID / Token 搜索"
        @search="applyFilters"
      />
      <FilterChip
        :label="statusOptions.find((o) => o.value === statusFilter)?.label ?? '全部状态'"
        :active="statusFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              v-for="option in statusOptions"
              :key="option.value"
              type="button"
              class="menu-list-item"
              :class="{ selected: statusFilter === option.value }"
              @click="statusFilter = option.value; applyFilters(); close()"
            >
              {{ option.label }}
            </button>
          </div>
        </template>
      </FilterChip>
      <FilterChip
        :label="expiryOptions.find((o) => o.value === expiryFilter)?.label ?? '全部到期状态'"
        :active="expiryFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              v-for="option in expiryOptions"
              :key="option.value"
              type="button"
              class="menu-list-item"
              :class="{ selected: expiryFilter === option.value }"
              @click="expiryFilter = option.value; applyFilters(); close()"
            >
              {{ option.label }}
            </button>
          </div>
        </template>
      </FilterChip>
      <button
        v-if="query || statusFilter || expiryFilter"
        type="button"
        class="btn ghost small"
        @click="resetFilters"
      >
        <X :size="13" />
        重置
      </button>
    </div>

    <DataTable
      :columns="columns"
      :rows="items"
      :row-key="(row) => row.id"
      :loading="loading"
      :total-count="total"
    >
      <template #cell-id="{ row }">
        <span class="chip mono">#{{ row.id }}</span>
      </template>
      <template #cell-username="{ row }">
        <RouterLink :to="`/users/${row.id}`">
          {{ row.username }}
        </RouterLink>
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
        <StatusBadge
          v-if="!row.expires_at"
          label="长期有效"
          tone="muted"
        />
        <StatusBadge
          v-else-if="isExpired(row)"
          :label="`已过期 · ${formatDate(row.expires_at)}`"
          tone="danger"
        />
        <div
          v-else
          class="expire-cell"
        >
          <span>{{ formatDate(row.expires_at) }}</span>
          <span class="text-secondary remaining">{{ formatRemaining(row.expires_at) }}</span>
        </div>
      </template>
      <template #cell-created_at="{ row }">
        {{ formatDateTime(row.created_at) }}
      </template>
      <template #cell-actions="{ row }">
        <OverflowMenu
          :items="rowActions(row)"
          :label="`用户 ${row.username} 的操作`"
        />
      </template>
      <template #empty>
        没有符合条件的用户
      </template>
    </DataTable>

    <TablePaginator
      :page="page"
      :page-size="pageSize"
      :total="total"
      @change="onPageChange"
    />

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


</style>
