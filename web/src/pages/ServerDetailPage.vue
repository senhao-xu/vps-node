<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import {
  Activity,
  Box,
  CalendarClock,
  Cpu,
  HardDrive,
  HeartPulse,
  MemoryStick,
  Pencil,
  Power,
  Radio,
  RefreshCw,
  Server as ServerIcon,
  ShieldCheck,
  Trash2,
  Users,
  Waypoints,
} from 'lucide-vue-next'
import { deleteServer, getServer, getAgentKey, generateAgentKey, updateServer } from '@/api/servers'
import { deleteNode } from '@/api/nodes'
import { getServerVisits } from '@/api/visits'
import { errorMessage } from '@/api/http'
import type { NodeBrief, ServerDetail, ServerStatus, Visit } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import TablePaginator from '@/components/TablePaginator.vue'
import MetricBar from '@/components/MetricBar.vue'
import NodeFormDialog from '@/components/NodeFormDialog.vue'
import ServerFormDialog from '@/components/ServerFormDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import DetailNav, { type DetailNavItem } from '@/components/ui/DetailNav.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import MetricStrip, { type MetricStripItem } from '@/components/ui/MetricStrip.vue'
import OverflowMenu, { type OverflowMenuItem } from '@/components/ui/OverflowMenu.vue'
import ResourceHeader from '@/components/ui/ResourceHeader.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import { useCopyFeedback } from '@/utils/clipboard'
import { formatDateTime, formatDuration, formatPercent, formatRelative } from '@/utils/format'
import { binaryInstallCommand, dockerInstallCommand } from '@/utils/installCommands'
import { nodeStatusInfo, protocolLabel, serverStatusInfo } from '@/utils/labels'

const route = useRoute()
const router = useRouter()

const server = ref<ServerDetail | null>(null)
const loading = ref(false)
const error = ref('')

const showEdit = ref(false)
const showDeleteConfirm = ref(false)
const deleting = ref(false)
const statusUpdating = ref(false)
const agentKey = ref('')
const agentKeyLoading = ref(false)
const agentKeyError = ref('')
const generatingAgentKey = ref(false)
const showResetKeyConfirm = ref(false)
const resettingAgentKey = ref(false)

type InstallTab = 'binary' | 'docker'

const installTab = ref<InstallTab>('binary')
const installTabItems: { value: InstallTab; label: string }[] = [
  { value: 'binary', label: '二进制安装' },
  { value: 'docker', label: 'Docker 安装' },
]

const detailNavItems: DetailNavItem[] = [
  { id: 'overview', label: '运行概览', hint: '身份与承载状态', icon: ServerIcon },
  { id: 'agent', label: 'Agent 管理', hint: '凭证与安装', icon: Radio },
  { id: 'health', label: '系统指标', hint: '资源压力与运行时间', icon: HeartPulse },
  { id: 'nodes', label: '节点拓扑', hint: '协议与服务端口', icon: Waypoints },
  { id: 'activity', label: '访问活动', hint: '最近连接目标', icon: Activity },
]
const { copied: commandCopied, copy } = useCopyFeedback()
const { copied: keyCopied, copy: copyKey } = useCopyFeedback()

const visits = ref<Visit[]>([])
const visitTotal = ref(0)
const visitPage = ref(1)
const visitPageSize = ref(20)
const visitsLoading = ref(false)
const visitsError = ref('')

const visitColumns: { key: string; label: string; width?: string }[] = [
  { key: 'created_at', label: '时间', width: '170px' },
  { key: 'username', label: '用户' },
  { key: 'node_name', label: '节点' },
  { key: 'target', label: '目标' },
  { key: 'client_ip', label: '来源 IP', width: '150px' },
]

function formatTarget(row: Visit): string {
  if (row.dest_port <= 0) return row.dest_host
  return row.dest_host.includes(':')
    ? `[${row.dest_host}]:${row.dest_port}`
    : `${row.dest_host}:${row.dest_port}`
}

