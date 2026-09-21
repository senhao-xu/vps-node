<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { RouterLink, useRoute, useRouter } from 'vue-router'
import { deleteServer, getServer, rotateAgentToken, createRegisterToken, updateServer } from '@/api/servers'
import { deleteNode } from '@/api/nodes'
import { errorMessage } from '@/api/http'
import type { AgentTokenResult, NodeBrief, RegisterTokenResult, ServerDetail, ServerStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import DataTable from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import MetricBar from '@/components/MetricBar.vue'
import NodeFormDialog from '@/components/NodeFormDialog.vue'
import OneTimeSecret from '@/components/OneTimeSecret.vue'
import ServerFormDialog from '@/components/ServerFormDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { copyText } from '@/utils/clipboard'
import { formatDateTime, formatDuration, formatRelative } from '@/utils/format'
import { binaryInstallCommand, dockerInstallCommand } from '@/utils/installCommands'
import { nodeStatusInfo, protocolLabel, serverStatusInfo } from '@/utils/labels'

const route = useRoute()
const router = useRouter()

const server = ref<ServerDetail | null>(null)
const loading = ref(false)
const error = ref('')
const actionError = ref('')

const showEdit = ref(false)
const showDeleteConfirm = ref(false)
const deleting = ref(false)
const statusUpdating = ref(false)
const rotatingAgentToken = ref(false)
const agentTokenResult = ref<AgentTokenResult | null>(null)
const registerTokenResult = ref<RegisterTokenResult | null>(null)
const generatingRegisterToken = ref(false)

type InstallTab = 'binary' | 'docker'

const installTab = ref<InstallTab>('binary')
const commandCopied = ref(false)
let copyTimer: ReturnType<typeof setTimeout> | null = null

const panelOrigin = window.location.origin
const freshRegisterToken = computed(() => registerTokenResult.value?.register_token ?? '')

const installNotes: Record<InstallTab, string> = {
  binary: 'install 脚本目前随 release tarball 分发，脚本地址需按实际发布渠道替换。',
  docker: '镜像从 GitHub Container Registry（ghcr.io）拉取；端口映射请按节点实际端口修改。',
}

const activeInstallCommand = computed(() =>
  installTab.value === 'binary'
    ? binaryInstallCommand(panelOrigin, server.value?.id ?? 0, freshRegisterToken.value)
    : dockerInstallCommand(panelOrigin, server.value?.id ?? 0, freshRegisterToken.value),
)

async function copyInstallCommand() {
  const ok = await copyText(activeInstallCommand.value)
  commandCopied.value = ok
  if (copyTimer) clearTimeout(copyTimer)
  copyTimer = setTimeout(() => {
    commandCopied.value = false
  }, 1500)
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

const nodeColumns: { key: string; label: string; align?: 'left' | 'right' | 'center'; width?: string }[] = [
  { key: 'name', label: '节点名称' },
  { key: 'protocol', label: '协议', width: '120px' },
  { key: 'port', label: '端口', align: 'right', width: '80px' },
  { key: 'status', label: '状态', width: '80px' },
  { key: 'actions', label: '操作', width: '130px' },
]

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
  actionError.value = ''
  try {
    await updateServer(current.id, { status: next })
    await load()
  } catch (err) {
    actionError.value = errorMessage(err)
  } finally {
    statusUpdating.value = false
  }
}

async function rotateToken() {
  const current = server.value
  if (!current) return
  rotatingAgentToken.value = true
  actionError.value = ''
  try {
    agentTokenResult.value = await rotateAgentToken(current.id)
  } catch (err) {
    actionError.value = errorMessage(err)
  } finally {
    rotatingAgentToken.value = false
  }
}

async function generateRegisterToken() {
  const current = server.value
  if (!current) return
  generatingRegisterToken.value = true
  actionError.value = ''
  try {
    registerTokenResult.value = await createRegisterToken(current.id)
  } catch (err) {
    actionError.value = errorMessage(err)
  } finally {
    generatingRegisterToken.value = false
  }
}

async function autoGenerateRegisterToken() {
  if (registerTokenResult.value || generatingRegisterToken.value) return
  const current = server.value
  if (!current) return
  generatingRegisterToken.value = true
  try {
    registerTokenResult.value = await createRegisterToken(current.id)
  } catch {
    registerTokenResult.value = null
  } finally {
    generatingRegisterToken.value = false
  }
}

async function confirmDeleteServer() {
  const current = server.value
  if (!current) return
  deleting.value = true
  actionError.value = ''
  try {
    await deleteServer(current.id)
    await router.replace('/servers')
  } catch (err) {
    actionError.value = errorMessage(err)
  } finally {
    deleting.value = false
  }
}

async function confirmDeleteNode() {
  const target = deleteNodeTarget.value
  if (!target) return
  deletingNode.value = true
  actionError.value = ''
  try {
    await deleteNode(target.id)
    deleteNodeTarget.value = null
    await load()
  } catch (err) {
    actionError.value = errorMessage(err)
  } finally {
    deletingNode.value = false
  }
}

function onSaved() {
  void load()
}

watch(serverId, () => {
  agentTokenResult.value = null
  registerTokenResult.value = null
  void load().then(() => {
    void autoGenerateRegisterToken()
  })
})

onMounted(() => {
  void load().then(() => {
    void autoGenerateRegisterToken()
  })
})
</script>

<template>
  <section class="page">
    <div class="page-header">
      <h1 class="page-title">
        <span class="eyebrow">INFRASTRUCTURE NODE</span>
        服务器详情
        <span
          v-if="server"
          class="head-status"
        >
          <StatusBadge v-bind="serverStatusInfo(server.status)" />
        </span>
      </h1>
      <div class="header-actions">
        <button
          v-if="server"
          type="button"
          class="btn secondary"
          :disabled="statusUpdating"
          @click="toggleStatus"
        >
          {{ server.status === 'disabled' ? '启用' : '禁用' }}
        </button>
        <button
          v-if="server"
          type="button"
          class="btn secondary"
          @click="showEdit = true"
        >
          编辑
        </button>
        <button
          type="button"
          class="btn secondary"
          @click="void router.push('/servers')"
        >
          返回列表
        </button>
        <button
          v-if="server"
          type="button"
          class="btn danger secondary"
          @click="showDeleteConfirm = true"
        >
          删除
        </button>
      </div>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <ErrorBanner
      :message="actionError"
      @dismiss="actionError = ''"
    />

    <div
      v-if="loading && !server"
      class="empty-tip"
    >
      加载中…
    </div>
    <template v-else-if="server">
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
            <span class="info-label">地址</span>
            <span class="mono">{{ server.address }}</span>
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
          该服务器还没有注册 Agent。点击下方按钮生成一次性注册 Token，在服务器上运行 Agent 时使用。
        </p>
        <div class="token-actions">
          <button
            v-if="server.agent"
            type="button"
            class="btn secondary small"
            :disabled="rotatingAgentToken"
            @click="rotateToken"
          >
            {{ rotatingAgentToken ? '生成中…' : '轮换 Agent Token' }}
          </button>
          <button
            type="button"
            class="btn secondary small"
            :disabled="generatingRegisterToken"
            @click="generateRegisterToken"
          >
            {{ generatingRegisterToken ? '生成中…' : (server.agent ? '重新生成注册 Token' : '生成注册 Token') }}
          </button>
        </div>
        <OneTimeSecret
          v-if="agentTokenResult"
          class="secret-block"
          label="新 Agent Token（旧 Token 已立即失效）"
          :value="agentTokenResult.agent_token"
          hint="仅显示这一次，请立即复制保存并更新 Agent 配置。"
        />
        <OneTimeSecret
          v-if="registerTokenResult"
          class="secret-block"
          label="注册 Token"
          :value="registerTokenResult.register_token"
          :hint="`仅显示这一次，有效期至 ${formatDateTime(registerTokenResult.expires_at)}。`"
        />
        <div class="install-block">
          <div class="install-head">
            <span class="install-title">Agent 安装</span>
            <div class="install-tabs">
              <button
                type="button"
                class="install-tab"
                :class="{ active: installTab === 'binary' }"
                @click="installTab = 'binary'"
              >
                二进制安装
              </button>
              <button
                type="button"
                class="install-tab"
                :class="{ active: installTab === 'docker' }"
                @click="installTab = 'docker'"
              >
                Docker 安装
              </button>
            </div>
          </div>
          <p
            v-if="freshRegisterToken === ''"
            class="install-hint"
          >
            {{ generatingRegisterToken ? '正在生成注册 Token…' : '注册 Token 生成失败，请点击上方「生成注册 Token」重试。' }}
          </p>
          <p
            v-else
            class="install-hint ok"
          >
            已自动内嵌注册 Token，有效期至 {{ registerTokenResult ? formatDateTime(registerTokenResult.expires_at) : '' }}。
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
          <pre class="install-code">{{ activeInstallCommand }}</pre>
          <p class="install-note">
            {{ installNotes[installTab] }}
          </p>
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
          :loading="loading"
        >
          <template #cell-protocol="{ row }">
            {{ protocolLabel(row.protocol) }}
          </template>
          <template #cell-status="{ row }">
            <StatusBadge v-bind="nodeStatusInfo(row.status)" />
          </template>
          <template #cell-actions="{ row }">
            <span class="actions">
              <button
                type="button"
                class="btn link"
                @click="editNode = row"
              >编辑</button>
              <button
                type="button"
                class="btn link"
                @click="deleteNodeTarget = row"
              >删除</button>
            </span>
          </template>
          <template #empty>
            该服务器还没有节点，点击右上角添加
          </template>
        </DataTable>
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
  </section>
</template>

<style scoped>
.header-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
}

.eyebrow {
  display: block;
  margin-bottom: var(--spacing-xs);
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.head-status {
  margin-left: var(--spacing-sm);
  vertical-align: middle;
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
  gap: var(--spacing-md) var(--spacing-lg);
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  min-width: 0;
}

.info-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.agent-missing {
  margin: 0 0 var(--spacing-md);
}

.token-actions {
  display: flex;
  gap: var(--spacing-sm);
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

.install-tabs {
  display: inline-flex;
  gap: var(--spacing-xs);
  padding: 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface-muted);
}

.install-tab {
  border: none;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  padding: 4px 12px;
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.install-tab.active {
  background: var(--color-surface);
  color: var(--color-primary);
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
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
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
  max-width: 480px;
}

.uptime-row {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-sm);
}

.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.card-head .card-title {
  margin-bottom: 0;
}

.node-head-actions {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.actions {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
}

@media (max-width: 700px) {
  .header-actions {
    width: 100%;
  }

  .info-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .install-head,
  .install-code-head {
    align-items: stretch;
    flex-direction: column;
  }

  .install-tabs {
    align-self: flex-start;
  }

  .card-head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
