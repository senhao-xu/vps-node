<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createNode, generateRealityKeypair, updateNode } from '@/api/nodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { NodeBrief, NodeSettingsInput, Protocol, Server } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import CopyText from '@/components/CopyText.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import { SHADOWSOCKS_METHODS, protocolLabel } from '@/utils/labels'

const props = withDefaults(
  defineProps<{
    open: boolean
    serverId?: number
    node?: NodeBrief | null
  }>(),
  { node: null, serverId: undefined },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const name = ref('')
const port = ref<number | null>(null)
const protocol = ref<Protocol>('shadowsocks')
const status = ref<'active' | 'disabled'>('active')

const ssMethod = ref<string>(SHADOWSOCKS_METHODS[0])
const ssMethodTouched = ref(false)

const vlessPrivateKey = ref('')
const vlessPublicKey = ref('')
const vlessShortId = ref('')
const vlessServerNames = ref('')

const hy2Up = ref<number | null>(null)
const hy2Down = ref<number | null>(null)
const hy2ObfsPassword = ref('')
const hy2HopPorts = ref('')

const tlsServerName = ref('')
const tlsCertificate = ref('')
const tlsPrivateKey = ref('')

const submitting = ref(false)
const generatingReality = ref(false)
const error = ref('')
const isEdit = ref(false)

const servers = ref<Server[]>([])
const serverChoice = ref<number | null>(null)
const loadingServers = ref(false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    error.value = ''
    isEdit.value = props.node !== null
    name.value = props.node?.name ?? ''
    port.value = props.node?.port ?? null
    protocol.value = props.node?.protocol ?? 'shadowsocks'
    status.value = props.node?.status === 'disabled' ? 'disabled' : 'active'
    ssMethod.value = SHADOWSOCKS_METHODS[0]
    ssMethodTouched.value = false
    vlessPrivateKey.value = ''
    vlessPublicKey.value = ''
    vlessShortId.value = ''
    vlessServerNames.value = ''
    hy2Up.value = null
    hy2Down.value = null
    hy2ObfsPassword.value = ''
    hy2HopPorts.value = ''
    tlsServerName.value = ''
    tlsCertificate.value = ''
    tlsPrivateKey.value = ''
    serverChoice.value = null
    if (!isEdit.value && props.serverId === undefined) {
      void loadServers()
    }
  },
)

async function loadServers() {
  loadingServers.value = true
  try {
    const result = await listServers({ page: 1, pageSize: 100 })
    servers.value = result.items
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loadingServers.value = false
  }
}

const title = computed(() => (isEdit.value ? '编辑节点' : '添加节点'))

const protocolOptions: Array<{ value: Protocol; label: string; dot: string }> = [
  { value: 'shadowsocks', label: protocolLabel('shadowsocks'), dot: 'success' },
  { value: 'vless', label: protocolLabel('vless'), dot: 'primary' },
  { value: 'hysteria2', label: protocolLabel('hysteria2'), dot: 'warning' },
  { value: 'anytls', label: protocolLabel('anytls'), dot: 'danger' },
]

const settingsPayload = computed<NodeSettingsInput | undefined>(() => {
  const payload: NodeSettingsInput = {}
  if (protocol.value === 'shadowsocks') {
    if (!isEdit.value || ssMethodTouched.value) payload['method'] = ssMethod.value
  } else if (protocol.value === 'vless') {
    if (vlessPrivateKey.value) payload['private_key'] = vlessPrivateKey.value
    if (vlessShortId.value) payload['short_id'] = vlessShortId.value
    const names = vlessServerNames.value
      .split(/[,，\s]+/)
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
    if (names.length > 0) payload['server_names'] = names
  } else if (protocol.value === 'hysteria2' || protocol.value === 'anytls') {
    if (protocol.value === 'hysteria2') {
      if (hy2Up.value !== null && !Number.isNaN(hy2Up.value) && hy2Up.value >= 0) {
        payload['up_mbps'] = hy2Up.value
      }
      if (hy2Down.value !== null && !Number.isNaN(hy2Down.value) && hy2Down.value >= 0) {
        payload['down_mbps'] = hy2Down.value
      }
      if (hy2ObfsPassword.value) payload['obfs_password'] = hy2ObfsPassword.value
      if (hy2HopPorts.value.trim()) payload['hop_ports'] = hy2HopPorts.value.trim()
    }
    if (tlsServerName.value) payload['server_name'] = tlsServerName.value.trim()
    if (tlsCertificate.value) payload['certificate'] = tlsCertificate.value
    if (tlsPrivateKey.value) payload['private_key'] = tlsPrivateKey.value
  }
  if (Object.keys(payload).length === 0) return undefined
  return payload
})