async function loadVisits() {
  const id = serverId.value
  if (id === null) return
  visitsLoading.value = true
  visitsError.value = ''
  try {
    const result = await getServerVisits(id, { page: visitPage.value, pageSize: visitPageSize.value })
    visits.value = result.items
    visitTotal.value = result.total
  } catch (err) {
    visitsError.value = errorMessage(err)
  } finally {
    visitsLoading.value = false
  }
}

function onVisitPageChange(nextPage: number, nextSize: number) {
  visitPage.value = nextPage
  visitPageSize.value = nextSize
  void loadVisits()
}

const panelOrigin = window.location.origin
const installNotes: Record<InstallTab, string> = {
  binary: 'install 脚本目前随 release tarball 分发，脚本地址需按实际发布渠道替换。',
  docker: '使用 host 网络模式，节点端口直接生效无需映射；镜像从 GitHub Container Registry（ghcr.io）拉取。',
}

const activeInstallCommand = computed(() =>
  installTab.value === 'binary'
    ? binaryInstallCommand(panelOrigin, server.value?.id ?? 0, agentKey.value)
    : dockerInstallCommand(panelOrigin, server.value?.id ?? 0, agentKey.value),
)

async function copyInstallCommand() {
  await copy(activeInstallCommand.value)
}

const showNodeDialog = ref(false)
const editNode = ref<NodeBrief | null>(null)
const deleteNodeTarget = ref<NodeBrief | null>(null)
const deletingNode = ref(false)

const serverId = computed(() => {
  const raw = route.params.id
  const id = typeof raw === 'string' ? Number(raw) : NaN
  return Number.isInteger(id) && id > 0 ? id : null
})

const serverMetrics = computed<MetricStripItem[]>(() => {
  const current = server.value
  if (!current) return []
  return [
    {
      key: 'cpu',
      label: 'CPU',
      value: formatPercent(current.cpu_percent),
      icon: Cpu,
      tone: current.cpu_percent >= 80 ? 'warning' : 'default',
    },
    {
      key: 'memory',
      label: '内存',
      value: formatPercent(current.memory_percent),
      icon: MemoryStick,
      tone: current.memory_percent >= 80 ? 'warning' : 'default',
    },
    {
      key: 'disk',
      label: '磁盘',
      value: formatPercent(current.disk_percent),
      icon: HardDrive,
      tone: current.disk_percent >= 80 ? 'warning' : 'default',
    },
    {
      key: 'users',
      label: '在线用户',
      value: current.online_users,
      hint: current.nodes.length + ' 个节点',
      icon: Users,
    },
    {
      key: 'uptime',
      label: '运行时间',
      value: formatDuration(current.uptime_seconds),
      icon: Activity,
    },
  ]
})

const maxResourcePercent = computed(() => {
  const current = server.value
  if (!current) return 0
  return Math.max(current.cpu_percent, current.memory_percent, current.disk_percent)
})

const pressureLabel = computed(() => {
  if (maxResourcePercent.value >= 90) return '高压'
  if (maxResourcePercent.value >= 75) return '偏高'
  return '平稳'
})

const pressureClass = computed(() =>
  maxResourcePercent.value >= 90
    ? 'text-danger'
    : maxResourcePercent.value >= 75
      ? 'text-warning'
      : 'text-success',
)

const nodeColumns: Column[] = [
  { key: 'name', label: '节点名称' },
  { key: 'protocol', label: '协议', width: '120px' },
  { key: 'port', label: '端口', align: 'right', width: '80px' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'actions', label: '', width: '48px', divider: true },
]

function nodeActions(row: NodeBrief): OverflowMenuItem[] {
  return [
    { label: '编辑', onSelect: () => (editNode.value = row) },
    { label: '删除', danger: true, onSelect: () => (deleteNodeTarget.value = row) },
  ]
}

