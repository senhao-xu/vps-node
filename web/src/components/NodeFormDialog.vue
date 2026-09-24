<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createNode, generateRealityKeypair, getNode, updateNode } from '@/api/nodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type {
  Hysteria2BandwidthInput,
  Hysteria2ObfsInput,
  NodeBrief,
  NodeSettingsInput,
  Protocol,
  RealitySettingsInput,
  Server,
  TLSSettingsInput,
} from '@/api/types'
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
const address = ref('')
const ipv6Enabled = ref(false)
const ipv6Address = ref('')
const port = ref<number | null>(null)
const protocol = ref<Protocol>('shadowsocks')
const status = ref<'active' | 'disabled'>('active')
const rate = ref<number | null>(1)
const tags = ref('')

const ssCipher = ref<string>(SHADOWSOCKS_METHODS[0])

const vlessPrivateKey = ref('')
const vlessPublicKey = ref('')
const vlessShortId = ref('')
const vlessServerName = ref('')
const vlessServerPort = ref<string | number | null>(443)
const vlessAllowInsecure = ref(false)

const hy2Up = ref<number | null>(null)
const hy2Down = ref<number | null>(null)
const hy2ObfsOpen = ref(false)
const hy2ObfsPassword = ref('')
const hy2HopInterval = ref('')
const hy2AllowInsecure = ref(false)

const anytlsPaddingScheme = ref('')
const anytlsAllowInsecure = ref(false)

const tlsServerName = ref('')
const tlsCertificate = ref('')
const tlsPrivateKey = ref('')

const submitting = ref(false)
const generatingReality = ref(false)
const error = ref('')
const isEdit = ref(false)
const loadingDetail = ref(false)
const detailLoaded = ref(false)

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
    address.value = props.node?.address ?? ''
    ipv6Enabled.value = props.node?.ipv6_enabled ?? false
    ipv6Address.value = props.node?.ipv6_address ?? ''
    port.value = props.node?.port ?? null
    protocol.value = props.node?.protocol ?? 'shadowsocks'
    status.value = props.node?.status === 'disabled' ? 'disabled' : 'active'
    rate.value = props.node?.rate ?? 1
    tags.value = props.node?.tags.join(', ') ?? ''
    ssCipher.value = SHADOWSOCKS_METHODS[0]
    vlessPrivateKey.value = ''
    vlessPublicKey.value = ''
    vlessShortId.value = ''
    vlessServerName.value = ''
    vlessServerPort.value = isEdit.value ? null : 443
    vlessAllowInsecure.value = false
    hy2Up.value = null
    hy2Down.value = null
    hy2ObfsOpen.value = false
    hy2ObfsPassword.value = ''
    hy2HopInterval.value = ''
    hy2AllowInsecure.value = false
    anytlsPaddingScheme.value = ''
    anytlsAllowInsecure.value = false
    tlsServerName.value = ''
    tlsCertificate.value = ''
    tlsPrivateKey.value = ''
    serverChoice.value = null
    detailLoaded.value = false
    if (isEdit.value && props.node) {
      void loadNodeDetail(props.node.id)
    } else if (!isEdit.value && props.serverId === undefined) {
      void loadServers()
    }
  },
)

async function loadNodeDetail(nodeId: number) {
  loadingDetail.value = true
  try {
    const detail = await getNode(nodeId)
    const settings = detail.settings
    if (!settings) {
      detailLoaded.value = true
      return
    }
    if (protocol.value === 'shadowsocks') {
      if (settings.cipher) ssCipher.value = settings.cipher
    } else if (protocol.value === 'vless') {
      const reality = settings.reality_settings
      if (reality) {
        vlessServerName.value = reality.server_name ?? ''
        vlessServerPort.value = reality.server_port ?? null
        vlessShortId.value = reality.short_id ?? ''
        vlessAllowInsecure.value = reality.allow_insecure ?? false
        vlessPublicKey.value = reality.public_key ?? ''
      }
    } else if (protocol.value === 'hysteria2') {
      const tls = typeof settings.tls === 'object' ? settings.tls : undefined
      tlsServerName.value = tls?.server_name ?? ''
      hy2AllowInsecure.value = tls?.allow_insecure ?? false
      hy2Up.value = settings.bandwidth?.up ?? null
      hy2Down.value = settings.bandwidth?.down ?? null
      const obfs = typeof settings.obfs === 'object' ? settings.obfs : undefined
      hy2ObfsOpen.value = obfs?.open ?? false
      hy2ObfsPassword.value = obfs?.password ?? ''
      hy2HopInterval.value = settings.hop_interval ?? ''
    } else if (protocol.value === 'anytls') {
      const tls = typeof settings.tls === 'object' ? settings.tls : undefined
      tlsServerName.value = tls?.server_name ?? ''
      anytlsAllowInsecure.value = tls?.allow_insecure ?? false
      const scheme = settings.padding_scheme
      anytlsPaddingScheme.value = Array.isArray(scheme) ? scheme.join('\n') : (scheme ?? '')
    }
    detailLoaded.value = true
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loadingDetail.value = false
  }
}

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