const validationMessage = computed(() => {
  if (!isEdit.value && props.serverId === undefined && serverChoice.value === null) {
    return '请选择服务器'
  }
  if (!name.value.trim()) return '请填写节点名称'
  const portValue = port.value
  if (portValue === null || !Number.isInteger(portValue) || portValue < 1 || portValue > 65535) {
    return '端口必须是 1-65535 的整数'
  }
  if (protocol.value === 'vless') {
    if (!isEdit.value && !vlessPrivateKey.value) return '请生成或填写 Reality 私钥'
    if (!isEdit.value && !vlessServerNames.value.trim()) return '请填写至少一个 Server Name'
    if (vlessPrivateKey.value && !/^[A-Za-z0-9_-]{43}$/.test(vlessPrivateKey.value)) {
      return 'Reality 私钥格式不正确'
    }
    if (vlessShortId.value && !/^(?:[0-9a-fA-F]{2}){1,8}$/.test(vlessShortId.value)) {
      return 'Short ID 必须是 2-16 位偶数长度十六进制字符'
    }
  } else if (protocol.value === 'hysteria2' || protocol.value === 'anytls') {
    if (protocol.value === 'hysteria2') {
      for (const value of [hy2Up.value, hy2Down.value]) {
        if (value !== null && (!Number.isInteger(value) || value < 0)) {
          return '带宽必须是非负整数'
        }
      }
      if (hy2ObfsPassword.value.length > 64) return 'obfs 混淆密码最长 64 个字符'
      const hop = hy2HopPorts.value.trim()
      if (hop) {
        const match = /^(\d+)-(\d+)$/.exec(hop)
        if (!match) return '端口跳跃格式应为 start-end，例如 30000-40000'
        const start = Number(match[1])
        const end = Number(match[2])
        if (start < 1 || end > 65535 || start > end) {
          return '端口跳跃范围必须在 1-65535 之间且起始端口不大于结束端口'
        }
      }
    }
    const tlsLabel = protocolLabel(protocol.value)
    if (!isEdit.value && !tlsServerName.value.trim()) return `请填写 ${tlsLabel} Server Name`
    if (!isEdit.value && (!tlsCertificate.value || !tlsPrivateKey.value)) return `请填写 ${tlsLabel} PEM 证书链和私钥`
    if ((tlsCertificate.value && !tlsPrivateKey.value) || (!tlsCertificate.value && tlsPrivateKey.value)) return '证书链和私钥必须成对提交'
    if (tlsServerName.value && !/^[A-Za-z0-9.-]+$/.test(tlsServerName.value.trim())) return 'Server Name 格式不正确'
  }
  return ''
})

function changeProtocol() {
  ssMethod.value = SHADOWSOCKS_METHODS[0]
  ssMethodTouched.value = false
  vlessPrivateKey.value = ''
  vlessPublicKey.value = ''
  vlessShortId.value = ''
  vlessServerNames.value = ''
  hy2Up.value = null
  hy2Down.value = null
  hy2ObfsPassword.value = ''
  hy2HopPorts.value = ''
  tlsServerName.value = ''
  tlsCertificate.value = ''
  tlsPrivateKey.value = ''
  error.value = ''
}