async function load() {
  const id = serverId.value
  if (id === null) {
    error.value = '无效的服务器 ID'
    return
  }
  loading.value = true
  error.value = ''
  try {
    server.value = await getServer(id)
  } catch (err) {
    error.value = errorMessage(err)
    if ((err as { status?: number }).status === 404) {
      void router.replace('/servers')
    }
  } finally {
    loading.value = false
  }
}

async function toggleStatus() {
  const current = server.value
  if (!current) return
  const next: ServerStatus = current.status === 'disabled' ? 'active' : 'disabled'
  statusUpdating.value = true
  error.value = ''
  try {
    await updateServer(current.id, { status: next })
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    statusUpdating.value = false
  }
}

async function loadAgentKey() {
  const current = server.value
  if (!current) return
  agentKeyLoading.value = true
  agentKeyError.value = ''
  try {
    agentKey.value = (await getAgentKey(current.id)).agent_key
  } catch (err) {
    agentKey.value = ''
    if ((err as { status?: number }).status !== 409) {
      agentKeyError.value = errorMessage(err)
    }
  } finally {
    agentKeyLoading.value = false
  }
}

async function generateAgentKeyNow() {
  const current = server.value
  if (!current) return
  generatingAgentKey.value = true
  agentKeyError.value = ''
  error.value = ''
  try {
    agentKey.value = (await generateAgentKey(current.id)).agent_key
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    generatingAgentKey.value = false
  }
}

async function confirmResetAgentKey() {
  const current = server.value
  if (!current) return
  resettingAgentKey.value = true
  agentKeyError.value = ''
  error.value = ''
  try {
    agentKey.value = (await generateAgentKey(current.id)).agent_key
    showResetKeyConfirm.value = false
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    resettingAgentKey.value = false
  }
}

async function copyAgentKey() {
  await copyKey(agentKey.value)
}

async function confirmDeleteServer() {
  const current = server.value
  if (!current) return
  deleting.value = true
  error.value = ''
  try {
    await deleteServer(current.id)
    await router.replace('/servers')
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    deleting.value = false
  }
}

async function confirmDeleteNode() {
  const target = deleteNodeTarget.value
  if (!target) return
  deletingNode.value = true
  error.value = ''
  try {
    await deleteNode(target.id)
    deleteNodeTarget.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    deletingNode.value = false
  }
}

function onSaved() {
  void load()
}

watch(serverId, () => {
  agentKey.value = ''
  visitPage.value = 1
  void load().then(() => {
    void loadAgentKey()
  })
  void loadVisits()
})

onMounted(() => {
  void load().then(() => {
    void loadAgentKey()
  })
  void loadVisits()
})
</script>

