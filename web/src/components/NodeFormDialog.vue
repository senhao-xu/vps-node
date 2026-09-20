<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createNode, updateNode } from '@/api/nodes'
import { errorMessage } from '@/api/http'
import type { NodeBrief, NodeSettingsInput, Protocol } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import { SHADOWSOCKS_METHODS, protocolLabel } from '@/utils/labels'

const props = withDefaults(
  defineProps<{
    open: boolean
    serverId: number
    node?: NodeBrief | null
  }>(),
  { node: null },
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
const ssPassword = ref('')

const vlessPrivateKey = ref('')
const vlessShortId = ref('')
const vlessServerNames = ref('')

const hy2Password = ref('')
const hy2Up = ref<number | null>(null)
const hy2Down = ref<number | null>(null)

const submitting = ref(false)
const error = ref('')
const isEdit = ref(false)

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
    ssPassword.value = ''
    vlessPrivateKey.value = ''
    vlessShortId.value = ''
    vlessServerNames.value = ''
    hy2Password.value = ''
    hy2Up.value = null
    hy2Down.value = null
  },
)

const title = computed(() => (isEdit.value ? '编辑节点' : '添加节点'))

const settingsPayload = computed<NodeSettingsInput | undefined>(() => {
  const payload: NodeSettingsInput = {}
  if (protocol.value === 'shadowsocks') {
    if (!isEdit.value || ssMethodTouched.value) payload['method'] = ssMethod.value
    if (ssPassword.value) payload['password'] = ssPassword.value
  } else if (protocol.value === 'vless') {
    if (vlessPrivateKey.value) payload['private_key'] = vlessPrivateKey.value
    if (vlessShortId.value) payload['short_id'] = vlessShortId.value
    const names = vlessServerNames.value
      .split(/[,，\s]+/)
      .map((item) => item.trim())
      .filter((item) => item.length > 0)
    if (names.length > 0) payload['server_names'] = names
  } else {
    if (hy2Password.value) payload['password'] = hy2Password.value
    if (hy2Up.value !== null && !Number.isNaN(hy2Up.value) && hy2Up.value >= 0) {
      payload['up_mbps'] = hy2Up.value
    }
    if (hy2Down.value !== null && !Number.isNaN(hy2Down.value) && hy2Down.value >= 0) {
      payload['down_mbps'] = hy2Down.value
    }
  }
  if (Object.keys(payload).length === 0) return undefined
  return payload
})

const validationMessage = computed(() => {
  if (!name.value.trim()) return '请填写节点名称'
  const portValue = port.value
  if (portValue === null || !Number.isInteger(portValue) || portValue < 1 || portValue > 65535) {
    return '端口必须是 1-65535 的整数'
  }
  if (!isEdit.value && settingsPayload.value === undefined) {
    return '请填写协议所需的配置'
  }
  return ''
})

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
        server_id: props.serverId,
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
    :width="520"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="form">
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
          class="field"
          style="max-width: 140px"
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
        <div class="field">
          <label for="node-protocol">协议</label>
          <select
            id="node-protocol"
            v-model="protocol"
            :disabled="isEdit"
          >
            <option value="shadowsocks">
              {{ protocolLabel('shadowsocks') }}
            </option>
            <option value="vless">
              {{ protocolLabel('vless') }}
            </option>
            <option value="hysteria2">
              {{ protocolLabel('hysteria2') }}
            </option>
          </select>
        </div>
        <div
          v-if="isEdit"
          class="field"
        >
          <label for="node-status">状态</label>
          <select
            id="node-status"
            v-model="status"
          >
            <option value="active">
              启用
            </option>
            <option value="disabled">
              停用
            </option>
          </select>
        </div>
      </div>

      <div
        v-if="protocol === 'shadowsocks'"
        class="settings-box"
      >
        <div class="field">
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
        <div class="field">
          <label for="ss-password">
            密码{{ isEdit ? '（留空保持不变）' : '' }}
          </label>
          <input
            id="ss-password"
            v-model="ssPassword"
            type="password"
            autocomplete="new-password"
            :placeholder="isEdit ? '留空保持不变' : '密码'"
          >
        </div>
      </div>

      <div
        v-else-if="protocol === 'vless'"
        class="settings-box"
      >
        <div class="field">
          <label for="vless-key">Reality 私钥{{ isEdit ? '（留空保持不变）' : '' }}</label>
          <input
            id="vless-key"
            v-model="vlessPrivateKey"
            type="password"
            autocomplete="new-password"
            :placeholder="isEdit ? '留空保持不变' : 'private_key'"
          >
        </div>
        <div class="form-row">
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
            <label for="vless-names">Server Names（逗号分隔）</label>
            <input
              id="vless-names"
              v-model="vlessServerNames"
              type="text"
              placeholder="例如 example.com,www.example.com"
            >
          </div>
        </div>
      </div>

      <div
        v-else
        class="settings-box"
      >
        <div class="field">
          <label for="hy2-password">
            密码{{ isEdit ? '（留空保持不变）' : '' }}
          </label>
          <input
            id="hy2-password"
            v-model="hy2Password"
            type="password"
            autocomplete="new-password"
            :placeholder="isEdit ? '留空保持不变' : '密码'"
          >
        </div>
        <div class="form-row">
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
      </div>

      <p class="text-secondary tip">
        协议密钥仅提交一次，由 Panel 加密保存，不会回显；编辑时留空即保持不变。协议配置在提交时整体替换，编辑节点请仅填写需要修改的配置项。
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
        :disabled="submitting"
        @click="submit"
      >
        {{ submitting ? '保存中…' : '保存' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.settings-box {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: var(--spacing-md);
}

.tip {
  margin: 0;
  font-size: var(--font-size-sm);
}
</style>
