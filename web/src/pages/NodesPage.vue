<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Copy, Pencil, Plus, QrCode, Trash2, Users, X } from 'lucide-vue-next'
import { deleteNode, copyNode, listNodes, updateNode } from '@/api/nodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { NodeBrief, NodeStatus, Paged, Protocol, Server } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import NodeFormDialog from '@/components/NodeFormDialog.vue'
import NodeShareDialog from '@/components/NodeShareDialog.vue'
import NodeUserAuthDialog from '@/components/NodeUserAuthDialog.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import FilterChip from '@/components/ui/FilterChip.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import SearchInput from '@/components/ui/SearchInput.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import { protocolLabel, type Tone } from '@/utils/labels'

const route = useRoute()
const router = useRouter()

const items = ref<NodeBrief[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const loading = ref(false)
const error = ref('')

const servers = ref<Server[]>([])
const query = ref('')
const serverFilter = ref('')
const protocolFilter = ref<Protocol | ''>('')
const statusFilter = ref<NodeStatus | ''>('')

const statusUpdatingId = ref<number | null>(null)

const sortKey = ref('')
const sortDir = ref<'asc' | 'desc'>('asc')

const showCreate = ref(false)
const editTarget = ref<NodeBrief | null>(null)
const deleteTarget = ref<NodeBrief | null>(null)
const shareTarget = ref<NodeBrief | null>(null)
const authTarget = ref<NodeBrief | null>(null)
const deleting = ref(false)

const columns: Column[] = [
  { key: 'name', label: '节点名称', width: '24%', sortable: true },
  { key: 'server', label: '所属服务器', width: '22%' },
  { key: 'protocol', label: '协议', width: '15%' },
  { key: 'port', label: '端口', width: '9%', sortable: true },
  { key: 'sni', label: '伪装目标 / SNI', width: '16%' },
  { key: 'enabled', label: '状态', width: '8%' },
  { key: 'actions', label: '操作', width: '150px', align: 'right' },
]

const protocolOptions: Array<{ value: Protocol | ''; label: string }> = [
  { value: '', label: '全部协议' },
  { value: 'shadowsocks', label: protocolLabel('shadowsocks') },
  { value: 'vless', label: protocolLabel('vless') },
  { value: 'hysteria2', label: protocolLabel('hysteria2') },
  { value: 'anytls', label: protocolLabel('anytls') },
  { value: 'socks', label: protocolLabel('socks') },
  { value: 'http', label: protocolLabel('http') },
]

const statusOptions: Array<{ value: NodeStatus | ''; label: string }> = [
  { value: '', label: '全部状态' },
  { value: 'active', label: '启用' },
  { value: 'disabled', label: '停用' },
]

// Node availability is computed by the backend: the server_status field folds
// in the owning server's heartbeat. A node is unavailable when it is disabled
// or its server is offline (red); a server disabled by an admin is grey.
function nodeDotTone(node: NodeBrief): Tone {
  if (node.status !== 'active') return 'danger'
  switch (node.server_status) {
    case 'offline':
      return 'danger'
    case 'disabled':
      return 'muted'
    default:
      return 'success'
  }
}

function nodeProtocolLabel(protocol: Protocol): string {
  if (protocol === 'vless') return 'VLESS + Reality'
  if (protocol === 'hysteria2') return 'Hysteria 2'
  return protocolLabel(protocol)
}

function addressFamily(address: string): string {
  if (address.includes(':')) return 'IPv6'
  if (/^[0-9]{1,3}(\.[0-9]{1,3}){3}$/.test(address)) return 'IPv4'
  return ''
}

function serverAddressFamilies(node: NodeBrief): string[] {
  const addresses = [node.server.ip, node.server.ipv6, node.server.observed_ip, node.server.observed_ipv6]
  const primaryFamily = addressFamily(node.address)
  const families: string[] = []
  if (addresses.some((address) => address !== '' && !address.includes(':')) || primaryFamily === 'IPv4') families.push('IPv4')
  if (addresses.some((address) => address.includes(':')) || primaryFamily === 'IPv6' || (node.ipv6_enabled && node.ipv6_address)) families.push('IPv6')
  return families
}

const serverChipLabel = computed(() => {
  if (!serverFilter.value) return '全部服务器'
  const hit = servers.value.find((server) => String(server.id) === serverFilter.value)
  return hit ? hit.name : '全部服务器'
})

const protocolChipLabel = computed(
  () => protocolOptions.find((option) => option.value === protocolFilter.value)?.label ?? '全部协议',
)

const statusChipLabel = computed(
  () => statusOptions.find((option) => option.value === statusFilter.value)?.label ?? '全部状态',
)

const sortedItems = computed(() => {
  if (!sortKey.value) return items.value
  const dir = sortDir.value === 'asc' ? 1 : -1
  return [...items.value].sort((a, b) => {
    if (sortKey.value === 'name') return a.name.localeCompare(b.name) * dir
    if (sortKey.value === 'port') return (a.port - b.port) * dir
    if (sortKey.value === 'created_at') return a.created_at.localeCompare(b.created_at) * dir
    return 0
  })
})

function onSort(key: string) {
  if (sortKey.value === key) {
    if (sortDir.value === 'asc') sortDir.value = 'desc'
    else {
      sortKey.value = ''
      sortDir.value = 'asc'
    }
  } else {
    sortKey.value = key
    sortDir.value = 'asc'
  }
}


async function copyRow(node: NodeBrief) {
  error.value = ''
  try {
    const copy = await copyNode(node.id)
    await load()
    // The copy reuses the source name/port and starts disabled; open the edit
    // form so the operator can adjust the port before enabling it.
    editTarget.value = copy
  } catch (err) {
    error.value = errorMessage(err)
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const serverId = Number(serverFilter.value)
    const result: Paged<NodeBrief> = await listNodes({
      serverId: Number.isInteger(serverId) && serverId > 0 ? serverId : undefined,
      protocol: protocolFilter.value || undefined,
      status: statusFilter.value || undefined,
      q: query.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
    })
    items.value = result.items
    total.value = result.total
    if (result.items.length === 0 && result.total > 0 && page.value > 1) {
      page.value = Math.ceil(result.total / pageSize.value)
      syncQuery()
      void load()
    }
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

function syncQuery() {
  const next: Record<string, string> = {}
  if (serverFilter.value) next['server_id'] = serverFilter.value
  if (protocolFilter.value) next['protocol'] = protocolFilter.value
  if (statusFilter.value) next['status'] = statusFilter.value
  if (query.value.trim()) next['q'] = query.value.trim()
  if (page.value > 1) next['page'] = String(page.value)
  void router.replace({ query: next })
}

function applyFilters() {
  page.value = 1
  syncQuery()
  void load()
}

function resetFilters() {
  query.value = ''
  serverFilter.value = ''
  protocolFilter.value = ''
  statusFilter.value = ''
  applyFilters()
}

function onPageChange(nextPage: number, nextSize: number) {
  page.value = nextPage
  pageSize.value = nextSize
  syncQuery()
  void load()
}

async function toggleStatus(node: NodeBrief) {
  const next: NodeStatus = node.status === 'active' ? 'disabled' : 'active'
  statusUpdatingId.value = node.id
  error.value = ''
  try {
    await updateNode(node.id, { status: next })
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
    await deleteNode(target.id)
    deleteTarget.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    deleting.value = false
  }
}

async function loadServers() {
  try {
    const result = await listServers({ page: 1, pageSize: 100 })
    servers.value = result.items
  } catch (err) {
    error.value = errorMessage(err)
  }
}

function restoreFromQuery() {
  const raw = route.query
  const serverId = typeof raw['server_id'] === 'string' ? Number(raw['server_id']) : NaN
  if (Number.isInteger(serverId) && serverId > 0) serverFilter.value = String(serverId)
  const protocol = raw['protocol']
  if (
    protocol === 'shadowsocks' ||
    protocol === 'vless' ||
    protocol === 'hysteria2' ||
    protocol === 'anytls' ||
    protocol === 'socks' ||
    protocol === 'http'
  ) {
    protocolFilter.value = protocol
  }
  const status = raw['status']
  if (status === 'active' || status === 'disabled') statusFilter.value = status
  if (typeof raw['q'] === 'string') {
    query.value = raw['q']
  }
  const rawPage = typeof raw['page'] === 'string' ? Number(raw['page']) : NaN
  if (Number.isInteger(rawPage) && rawPage > 0) page.value = rawPage
}

onMounted(() => {
  restoreFromQuery()
  void load()
  void loadServers()
})
</script>

<template>
  <section class="page">
    <PageHeader
      title="节点"
      :subtitle="`共 ${total} 个节点`"
    >
      <template #actions>
        <button
          type="button"
          class="btn"
          @click="showCreate = true"
        >
          <Plus :size="15" />
          新建节点
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
        placeholder="按名称搜索"
        @search="applyFilters"
      />
      <FilterChip
        :label="serverChipLabel"
        :active="serverFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              type="button"
              class="menu-list-item"
              :class="{ selected: serverFilter === '' }"
              @click="serverFilter = ''; applyFilters(); close()"
            >
              全部服务器
            </button>
            <button
              v-for="server in servers"
              :key="server.id"
              type="button"
              class="menu-list-item"
              :class="{ selected: serverFilter === String(server.id) }"
              @click="serverFilter = String(server.id); applyFilters(); close()"
            >
              {{ server.name }}
            </button>
          </div>
        </template>
      </FilterChip>
      <FilterChip
        :label="protocolChipLabel"
        :active="protocolFilter !== ''"
      >
        <template #default="{ close }">
          <div class="menu-list">
            <button
              v-for="option in protocolOptions"
              :key="option.value"
              type="button"
              class="menu-list-item"
              :class="{ selected: protocolFilter === option.value }"
              @click="protocolFilter = option.value; applyFilters(); close()"
            >
              {{ option.label }}
            </button>
          </div>
        </template>
      </FilterChip>
      <FilterChip
        :label="statusChipLabel"
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
      <button
        v-if="serverFilter || protocolFilter || statusFilter || query"
        type="button"
        class="btn ghost small"
        @click="resetFilters"
      >
        <X :size="13" />
        重置
      </button>
    </div>

    <div class="table-card">
      <DataTable
        :columns="columns"
        :rows="sortedItems"
        :row-key="(row) => row.id"
        :loading="loading"
        :sort-key="sortKey"
        :sort-dir="sortDir"
        :total-count="total"
        aria-label="节点列表"
        @sort="onSort"
      >
        <template #cell-name="{ row }">
          <strong
            class="node-name"
            :title="row.name"
          >{{ row.name }}</strong>
        </template>
        <template #cell-server="{ row }">
          <div class="server-cell">
            <div class="server-line">
              <i
                class="status-dot"
                :class="nodeDotTone(row)"
              />
              <RouterLink
                :to="`/servers/${row.server.id}`"
                class="server-link"
              >
                {{ row.server.name }}
              </RouterLink>
              <span
                v-for="family in serverAddressFamilies(row)"
                :key="family"
                class="address-family"
              >{{ family }}</span>
            </div>
            <span
              class="server-address mono"
              :title="row.address"
            >{{ row.address }}</span>
          </div>
        </template>
        <template #cell-protocol="{ row }">
          <div class="protocol-cell">
            <span class="protocol-badge">{{ nodeProtocolLabel(row.protocol) }}</span>
            <span
              v-if="row.rate !== 1"
              class="rate-label"
            >{{ row.rate }}×</span>
          </div>
        </template>
        <template #cell-port="{ row }">
          <span class="mono">{{ row.port }}</span>
        </template>
        <template #cell-sni="{ row }">
          <span
            class="sni-value mono"
            :title="row.sni || undefined"
          >{{ row.sni || '—' }}</span>
        </template>
        <template #cell-enabled="{ row }">
          <ToggleSwitch
            :model-value="row.status === 'active'"
            :disabled="statusUpdatingId === row.id"
            :label="`${row.status === 'active' ? '禁用' : '启用'}节点 ${row.name}`"
            @update:model-value="toggleStatus(row)"
          />
        </template>
        <template #cell-actions="{ row }">
          <div class="row-actions">
            <button
              type="button"
              class="icon-action"
              :aria-label="`分享节点 ${row.name}`"
              title="分享 / 二维码"
              @click="shareTarget = row"
            >
              <QrCode :size="17" />
            </button>
            <button
              type="button"
              class="icon-action"
              :aria-label="`复制节点 ${row.name}`"
              title="复制"
              @click="copyRow(row)"
            >
              <Copy :size="17" />
            </button>
            <button
              type="button"
              class="icon-action"
              :aria-label="`管理节点 ${row.name} 的用户授权`"
              title="用户授权"
              @click="authTarget = row"
            >
              <Users :size="17" />
            </button>
            <button
              type="button"
              class="icon-action"
              :aria-label="`编辑节点 ${row.name}`"
              title="编辑"
              @click="editTarget = row"
            >
              <Pencil :size="17" />
            </button>
            <button
              type="button"
              class="icon-action danger"
              :aria-label="`删除节点 ${row.name}`"
              title="删除"
              @click="deleteTarget = row"
            >
              <Trash2 :size="17" />
            </button>
          </div>
        </template>
        <template #empty>
          没有符合条件的节点
        </template>
      </DataTable>

      <TablePaginator
        :page="page"
        :page-size="pageSize"
        :total="total"
        @change="onPageChange"
      />
    </div>

    <NodeFormDialog
      :open="showCreate"
      @close="showCreate = false"
      @saved="load"
    />
    <NodeFormDialog
      :open="editTarget !== null"
      :node="editTarget"
      @close="editTarget = null"
      @saved="load"
    />

    <NodeShareDialog
      :open="shareTarget !== null"
      :node="shareTarget"
      @close="shareTarget = null"
    />

    <NodeUserAuthDialog
      :open="authTarget !== null"
      :node="authTarget"
      @close="authTarget = null"
    />

    <ConfirmDialog
      :open="deleteTarget !== null"
      title="删除节点"
      :message="`确定删除节点「${deleteTarget?.name ?? ''}」吗？\n将移除该节点的用户授权，Agent 下次同步后停止该服务。`"
      danger
      confirm-text="删除"
      :loading="deleting"
      @cancel="deleteTarget = null"
      @confirm="confirmDelete"
    />
  </section>
</template>

<style scoped>
.table-card { min-width: 0; }
.node-name { display: block; overflow: hidden; font-weight: 650; text-overflow: ellipsis; white-space: nowrap; }
.server-cell { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.server-line { display: flex; min-width: 0; align-items: center; gap: 8px; }
.server-link { overflow: hidden; color: var(--color-text); font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.address-family { flex: none; padding: 1px 7px; border: 1px solid var(--color-success); border-radius: var(--radius-full); color: var(--color-success); font-size: var(--font-size-xs); line-height: 1.35; }
.server-address { overflow: hidden; padding-left: 16px; color: var(--color-text-secondary); text-overflow: ellipsis; white-space: nowrap; }
.protocol-cell { display: flex; align-items: center; gap: 8px; }
.protocol-badge { display: inline-flex; align-items: center; padding: 4px 13px; border-radius: var(--radius-full); background: var(--color-badge-purple-solid); color: #fff; font-size: var(--font-size-sm); font-weight: 650; line-height: 1; white-space: nowrap; }
.rate-label { color: var(--color-text-secondary); font-size: var(--font-size-xs); white-space: nowrap; }
.sni-value { display: block; overflow: hidden; color: var(--color-text-secondary); text-overflow: ellipsis; white-space: nowrap; }
.row-actions { display: flex; justify-content: flex-end; gap: 2px; }
.icon-action { display: inline-grid; place-items: center; width: 28px; height: 28px; border: 0; border-radius: var(--radius-sm); background: transparent; color: var(--color-text-secondary); cursor: pointer; }
.icon-action:hover { background: var(--color-surface-muted); color: var(--color-text); }
.icon-action.danger { color: var(--color-danger); }
.icon-action:focus-visible { outline: 2px solid var(--color-focus-ring); }
:deep(.data-table) { min-width: 980px; }
:deep(.data-table th) { height: 40px; padding: 0 12px; color: var(--color-text); font-size: var(--font-size-sm); }
:deep(.data-table td) { height: 62px; padding: 8px 12px; }
</style>