const serverSelectValue = computed<number>({
  get: () => serverChoice.value ?? 0,
  set: (value) => {
    serverChoice.value = value === 0 ? null : value
  },
})

const serverOptions = computed<Array<{ value: number; label: string }>>(() => [
  {
    value: 0,
    label: loadingServers.value ? '加载服务器列表…' : '请选择服务器',
  },
  ...servers.value.map((server) => ({ value: server.id, label: server.name })),
])

const cipherOptions: Array<{ value: string; label: string }> = SHADOWSOCKS_METHODS.map(
  (method) => ({ value: method, label: method }),
)

function parseList(raw: string): string[] {
  return raw
    .split(/[,，\s]+/)
    .map((item) => item.trim())
    .filter((item) => item.length > 0)
}

/**
 * Reality handshake port. `present: false` means the field was left blank, so an
 * edit keeps the stored value and a create falls back to the panel default.
 * `value: null` with `present: true` is malformed input.
 */
function normaliseServerPort(raw: string | number | null): { present: boolean; value: number | null } {
  if (raw === null || raw === '') return { present: false, value: null }
  const value = typeof raw === 'number' ? raw : Number(raw)
  return { present: true, value: Number.isInteger(value) ? value : null }
}

function isIPv6Literal(value: string): boolean {
  if (!value.includes(':')) return false
  const halves = value.split('::')
  if (halves.length > 2) return false
  const groups = halves.flatMap((half) => (half ? half.split(':') : []))
  if (groups.some((group) => !/^[0-9a-fA-F]{1,4}$/.test(group))) return false
  if (halves.length === 1) return groups.length === 8
  return groups.length < 8
}

const tagsPayload = computed(() => parseList(tags.value))

const settingsPayload = computed<NodeSettingsInput | undefined>(() => {
  if (isEdit.value && !detailLoaded.value) return undefined
  const payload: NodeSettingsInput = {}
  if (protocol.value === 'shadowsocks') {
    payload.cipher = ssCipher.value
  } else if (protocol.value === 'vless') {
    payload.tls = 2
    const reality: RealitySettingsInput = {}
    if (vlessServerName.value.trim()) reality.server_name = vlessServerName.value.trim()
    const serverPort = normaliseServerPort(vlessServerPort.value)
    if (serverPort.value !== null) reality.server_port = serverPort.value
    if (vlessShortId.value.trim()) reality.short_id = vlessShortId.value.trim()
    if (vlessPublicKey.value) reality.public_key = vlessPublicKey.value
    reality.allow_insecure = vlessAllowInsecure.value
    if (Object.keys(reality).length > 0) payload.reality_settings = reality
    if (vlessPrivateKey.value) payload.private_key = vlessPrivateKey.value
  } else if (protocol.value === 'hysteria2') {
    payload.version = 2
    const tls: TLSSettingsInput = {}
    if (tlsServerName.value.trim()) tls.server_name = tlsServerName.value.trim()
    tls.allow_insecure = hy2AllowInsecure.value
    payload.tls = tls

    const bandwidth: Hysteria2BandwidthInput = {}
    if (hy2Up.value !== null && !Number.isNaN(hy2Up.value)) bandwidth.up = hy2Up.value
    if (hy2Down.value !== null && !Number.isNaN(hy2Down.value)) bandwidth.down = hy2Down.value
    if (Object.keys(bandwidth).length > 0) payload.bandwidth = bandwidth

    const obfs: Hysteria2ObfsInput = {}
    obfs.open = hy2ObfsOpen.value
    if (hy2ObfsOpen.value) obfs.type = 'salamander'
    if (hy2ObfsPassword.value) obfs.password = hy2ObfsPassword.value
    payload.obfs = obfs

    if (hy2HopInterval.value.trim()) payload.hop_interval = hy2HopInterval.value.trim()
    if (tlsCertificate.value) payload.certificate = tlsCertificate.value
    if (tlsPrivateKey.value) payload.private_key = tlsPrivateKey.value
  } else if (protocol.value === 'anytls') {
    const tls: TLSSettingsInput = {}
    if (tlsServerName.value.trim()) tls.server_name = tlsServerName.value.trim()
    tls.allow_insecure = anytlsAllowInsecure.value
    payload.tls = tls

    const scheme = parseList(anytlsPaddingScheme.value)
    if (scheme.length > 0) payload.padding_scheme = scheme
    if (tlsCertificate.value) payload.certificate = tlsCertificate.value
    if (tlsPrivateKey.value) payload.private_key = tlsPrivateKey.value
  }
  if (Object.keys(payload).length === 0) return undefined
  return payload
})