async function generateReality() {
  generatingReality.value = true
  error.value = ''
  try {
    const generated = await generateRealityKeypair()
    vlessPrivateKey.value = generated.private_key
    vlessPublicKey.value = generated.public_key
    vlessShortId.value = generated.short_id
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    generatingReality.value = false
  }
}

async function submit() {
  const problem = validationMessage.value
  if (problem) {
    error.value = problem
    return
  }
  submitting.value = true
  error.value = ''
  try {
    if (isEdit.value && props.node) {
      await updateNode(props.node.id, {
        name: name.value.trim(),
        port: port.value ?? undefined,
        settings: settingsPayload.value,
        status: status.value,
      })
    } else {
      await createNode({
        server_id: props.serverId ?? serverChoice.value ?? 0,
        name: name.value.trim(),
        protocol: protocol.value,
        port: port.value ?? 0,
        settings: settingsPayload.value,
      })
    }
    emit('saved')
    emit('close')
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="title"
    :subtitle="isEdit ? '修改节点配置，协议不可变更' : '创建入站节点并配置协议参数'"
    :width="620"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="form">
      <section class="form-section">
        <div class="section-heading">
          <span class="section-index">1</span>
          <div>
            <strong>基础信息</strong>
            <span>定义节点名称、监听端口和协议类型</span>
          </div>
          <small>基本设置</small>
        </div>
        <div
          v-if="!isEdit && props.serverId === undefined"
          class="form-row"
        >
          <div class="field">
            <label for="node-server">所属服务器</label>
            <select
              id="node-server"
              v-model="serverChoice"
              :disabled="loadingServers"
            >
              <option
                :value="null"
                disabled
              >
                {{ loadingServers ? '加载服务器列表…' : '请选择服务器' }}
              </option>
              <option
                v-for="server in servers"
                :key="server.id"
                :value="server.id"
              >
                {{ server.name }}（{{ server.address }}）
              </option>
            </select>
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label for="node-name">名称</label>
            <input
              id="node-name"
              v-model="name"
              type="text"
              placeholder="例如 HK-SS"
            >
          </div>
          <div
            class="field port-field"
          >
            <label for="node-port">端口</label>
            <input
              id="node-port"
              v-model.number="port"
              type="number"
              min="1"
              max="65535"
            >
          </div>
        </div>
        <div class="form-row">
          <div class="field protocol-field">
            <label id="node-protocol-label">协议</label>
            <AppSelect
              v-model="protocol"
              :options="protocolOptions"
              :disabled="isEdit"
              label="协议"
              @update:model-value="changeProtocol"
            />
          </div>
          <div
            v-if="isEdit"
            class="field status-field"
          >
            <label>状态</label>
            <div class="toggle-row status-toggle">
              <div class="toggle-row-text">
                <span class="toggle-row-label">启用节点</span>
              </div>
              <ToggleSwitch
                :model-value="status === 'active'"
                label="启用节点"
                @update:model-value="status = $event ? 'active' : 'disabled'"
              />
            </div>
          </div>
        </div>
      </section>

      <section
        class="form-section"
      >
        <div class="section-heading">
          <span class="section-index">2</span>
          <div>
            <strong>协议配置</strong>
            <span>{{ protocolLabel(protocol) }} 入站使用的参数</span>
          </div>
          <span class="protocol-badge">{{ protocolLabel(protocol) }}</span>
        </div>
        <div
          v-if="protocol === 'shadowsocks'"
          class="settings-box"
        >
          <div class="field method-field">
            <label for="ss-method">加密方式{{ isEdit ? '（更改后才会提交）' : '' }}</label>
            <select
              id="ss-method"
              v-model="ssMethod"
              @change="ssMethodTouched = true"
            >
              <option
                v-for="method in SHADOWSOCKS_METHODS"
                :key="method"
                :value="method"
              >
                {{ method }}
              </option>
            </select>
          </div>
          <p class="protocol-note">
            服务端与用户密钥由 Panel 安全派生，无需手工填写密码。
          </p>
        </div>

        <div
          v-else-if="protocol === 'vless'"
          class="settings-box"
        >
          <div class="reality-actions">
            <div>
              <strong>Reality 密钥</strong>
              <p>生成匹配的 X25519 密钥对和 Short ID。</p>
            </div>
            <button
              type="button"
              class="btn secondary small"
              :disabled="generatingReality || submitting"
              @click="generateReality"
            >
              {{ generatingReality ? '生成中…' : vlessPublicKey ? '重新生成' : '一键生成' }}
            </button>
          </div>
          <div
            v-if="vlessPublicKey"
            class="public-key"
          >
            <span>公钥（仅本次显示）</span>
            <CopyText
              :text="vlessPublicKey"
              :display="vlessPublicKey"
            />
          </div>
          <div class="field secret-field">
            <label for="vless-key">Reality 私钥{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <input
              id="vless-key"
              v-model="vlessPrivateKey"
              type="password"
              autocomplete="new-password"
              :placeholder="isEdit ? '留空保持不变' : '点击一键生成或手工填写'"
            >
          </div>
          <div class="form-row vless-fields">
            <div class="field">
              <label for="vless-short-id">Short ID</label>
              <input
                id="vless-short-id"
                v-model="vlessShortId"
                type="text"
                placeholder="例如 0123456789abcdef"
              >
            </div>
            <div class="field">
              <label for="vless-names">Server Names{{ isEdit ? '（留空保持不变）' : '' }}</label>
              <input
                id="vless-names"
                v-model="vlessServerNames"
                type="text"
                placeholder="例如 example.com,www.example.com"
              >
            </div>
          </div>
          <p class="field-hint vless-hint">
            Server Name 用逗号分隔；Short ID 为可选的 2–16 位偶数长度十六进制字符。
          </p>
        </div>

        <div
          v-else-if="protocol === 'hysteria2' || protocol === 'anytls'"
          class="settings-box"
        >
          <p class="protocol-note">
            用户 UUID 直接作为认证凭据{{ protocol === 'hysteria2' ? '；带宽留空表示不限制' : '' }}。
          </p>
          <div
            v-if="protocol === 'hysteria2'"
            class="form-row"
          >
            <div class="field">
              <label for="hy2-up">上行带宽（Mbps）</label>
              <input
                id="hy2-up"
                v-model.number="hy2Up"
                type="number"
                min="0"
                placeholder="选填"
              >
            </div>
            <div class="field">
              <label for="hy2-down">下行带宽（Mbps）</label>
              <input
                id="hy2-down"
                v-model.number="hy2Down"
                type="number"
                min="0"
                placeholder="选填"
              >
            </div>
          </div>
          <div class="form-row">
            <div class="field">
              <label for="tls-server-name">TLS Server Name{{ isEdit ? '（留空保持不变）' : '' }}</label>
              <input
                id="tls-server-name"
                v-model="tlsServerName"
                type="text"
                placeholder="例如 tls.example.com"
              >
            </div>
          </div>
          <div class="field">
            <label for="tls-certificate">PEM 证书链{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <textarea
              id="tls-certificate"
              v-model="tlsCertificate"
              rows="5"
              :placeholder="isEdit ? '留空保持不变' : '-----BEGIN CERTIFICATE-----'"
            />
          </div>
          <div class="field">
            <label for="tls-private-key">PEM 私钥{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <textarea
              id="tls-private-key"
              v-model="tlsPrivateKey"
              rows="5"
              :placeholder="isEdit ? '留空保持不变' : '-----BEGIN PRIVATE KEY-----'"
            />
          </div>
          <p class="field-hint">
            证书和私钥会加密保存，并由 Panel 以内联 TLS 配置下发 Agent；编辑时两个字段必须一起替换。
          </p>
          <template v-if="protocol === 'hysteria2'">
            <div class="form-row">
              <div class="field">
                <label for="hy2-obfs-password">obfs 混淆密码{{ isEdit ? '（留空保持不变）' : '' }}</label>
                <input
                  id="hy2-obfs-password"
                  v-model="hy2ObfsPassword"
                  type="text"
                  placeholder="选填，最长 64 字符"
                >
              </div>
              <div class="field">
                <label for="hy2-hop-ports">端口跳跃{{ isEdit ? '（留空保持不变）' : '' }}</label>
                <input
                  id="hy2-hop-ports"
                  v-model="hy2HopPorts"
                  type="text"
                  placeholder="选填，如 30000-40000"
                >
              </div>
            </div>
            <p class="field-hint">
              启用 obfs 后使用 Salamander 混淆，客户端需同步填写相同密码。
            </p>
            <p class="field-hint hop-warning">
              端口跳跃仅影响订阅客户端，需在服务器上自行配置 NAT 端口转发，否则客户端无法连通。
            </p>
            <p class="field-hint">
              留空表示不限制带宽，单位为 Mbps。
            </p>
          </template>
        </div>
      </section>

      <p class="text-secondary tip">
        协议密钥由 Panel 加密保存且不会回显；编辑时留空即保持不变。
      </p>
    </div>
    <template #footer>
      <button
        type="button"
        class="btn secondary"
        :disabled="submitting"
        @click="emit('close')"
      >
        取消
      </button>
      <button
        type="button"
        class="btn"
        :class="{ 'is-loading': submitting }"
        :disabled="submitting"
        @click="submit"
      >
        <LoadingSpinner
          v-if="submitting"
          size="sm"
        />
        {{ submitting ? '保存中…' : '保存' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: 28px;
}

.form-section {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  min-width: 0;
}

.section-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  padding-bottom: var(--spacing-sm);
  border-bottom: 1px solid var(--color-border);
}

.section-heading > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
  flex: 1;
}

.section-index {
  display: inline-flex;
  width: 24px;
  height: 24px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: 50%;
  background: var(--color-primary);
  color: var(--color-on-primary) !important;
  font-size: var(--font-size-sm) !important;
  font-weight: 700;
}

.section-heading small {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.section-heading span,
.protocol-note,
.reality-actions p,
.public-key > span {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.protocol-badge {
  flex: none;
  padding: 3px 8px;
  border: 1px solid var(--color-primary-border);
  border-radius: 999px;
  background: var(--color-primary-soft);
  color: var(--color-primary) !important;
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.protocol-note,
.reality-actions p {
  margin: 0;
}

.reality-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding: 14px;
  border: 1px solid var(--color-primary-border);
  border-radius: var(--radius-md);
  background: var(--color-primary-soft);
}

.reality-actions > div {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.public-key {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: var(--spacing-sm);
  border: 1px solid var(--color-success-border);
  border-radius: var(--radius-md);
  background: var(--color-success-soft);
  min-width: 0;
}

.public-key :deep(.copy-text) {
  justify-content: space-between;
}

.settings-box {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  padding: 20px;
  background: linear-gradient(180deg, var(--color-surface-muted), var(--color-surface));
}

.port-field,
.status-field {
  flex: 0 1 140px !important;
}

.status-toggle {
  padding: 0 var(--spacing-sm);
  min-height: var(--control-height);
  border-radius: var(--radius-sm);
}

.status-toggle .toggle-row-label {
  font-weight: 400;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.method-field {
  max-width: 360px;
}

.field-hint {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.5;
}

.vless-hint {
  margin-top: calc(var(--spacing-xs) * -1);
}

.hop-warning {
  padding: var(--spacing-sm);
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius-md);
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.public-key :deep(.value) {
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: var(--font-size-sm);
}

.tip {
  margin: 0;
  font-size: var(--font-size-sm);
}

@media (max-width: 560px) {
  .form-row,
  .reality-actions {
    align-items: stretch;
    flex-direction: column;
  }

  .port-field,
  .status-field,
  .method-field {
    flex-basis: auto !important;
    max-width: none;
  }

  .protocol-badge {
    align-self: flex-start;
  }

  .settings-box {
    padding: var(--spacing-md);
  }

  .section-heading {
    align-items: flex-start;
    flex-wrap: wrap;
  }

  .section-heading > div {
    min-width: calc(100% - 40px);
  }
}
</style>