<template>
  <section class="page detail-page">
    <ResourceHeader
      :title="server ? server.name : '服务器详情'"
      eyebrow="服务器资源"
      :subtitle="server ? `由 Agent ${server.agent?.version || '未注册'} 管理的运行实例` : '正在读取服务器状态'"
      back-to="/servers"
      back-label="返回服务器列表"
      :icon="ServerIcon"
    >
      <template #status>
        <StatusBadge
          v-if="server"
          v-bind="serverStatusInfo(server.status)"
        />
      </template>
      <template #actions>
        <template v-if="server">
          <button
            type="button"
            class="btn secondary"
            :disabled="statusUpdating"
            @click="toggleStatus"
          >
            <Power :size="15" />
            {{ server.status === 'disabled' ? '启用' : '禁用' }}
          </button>
          <button
            type="button"
            class="btn secondary"
            @click="showEdit = true"
          >
            <Pencil :size="15" />
            编辑
          </button>
          <button
            type="button"
            class="btn danger secondary"
            @click="showDeleteConfirm = true"
          >
            <Trash2 :size="15" />
            删除
          </button>
        </template>
      </template>
      <template #meta>
        <span class="resource-meta-item">
          <Box :size="15" />
          Server ID <strong>#{{ server?.id ?? '—' }}</strong>
        </span>
        <span class="resource-meta-item">
          <RefreshCw :size="15" />
          配置版本 <strong>r{{ server?.revision ?? '—' }}</strong>
        </span>
        <span class="resource-meta-item">
          <CalendarClock :size="15" />
          创建于 <strong>{{ formatDateTime(server?.created_at) }}</strong>
        </span>
        <span class="resource-meta-item">
          <Radio :size="15" />
          最后心跳 <strong>{{ formatRelative(server?.last_seen_at) }}</strong>
        </span>
      </template>
    </ResourceHeader>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="loading && !server"
      class="loading-block"
    >
      <LoadingSpinner size="lg" />
    </div>
    <template v-else-if="server">
      <MetricStrip
        :items="serverMetrics"
        aria-label="服务器资源指标"
      />
      <DetailNav
        :items="detailNavItems"
        aria-label="服务器详情分区"
      />
      <div class="detail-workspace">
        <main class="detail-main">
          <section
            id="overview"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Overview</span>
                <h2>运行概览</h2>
                <p>核对服务器身份、配置修订和当前承载情况。</p>
              </div>
            </div>
            <div class="card">
              <h2 class="card-title">
                基础信息
              </h2>
              <div class="info-grid">
                <div class="info-item">
                  <span class="info-label">名称</span>
                  <span>{{ server.name }}</span>
                </div>
                <div class="info-item">
                  <span class="info-label">Server ID</span>
                  <span>{{ server.id }}</span>
                </div>
                <div class="info-item">
                  <span class="info-label">配置版本（revision）</span>
                  <span class="mono">{{ server.revision }}</span>
                </div>
                <div class="info-item">
                  <span class="info-label">在线用户</span>
                  <span>{{ server.online_users }}</span>
                </div>
                <div class="info-item">
                  <span class="info-label">创建时间</span>
                  <span>{{ formatDateTime(server.created_at) }}</span>
                </div>
              </div>
            </div>
          </section>

          <section
            id="agent"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Agent</span>
                <h2>Agent 管理</h2>
                <p>管理 Agent Key、安装方式和运行端连接状态。</p>
              </div>
            </div>
            <div class="card">
              <h2 class="card-title">
                Agent
              </h2>
              <div
                v-if="server.agent"
                class="info-grid"
              >
                <div class="info-item">
                  <span class="info-label">Agent ID</span>
                  <span>{{ server.agent.id }}</span>
                </div>
                <div class="info-item">
                  <span class="info-label">版本</span>
                  <span>{{ server.agent.version || '—' }}</span>
                </div>
                <div class="info-item">
                  <span class="info-label">最后心跳</span>
                  <span>
                    {{ server.agent.last_seen_at ? formatDateTime(server.agent.last_seen_at) : '从未' }}
                    <span
                      v-if="server.agent.last_seen_at"
                      class="text-secondary"
                    >
                      （{{ formatRelative(server.agent.last_seen_at) }}）
                    </span>
                  </span>
                </div>
                <div class="info-item">
                  <span class="info-label">连接时间</span>
                  <span>{{ formatDateTime(server.agent.connected_at) }}</span>
                </div>
              </div>
              <p
                v-else
                class="text-secondary agent-missing"
              >
                该服务器还没有 Agent 连接。生成 Agent Key 并在节点上配置后即可接入。
              </p>

              <div class="agent-key-block">
                <div class="install-head">
                  <span class="install-title">Agent Key</span>
                  <div class="token-actions">
                    <button
                      v-if="agentKey"
                      type="button"
                      class="btn secondary small"
                      :disabled="generatingAgentKey"
                      @click="showResetKeyConfirm = true"
                    >
                      <RefreshCw :size="14" />
                      重置
                    </button>
                    <button
                      v-else
                      type="button"
                      class="btn small"
                      :disabled="generatingAgentKey"
                      @click="generateAgentKeyNow"
                    >
                      {{ generatingAgentKey ? '生成中…' : '生成 Agent Key' }}
                    </button>
                  </div>
                </div>
                <ErrorBanner
                  :message="agentKeyError"
                  @dismiss="agentKeyError = ''"
                />
                <div
                  v-if="agentKey"
                  class="agent-key-value"
                >
                  <code class="mono">{{ agentKey }}</code>
                  <button
                    type="button"
                    class="btn link small"
                    @click="copyAgentKey"
                  >
                    {{ keyCopied ? '已复制' : '复制' }}
                  </button>
                </div>
                <p
                  v-else-if="agentKeyLoading"
                  class="install-hint"
                >
                  正在读取 Agent Key…
                </p>
                <p
                  v-else
                  class="install-hint"
                >
                  尚未生成 Agent Key。生成后请填入 Agent 的 <code class="mono">agent_key</code> 配置或 <code class="mono">AGENT_KEY</code> 环境变量；重置会立即使旧 Key 失效。
                </p>
              </div>

              <div class="install-block">
                <div class="install-head">
                  <span class="install-title">Agent 安装</span>
                  <SegmentedControl
                    v-model="installTab"
                    class="install-segmented"
                    :items="installTabItems"
                    aria-label="安装方式"
                  />
                </div>
                <p
                  v-if="agentKey === ''"
                  class="install-hint"
                >
                  {{ agentKeyLoading ? '正在读取 Agent Key…' : '生成 Agent Key 后，安装命令会自动内嵌该 Key。' }}
                </p>
                <p
                  v-else
                  class="install-hint ok"
                >
                  已内嵌 Agent Key；重置后请同步更新节点配置与命令。
                </p>
                <div class="install-code-head">
                  <span class="text-secondary">安装命令</span>
                  <button
                    type="button"
                    class="btn link small"
                    @click="copyInstallCommand"
                  >
                    {{ commandCopied ? '已复制' : '复制' }}
                  </button>
                </div>
                <pre class="install-code mono">{{ activeInstallCommand }}</pre>
                <p class="install-note">
                  {{ installNotes[installTab] }}
                </p>
              </div>
            </div>
          </section>

          <section
            id="health"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Health</span>
                <h2>系统指标</h2>
                <p>持续观察 CPU、内存、磁盘和运行时长。</p>
              </div>
            </div>
            <div class="card">
              <h2 class="card-title">
                系统指标
              </h2>
              <div class="metrics">
                <MetricBar
                  label="CPU"
                  :percent="server.cpu_percent"
                />
                <MetricBar
                  label="内存"
                  :percent="server.memory_percent"
                />
                <MetricBar
                  label="磁盘"
                  :percent="server.disk_percent"
                />
                <div class="uptime-row">
                  <span class="text-secondary">Uptime</span>
                  <span>{{ formatDuration(server.uptime_seconds) }}</span>
                </div>
              </div>
            </div>
          </section>

          <section
            id="nodes"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Topology</span>
                <h2>节点拓扑</h2>
                <p>查看并维护该服务器承载的协议、端口和节点状态。</p>
              </div>
            </div>
            <div class="card">
              <div class="card-head">
                <h2 class="card-title">
                  节点（{{ server.nodes.length }}）
                </h2>
                <div class="node-head-actions">
                  <RouterLink
                    class="btn secondary small"
                    :to="`/nodes?server_id=${server.id}`"
                  >
                    在节点页查看
                  </RouterLink>
                  <button
                    type="button"
                    class="btn small"
                    @click="showNodeDialog = true"
                  >
                    添加节点
                  </button>
                </div>
              </div>
              <DataTable
                :columns="nodeColumns"
                :rows="server.nodes"
                :row-key="(row) => row.id"
                :loading="loading"
                :bordered="false"
                aria-label="服务器节点列表"
              >
                <template #cell-protocol="{ row }">
                  {{ protocolLabel(row.protocol) }}
                </template>
                <template #cell-status="{ row }">
                  <StatusBadge v-bind="nodeStatusInfo(row.status)" />
                </template>
                <template #cell-actions="{ row }">
                  <OverflowMenu
                    :items="nodeActions(row)"
                    :label="`节点 ${row.name} 的操作`"
                  />
                </template>
                <template #empty>
                  该服务器还没有节点，点击右上角添加
                </template>
              </DataTable>
            </div>
          </section>

          <section
            id="activity"
            class="detail-section"
          >
            <div class="section-heading">
              <div class="section-heading-copy">
                <span class="section-kicker">Activity</span>
                <h2>访问活动</h2>
                <p>检查用户、节点、目标地址和来源 IP 的最近访问记录。</p>
              </div>
            </div>
            <div class="card">
              <div class="card-head">
                <h2 class="card-title">
                  访问站点
                </h2>
                <span class="text-secondary visit-count">共 {{ visitTotal }} 条</span>
              </div>
              <ErrorBanner
                :message="visitsError"
                @dismiss="visitsError = ''"
              />
              <DataTable
                :columns="visitColumns"
                :rows="visits"
                :row-key="(row) => row.id"
                :loading="visitsLoading"
                :bordered="false"
                aria-label="服务器访问记录"
              >
                <template #cell-created_at="{ row }">
                  {{ formatDateTime(row.created_at) }}
                </template>
                <template #cell-username="{ row }">
                  <RouterLink :to="`/users/${row.user_id}`">
                    {{ row.username }}
                  </RouterLink>
                </template>
                <template #cell-node_name="{ row }">
                  {{ row.node_name || '—' }}
                </template>
                <template #cell-target="{ row }">
                  <span class="mono">{{ formatTarget(row) }}</span>
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
                  该服务器暂无访问记录
                </template>
              </DataTable>
              <TablePaginator
                :page="visitPage"
                :page-size="visitPageSize"
                :total="visitTotal"
                @change="onVisitPageChange"
              />
            </div>
          </section>
        </main>

        <aside
          class="detail-aside"
          aria-label="服务器摘要"
        >
          <div class="detail-aside-inner">
            <section class="summary-panel">
              <div class="summary-panel-head">
                <h2>运行摘要</h2>
                <HeartPulse :size="17" />
              </div>
              <dl class="summary-list">
                <div class="summary-row">
                  <dt>服务器状态</dt>
                  <dd><StatusBadge v-bind="serverStatusInfo(server.status)" /></dd>
                </div>
                <div class="summary-row">
                  <dt>Agent 状态</dt>
                  <dd>{{ server.agent ? '已注册' : '待注册' }}</dd>
                </div>
                <div class="summary-row">
                  <dt>Agent 版本</dt>
                  <dd class="mono">
                    {{ server.agent?.version || '—' }}
                  </dd>
                </div>
                <div class="summary-row">
                  <dt>运行时间</dt>
                  <dd>{{ formatDuration(server.uptime_seconds) }}</dd>
                </div>
                <div class="summary-row">
                  <dt>在线用户</dt>
                  <dd>{{ server.online_users }}</dd>
                </div>
              </dl>
            </section>

            <section class="summary-panel">
              <div class="summary-panel-head">
                <h2>资源与拓扑</h2>
                <Waypoints :size="17" />
              </div>
              <dl class="summary-list">
                <div class="summary-row">
                  <dt>资源压力</dt>
                  <dd :class="pressureClass">
                    {{ pressureLabel }}
                  </dd>
                </div>
                <div class="summary-row">
                  <dt>最高占用</dt>
                  <dd>{{ formatPercent(maxResourcePercent) }}</dd>
                </div>
                <div class="summary-row">
                  <dt>节点数量</dt>
                  <dd>{{ server.nodes.length }} 个</dd>
                </div>
                <div class="summary-row">
                  <dt>配置版本</dt>
                  <dd class="mono">
                    r{{ server.revision }}
                  </dd>
                </div>
              </dl>
              <div class="summary-panel-footer">
                <div class="summary-callout">
                  <ShieldCheck :size="16" />
                  <span v-if="server.agent">
                    Agent 最近一次心跳为 {{ formatRelative(server.agent.last_seen_at) }}，配置由面板统一下发。
                  </span>
                  <span v-else>
                    生成 Agent Key 并完成 Agent 安装后，运行指标和节点配置才会开始同步。
                  </span>
                </div>
              </div>
            </section>
          </div>
        </aside>
      </div>
    </template>

    <ServerFormDialog
      :open="showEdit"
      :server="server"
      @close="showEdit = false"
      @saved="load"
    />
    <NodeFormDialog
      :open="showNodeDialog"
      :server-id="serverId ?? 0"
      @close="showNodeDialog = false"
      @saved="onSaved"
    />
    <NodeFormDialog
      :open="editNode !== null"
      :server-id="serverId ?? 0"
      :node="editNode"
      @close="editNode = null"
      @saved="onSaved"
    />

    <ConfirmDialog
      :open="showDeleteConfirm"
      title="删除服务器"
      :message="`确定删除服务器「${server?.name ?? ''}」吗？\n将同时删除其 Agent、节点、节点授权和当前连接。`"
      danger
      confirm-text="删除"
      :loading="deleting"
      @cancel="showDeleteConfirm = false"
      @confirm="confirmDeleteServer"
    />

    <ConfirmDialog
      :open="deleteNodeTarget !== null"
      title="删除节点"
      :message="`确定删除节点「${deleteNodeTarget?.name ?? ''}」吗？\n将移除该节点的用户授权，Agent 下次同步后停止该服务。`"
      danger
      confirm-text="删除"
      :loading="deletingNode"
      @cancel="deleteNodeTarget = null"
      @confirm="confirmDeleteNode"
    />

    <ConfirmDialog
      :open="showResetKeyConfirm"
      title="重置 Agent Key"
      :message="`确定重置服务器「${server?.name ?? ''}」的 Agent Key 吗？\n旧 Key 将立即失效，需要使用新 Key 更新节点上的 Agent 配置。`"
      danger
      confirm-text="重置"
      :loading="resettingAgentKey"
      @cancel="showResetKeyConfirm = false"
      @confirm="confirmResetAgentKey"
    />
  </section>