const validationMessage = computed(() => {
  if (!isEdit.value && props.serverId === undefined && serverChoice.value === null) {
    return '请选择服务器'
  }
  if (!name.value.trim()) return '请填写节点名称'
  if (!address.value.trim()) return '请填写节点地址'
  if (address.value.trim().length > 255) return '节点地址最长 255 个字符'
  const ipv6 = ipv6Address.value.trim()
  if (ipv6Enabled.value && !ipv6) return '启用 IPv6 入口后请填写 IPv6 地址'
  if (ipv6 && !isIPv6Literal(ipv6)) return 'IPv6 地址格式不正确'
  const portValue = port.value
  if (portValue === null || !Number.isInteger(portValue) || portValue < 1 || portValue > 65535) {
    return '端口必须是 1-65535 的整数'
  }
  const rateValue = rate.value
  if (rateValue === null || Number.isNaN(rateValue) || rateValue <= 0) {
    return '流量倍率必须是大于 0 的数字'
  }
  const tagList = tagsPayload.value
  if (tagList.length > 20) return '标签最多 20 个'
  if (tagList.some((tag) => tag.length > 32)) return '每个标签最长 32 个字符'

  if (protocol.value === 'shadowsocks') {
    if (!SHADOWSOCKS_METHODS.some((method) => method === ssCipher.value)) {
      return '请选择受支持的加密方式'
    }
  } else if (protocol.value === 'vless') {
    if (!isEdit.value && !vlessPrivateKey.value) return '请生成或填写 Reality 私钥'
    if (!isEdit.value && !vlessServerName.value.trim()) return '请填写 Reality Server Name'
    if (vlessPrivateKey.value && !/^[A-Za-z0-9_-]{43}$/.test(vlessPrivateKey.value)) {
      return 'Reality 私钥格式不正确'
    }
    const serverPort = normaliseServerPort(vlessServerPort.value)
    if (
      serverPort.present &&
      (serverPort.value === null || serverPort.value < 1 || serverPort.value > 65535)
    ) {
      return 'Reality 端口必须是 1-65535 的整数'
    }
    if (vlessShortId.value && !/^(?:[0-9a-fA-F]{2}){0,8}$/.test(vlessShortId.value)) {
      return 'Short ID 必须是偶数长度且不超过 16 位的十六进制字符'
    }
  } else if (protocol.value === 'hysteria2') {
    for (const value of [hy2Up.value, hy2Down.value]) {
      if (value !== null && (!Number.isInteger(value) || value < 0)) {
        return '带宽必须是非负整数'
      }
    }
    if (hy2ObfsPassword.value.length > 64) return 'obfs 混淆密码最长 64 个字符'
    const hop = hy2HopInterval.value.trim()
    if (hop) {
      const match = /^(\d+)-(\d+)$/.exec(hop)
      if (!match) return '端口跳跃格式应为 start-end，例如 30000-40000'
      const start = Number(match[1])
      const end = Number(match[2])
      if (start < 1 || end > 65535 || start > end) {
        return '端口跳跃范围必须在 1-65535 之间且起始端口不大于结束端口'
      }
    }
    if (!isEdit.value && !tlsServerName.value.trim()) return '请填写 Hysteria2 Server Name'
    if (!isEdit.value && (!tlsCertificate.value || !tlsPrivateKey.value)) {
      return '请填写 Hysteria2 PEM 证书链和私钥'
    }
    if ((tlsCertificate.value && !tlsPrivateKey.value) || (!tlsCertificate.value && tlsPrivateKey.value)) {
      return '证书链和私钥必须成对提交'
    }
    if (tlsServerName.value.trim() && !/^[A-Za-z0-9.-]+$/.test(tlsServerName.value.trim())) {
      return 'Server Name 格式不正确'
    }
  } else if (protocol.value === 'anytls') {
    if (!isEdit.value && !tlsServerName.value.trim()) return '请填写 AnyTLS Server Name'
    if (!isEdit.value && (!tlsCertificate.value || !tlsPrivateKey.value)) {
      return '请填写 AnyTLS PEM 证书链和私钥'
    }
    if ((tlsCertificate.value && !tlsPrivateKey.value) || (!tlsCertificate.value && tlsPrivateKey.value)) {
      return '证书链和私钥必须成对提交'
    }
    if (tlsServerName.value.trim() && !/^[A-Za-z0-9.-]+$/.test(tlsServerName.value.trim())) {
      return 'Server Name 格式不正确'
    }
  }
  return ''
})

