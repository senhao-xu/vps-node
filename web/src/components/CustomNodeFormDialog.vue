<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createCustomNode, updateCustomNode } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, CustomNodeSourceType } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import {
  CUSTOM_USER_AGENT_CHOICE,
  DEFAULT_CUSTOM_NODE_USER_AGENT,
  USER_AGENT_PRESETS,
  isUserAgentPreset,
} from '@/utils/customNodeUserAgents'

const props = withDefaults(
  defineProps<{
    open: boolean
    node?: CustomNode | null
  }>(),
  { node: null },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const name = ref('')
const sourceType = ref<CustomNodeSourceType>('links')
const content = ref('')
const status = ref<'active' | 'disabled'>('active')
const userAgentChoice = ref<string>(DEFAULT_CUSTOM_NODE_USER_AGENT)
const customUserAgent = ref('')
const insecureSkipVerify = ref(false)

const submitting = ref(false)
const error = ref('')
const savedWarnings = ref<string[]>([])

const isEdit = computed(() => props.node !== null)
const title = computed(() => (isEdit.value ? '编辑自定义节点' : '新建自定义节点'))

const sourceOptions: Array<{ value: CustomNodeSourceType; label: string }> = [
  { value: 'links', label: '分享链接' },
  { value: 'subscription', label: '订阅链接' },
]

const userAgentOptions = computed(() => [
  ...USER_AGENT_PRESETS,
  { value: CUSTOM_USER_AGENT_CHOICE, label: '自定义…' },
])

const effectiveUserAgent = computed(() => {
  if (userAgentChoice.value === CUSTOM_USER_AGENT_CHOICE) return customUserAgent.value.trim()
  return userAgentChoice.value
})

watch(
  () => props.open,
  (open) => {
    if (!open) return
    error.value = ''
    savedWarnings.value = []
    name.value = props.node?.name ?? ''
    sourceType.value = props.node?.source_type ?? 'links'
    content.value = ''
    status.value = props.node?.status === 'disabled' ? 'disabled' : 'active'
    const userAgent = props.node?.user_agent ?? ''
    if (userAgent === '' || isUserAgentPreset(userAgent)) {
      userAgentChoice.value = userAgent === '' ? DEFAULT_CUSTOM_NODE_USER_AGENT : userAgent
      customUserAgent.value = ''
    } else {
      userAgentChoice.value = CUSTOM_USER_AGENT_CHOICE
      customUserAgent.value = userAgent
    }
    insecureSkipVerify.value = props.node?.insecure_skip_verify ?? false
  },
)

const contentLabel = computed(() => {
  if (sourceType.value === 'subscription') return '上游订阅 URL'
  return isEdit.value ? '分享链接（留空保持不变）' : '分享链接'
})

const contentPlaceholder = computed(() => {
  if (sourceType.value === 'subscription') return 'https://example.com/subscribe'
  if (isEdit.value) return '留空则保持现有链接不变'
  return '每行一条分享链接，支持 ss / vless / hysteria2 / anytls / http / https / trojan / vmess'
})

const validationMessage = computed(() => {
  const trimmed = name.value.trim()
  if (!trimmed || trimmed.length > 128) return '名称必填，最长 128 个字符'
  if (!isEdit.value && !content.value.trim()) return '请填写内容'
  if (sourceType.value === 'subscription' && content.value.trim()) {
    const raw = content.value.trim()
    if (!/^https?:\/\//i.test(raw)) return '订阅 URL 必须是 http 或 https 地址'
    const userAgent = effectiveUserAgent.value
    if (userAgent.length > 255) return 'User-Agent 最长 255 个字符'
    for (const ch of userAgent) {
      const code = ch.codePointAt(0) ?? 0
      if (code < 0x20 || code === 0x7f) return 'User-Agent 不能包含控制字符'
    }
  }
  return ''
})

async function submit() {
  if (savedWarnings.value.length > 0) {
    emit('close')
    return
  }
  const problem = validationMessage.value
  if (problem) {
    error.value = problem
    return
  }
  submitting.value = true
  error.value = ''
  try {
    const trimmedContent = content.value.trim()
    const result =
      isEdit.value && props.node
        ? await updateCustomNode(props.node.id, {
            name: name.value.trim(),
            status: status.value,
            ...(trimmedContent ? { content: trimmedContent } : {}),
            ...(sourceType.value === 'subscription'
              ? { user_agent: effectiveUserAgent.value, insecure_skip_verify: insecureSkipVerify.value }
              : {}),
          })
        : await createCustomNode({
            name: name.value.trim(),
            source_type: sourceType.value,
            content: trimmedContent,
            ...(sourceType.value === 'subscription'
              ? { user_agent: effectiveUserAgent.value, insecure_skip_verify: insecureSkipVerify.value }
              : {}),
          })
    emit('saved')
    if (result.warnings && result.warnings.length > 0) {
      savedWarnings.value = result.warnings
      return
    }
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
    :subtitle="
      isEdit
        ? '修改名称、状态或替换内容；内容加密存储且不回显'
        : '自定义节点并入已授权用户的订阅输出，不参与流量统计'
    "
    :width="520"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="savedWarnings.length > 0"
      class="warnings"
      role="alert"
    >
      <strong>已保存，但以下条目无法转换为 Clash 节点（general 订阅仍会原样输出）：</strong>
      <ul>
        <li
          v-for="warning in savedWarnings"
          :key="warning"
          class="mono"
        >
          {{ warning }}
        </li>
      </ul>
    </div>

    <div
      v-else
      class="form"
    >
      <div class="field">
        <label for="custom-node-name">名称</label>
        <input
          id="custom-node-name"
          v-model="name"
          type="text"
          placeholder="例如 机场备用节点"
        >
      </div>

      <div class="field">
        <label id="custom-node-source-label">来源类型</label>
        <span
          v-if="isEdit"
          class="chip"
        >{{ sourceOptions.find((o) => o.value === sourceType)?.label }}</span>
        <SegmentedControl
          v-else
          v-model="sourceType"
          :items="sourceOptions"
          aria-label="来源类型"
        />
        <p class="field-hint">
          <template v-if="sourceType === 'links'">
            逐条粘贴分享链接，逐行一条。
          </template>
          <template v-else>
            订阅渲染时拉取上游内容，带 5 分钟缓存；上游不可达时使用旧缓存。
          </template>
        </p>
      </div>

      <div class="field">
        <label for="custom-node-content">{{ contentLabel }}</label>
        <textarea
          id="custom-node-content"
          v-model="content"
          :rows="sourceType === 'links' ? 6 : 2"
          :placeholder="contentPlaceholder"
        />
        <p
          v-if="isEdit"
          class="field-hint"
        >
          内容不回显；留空表示保持不变。替换订阅 URL 会立即丢弃旧缓存。
        </p>
      </div>

      <div
        v-if="sourceType === 'subscription'"
        class="field"
      >
        <label id="custom-node-user-agent-label">上游 User-Agent</label>
        <AppSelect
          v-model="userAgentChoice"
          :options="userAgentOptions"
          label="上游 User-Agent"
        />
        <input
          v-if="userAgentChoice === CUSTOM_USER_AGENT_CHOICE"
          v-model="customUserAgent"
          type="text"
          maxlength="255"
          placeholder="留空使用默认 Clash UA"
        >
        <p class="field-hint">
          部分上游按 User-Agent 返回不同内容；留空使用默认
          <code>{{ DEFAULT_CUSTOM_NODE_USER_AGENT }}</code>。修改后会清空该节点的上游缓存。
        </p>
      </div>

      <div
        v-if="sourceType === 'subscription'"
        class="toggle-row"
      >
        <div class="toggle-row-text">
          <span class="toggle-row-label">跳过证书校验</span>
          <span class="toggle-row-desc">上游使用自签名证书时启用；会关闭 TLS 校验，存在中间人风险</span>
        </div>
        <ToggleSwitch
          :model-value="insecureSkipVerify"
          label="跳过上游证书校验"
          @update:model-value="insecureSkipVerify = $event"
        />
      </div>

      <div
        v-if="isEdit"
        class="toggle-row"
      >
        <div class="toggle-row-text">
          <span class="toggle-row-label">启用节点</span>
          <span class="toggle-row-desc">停用后立即从所有用户订阅中消失</span>
        </div>
        <ToggleSwitch
          :model-value="status === 'active'"
          label="启用自定义节点"
          @update:model-value="status = $event ? 'active' : 'disabled'"
        />
      </div>
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
        {{ savedWarnings.length > 0 ? '完成' : submitting ? '保存中…' : '保存' }}
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

.field-hint {
  margin: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  line-height: 1.5;
}

.warnings {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  padding: var(--spacing-md);
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius-md);
  background: var(--color-warning-soft);
  color: var(--color-warning);
  font-size: var(--font-size-sm);
}

.warnings ul {
  margin: 0;
  padding-left: var(--spacing-lg);
  color: var(--color-text);
}

.warnings li {
  word-break: break-all;
}
</style>