</template>

<style scoped>
.detail-page {
  max-width: 1540px;
}

.detail-main :deep(.card) {
  margin-top: 0;
}

.text-success {
  color: var(--color-success);
}

.text-warning {
  color: var(--color-warning);
}

.loading-block {
  display: flex;
  justify-content: center;
  padding: 72px 0;
}

.agent-missing {
  margin: 0 0 var(--spacing-md);
}

.token-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

.agent-key-block {
  margin-top: var(--spacing-md);
  padding-top: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}

.agent-key-value {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  overflow-x: auto;
}

.agent-key-value code {
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.secret-block {
  margin-top: var(--spacing-md);
}

.install-block {
  margin-top: var(--spacing-md);
  padding-top: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}

.install-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-sm);
}

.install-title {
  font-size: var(--font-size-md);
  font-weight: 600;
}

.install-hint {
  margin: 0 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.install-hint.ok {
  color: var(--color-success);
}

.install-code-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  font-size: var(--font-size-sm);
}

.install-code {
  margin: var(--spacing-xs) 0 0;
  padding: var(--spacing-sm) var(--spacing-md);
  background: var(--color-surface-muted);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  font-size: var(--font-size-sm);
  line-height: 1.6;
  overflow-x: auto;
}

.install-note {
  margin: var(--spacing-xs) 0 0;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.metrics {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  max-width: 640px;
}

.uptime-row {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-sm);
}

.node-head-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.visit-count {
  font-size: var(--font-size-sm);
}

@media (max-width: 700px) {
  .token-actions .btn,
  .node-head-actions .btn {
    flex: 1 1 auto;
  }

  .install-head,
  .install-code-head {
    align-items: stretch;
    flex-direction: column;
  }

  .install-segmented {
    align-self: flex-start;
  }
}
</style>