function changeProtocol() {
  ssCipher.value = SHADOWSOCKS_METHODS[0]
  vlessPrivateKey.value = ''
  vlessPublicKey.value = ''
  vlessShortId.value = ''
  vlessServerName.value = ''
  vlessServerPort.value = 443
  vlessAllowInsecure.value = false
  hy2Up.value = null
  hy2Down.value = null
  hy2ObfsOpen.value = false
  hy2ObfsPassword.value = ''
  hy2HopInterval.value = ''
  hy2AllowInsecure.value = false
  anytlsPaddingScheme.value = ''
  anytlsAllowInsecure.value = false
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
  if (loadingDetail.value) return
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
        address: address.value.trim(),
        ipv6_enabled: ipv6Enabled.value,
        ipv6_address: ipv6Address.value.trim(),
        port: port.value ?? undefined,
        rate: rate.value ?? undefined,
        tags: tagsPayload.value,
        settings: settingsPayload.value,
        status: status.value,
      })
    } else {
      await createNode({
        server_id: props.serverId ?? serverChoice.value ?? 0,
        address: address.value.trim(),
        ipv6_enabled: ipv6Enabled.value,
        ipv6_address: ipv6Address.value.trim(),
        name: name.value.trim(),
        protocol: protocol.value,
        port: port.value ?? 0,
        rate: rate.value ?? undefined,
        tags: tagsPayload.value,
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
    :width="640"
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
            <span>定义节点名称、监听端口、流量倍率和标签</span>
          </div>
          <small>基本设置</small>
        </div>
        <div
          v-if="!isEdit && props.serverId === undefined"
          class="form-row"
        >
          <div class="field">
            <label id="node-server-label">所属服务器</label>
            <AppSelect
              v-model="serverSelectValue"
              :options="serverOptions"
              :disabled="loadingServers"
              label="所属服务器"
            />
          </div>
        </div>
        <div class="field">
          <label for="node-address">地址</label>
          <input
            id="node-address"
            v-model="address"
            type="text"
            placeholder="例如 hk01.example.com 或 1.2.3.4"
          >
          <p class="field-hint">
            用户连接使用的域名或 IP，订阅链接按该地址生成。
          </p>
        </div>
        <div class="field">
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">IPv6 入口</span>
            </div>
            <ToggleSwitch
              v-model="ipv6Enabled"
              label="启用 IPv6 入口"
            />
          </div>
          <input
            v-if="ipv6Enabled"
            id="node-ipv6-address"
            v-model="ipv6Address"
            type="text"
            placeholder="例如 2001:db8::1"
          >
          <p class="field-hint">
            启用后订阅会额外生成一条使用该 IPv6 地址的节点条目，端口与协议参数不变。
          </p>
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
          <div class="field port-field">
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
          <div class="field rate-field">
            <label for="node-rate">流量倍率</label>
            <input
              id="node-rate"
              v-model.number="rate"
              type="number"
              min="0.01"
              step="0.1"
            >
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
        <div class="field">
          <label for="node-tags">标签（可选）</label>
          <input
            id="node-tags"
            v-model="tags"
            type="text"
            placeholder="逗号分隔，最多 20 个，例如 hk, premium"
          >
          <p class="field-hint">
            用于订阅分组与筛选；单个标签最长 32 个字符。
          </p>
        </div>
      </section>

      <section class="form-section">
        <div class="section-heading">
          <span class="section-index">2</span>
          <div>
            <strong>协议配置</strong>
            <span>{{ protocolLabel(protocol) }} 入站使用的参数</span>
          </div>
          <span class="protocol-badge">{{ protocolLabel(protocol) }}</span>
        </div>

        <div
          v-if="loadingDetail"
          class="detail-loading"
        >
          <LoadingSpinner size="sm" />
          <span>正在加载节点配置…</span>
        </div>

        <div
          v-if="protocol === 'shadowsocks'"
          class="settings-box"
        >
          <div class="section-label">
            Shadowsocks
          </div>
          <div class="field method-field">
            <label id="ss-cipher-label">加密方式</label>
            <AppSelect
              v-model="ssCipher"
              :options="cipherOptions"
              label="Shadowsocks 加密方式"
            />
          </div>
          <p class="protocol-note">
            服务端与用户密钥由 Panel 安全派生，无需手工填写密码。
          </p>
        </div>

        <div
          v-else-if="protocol === 'vless'"
          class="settings-box"
        >
          <div class="section-label">
            Reality
          </div>
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
            <span>{{ isEdit ? '公钥' : '公钥（仅本次显示）' }}</span>
            <CopyText
              class="mono"
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
          <div class="form-row">
            <div class="field">
              <label for="vless-server-name">Server Name</label>
              <input
                id="vless-server-name"
                v-model="vlessServerName"
                type="text"
                placeholder="例如 www.example.com"
              >
            </div>
            <div class="field port-field">
              <label for="vless-server-port">握手端口</label>
              <input
                id="vless-server-port"
                v-model.number="vlessServerPort"
                type="number"
                min="1"
                max="65535"
                placeholder="443"
              >
            </div>
          </div>
          <div class="field">
            <label for="vless-short-id">Short ID</label>
            <input
              id="vless-short-id"
              v-model="vlessShortId"
              type="text"
              placeholder="例如 0123456789abcdef（≤16 位偶数长度十六进制）"
            >
          </div>
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">允许不安全（allow_insecure）</span>
              <span class="toggle-row-desc">仅测试环境使用，生产环境请保持关闭</span>
            </div>
            <ToggleSwitch
              v-model="vlessAllowInsecure"
              label="允许不安全"
            />
          </div>
          <p class="field-hint">
            Server Name 为 Reality 伪装目标；公钥由私钥自动派生，无需填写。
          </p>
        </div>

        <div
          v-else-if="protocol === 'hysteria2'"
          class="settings-box"
        >
          <div class="section-label">
            Hysteria2 · TLS
          </div>
          <div class="form-row">
            <div class="field">
              <label for="hy2-server-name">TLS Server Name</label>
              <input
                id="hy2-server-name"
                v-model="tlsServerName"
                type="text"
                placeholder="例如 tls.example.com"
              >
            </div>
            <div class="field port-field">
              <label for="hy2-version">协议版本</label>
              <input
                id="hy2-version"
                :value="2"
                type="text"
                disabled
              >
            </div>
          </div>
          <div class="field">
            <label for="hy2-certificate">PEM 证书链{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <textarea
              id="hy2-certificate"
              v-model="tlsCertificate"
              rows="5"
              :placeholder="isEdit ? '留空保持不变' : '-----BEGIN CERTIFICATE-----'"
            />
          </div>
          <div class="field">
            <label for="hy2-private-key">PEM 私钥{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <textarea
              id="hy2-private-key"
              v-model="tlsPrivateKey"
              rows="5"
              :placeholder="isEdit ? '留空保持不变' : '-----BEGIN PRIVATE KEY-----'"
            />
          </div>
          <p class="field-hint">
            证书和私钥会加密保存，并由 Panel 以内联 TLS 配置下发 Agent；编辑时两个字段必须一起替换。
          </p>

          <div class="section-label">
            Hysteria2 · 带宽与混淆
          </div>
          <div class="form-row">
            <div class="field">
              <label for="hy2-up">上行带宽（Mbps）</label>
              <input
                id="hy2-up"
                v-model.number="hy2Up"
                type="number"
                min="0"
                placeholder="选填，留空表示不限制"
              >
            </div>
            <div class="field">
              <label for="hy2-down">下行带宽（Mbps）</label>
              <input
                id="hy2-down"
                v-model.number="hy2Down"
                type="number"
                min="0"
                placeholder="选填，留空表示不限制"
              >
            </div>
          </div>
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">启用 Salamander 混淆</span>
              <span class="toggle-row-desc">客户端需同步填写相同的混淆密码</span>
            </div>
            <ToggleSwitch
              v-model="hy2ObfsOpen"
              label="启用混淆"
            />
          </div>
          <div class="form-row">
            <div class="field">
              <label for="hy2-obfs-type">混淆类型</label>
              <input
                id="hy2-obfs-type"
                :value="'salamander'"
                type="text"
                disabled
              >
            </div>
            <div class="field">
              <label for="hy2-obfs-password">obfs 混淆密码</label>
              <input
                id="hy2-obfs-password"
                v-model="hy2ObfsPassword"
                type="text"
                placeholder="选填，最长 64 字符"
              >
            </div>
          </div>
          <div class="field">
            <label for="hy2-hop-interval">端口跳跃 hop_interval</label>
            <input
              id="hy2-hop-interval"
              v-model="hy2HopInterval"
              type="text"
              placeholder="选填，如 30000-40000"
            >
          </div>
          <p class="field-hint hop-warning">
            端口跳跃仅影响订阅客户端，需在服务器上自行配置 NAT 端口转发，否则客户端无法连通。
          </p>
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">允许不安全（allow_insecure）</span>
              <span class="toggle-row-desc">仅测试环境使用，生产环境请保持关闭</span>
            </div>
            <ToggleSwitch
              v-model="hy2AllowInsecure"
              label="允许不安全"
            />
          </div>
        </div>

        <div
          v-else-if="protocol === 'anytls'"
          class="settings-box"
        >
          <div class="section-label">
            AnyTLS · TLS
          </div>
          <div class="field">
            <label for="anytls-server-name">TLS Server Name</label>
            <input
              id="anytls-server-name"
              v-model="tlsServerName"
              type="text"
              placeholder="例如 tls.example.com"
            >
          </div>
          <div class="field">
            <label for="anytls-certificate">PEM 证书链{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <textarea
              id="anytls-certificate"
              v-model="tlsCertificate"
              rows="5"
              :placeholder="isEdit ? '留空保持不变' : '-----BEGIN CERTIFICATE-----'"
            />
          </div>
          <div class="field">
            <label for="anytls-private-key">PEM 私钥{{ isEdit ? '（留空保持不变）' : '' }}</label>
            <textarea
              id="anytls-private-key"
              v-model="tlsPrivateKey"
              rows="5"
              :placeholder="isEdit ? '留空保持不变' : '-----BEGIN PRIVATE KEY-----'"
            />
          </div>
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">允许不安全（allow_insecure）</span>
              <span class="toggle-row-desc">仅测试环境使用，生产环境请保持关闭</span>
            </div>
            <ToggleSwitch
              v-model="anytlsAllowInsecure"
              label="允许不安全"
            />
          </div>
          <div class="field">
            <label for="anytls-padding">padding_scheme（可选）</label>
            <textarea
              id="anytls-padding"
              v-model="anytlsPaddingScheme"
              rows="1"
              placeholder="每行或逗号分隔一个填充方案，留空使用默认"
            />
          </div>
          <p class="field-hint">
            用户 UUID 直接作为认证凭据；证书和私钥必须成对替换。
          </p>
        </div>
      </section>

      <p class="text-secondary tip">
        协议密钥由 Panel 加密保存且不会回显；编辑时密钥类字段留空即保持不变，公开字段展示即当前生效值。
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
        :disabled="submitting || loadingDetail"
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

.section-label {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.04em;
}

.protocol-badge {
  flex: none;
  padding: 3px 8px;
  border: 1px solid var(--color-primary-border);
  border-radius: var(--radius-full);
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
.status-field,
.rate-field {
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

.hop-warning {
  padding: var(--spacing-sm);
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius-md);
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.public-key :deep(.value) {
  font-size: var(--font-size-sm);
}

.tip {
  margin: 0;
  font-size: var(--font-size-sm);
}

.detail-loading {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  color: var(--color-text-secondary);
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
  .rate-field,
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
