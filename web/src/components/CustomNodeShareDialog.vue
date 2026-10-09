<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getCustomNodeShare } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, CustomNodeShare } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'
import { copyText } from '@/utils/clipboard'
import { formatDateTime } from '@/utils/format'
import { customNodeSourceLabel } from '@/utils/labels'

const props = withDefaults(
  defineProps<{
    open: boolean
    node?: CustomNode | null
  }>(),
  { node: null },
)

const emit = defineEmits<{
  (e: 'close'): void
}>()

type ShareFormat = 'clash' | 'v2'

const formatOptions: Array<{ value: ShareFormat; label: string }> = [
  { value: 'clash', label: 'Clash' },
  { value: 'v2', label: 'V2 原生' },
]

const format = ref<ShareFormat>('clash')
const share = ref<CustomNodeShare | null>(null)
const loading = ref(false)
const error = ref('')
const copiedFormat = ref<ShareFormat | ''>('')
let shareRequest = 0

const isSubscription = computed(() => props.node?.source_type === 'subscription')

const subtitle = computed(() => {
  if (!props.node) return undefined
  const base = customNodeSourceLabel(props.node.source_type)
  if (!isSubscription.value) return `${base} · 本地解析，不联网`
  return share.value?.has_cache
    ? `${base} · 缓存于 ${formatDateTime(share.value.fetched_at)}`
    : `${base} · 尚未拉取`
})

const clashText = computed(() => share.value?.clash ?? '')
const linksText = computed(() => (share.value?.links ?? []).join('\n'))
const currentText = computed(() => (format.value === 'clash' ? clashText.value : linksText.value))
const currentTitle = computed(() =>
  format.value === 'clash' ? 'Clash（proxies 片段）' : 'V2 原生（明文链接）',
)
const isEmpty = computed(() => {
  if (format.value === 'clash') {
    const text = clashText.value.trim()
    return text === '' || text === 'proxies: []'
  }
  return currentText.value.trim() === ''
})
const emptyTitle = computed(() =>
  format.value === 'clash' ? '没有可转换的 Clash 节点' : '没有可输出的明文链接',
)
const emptyHint = computed(() =>
  format.value === 'clash'
    ? '该来源没有可转换为 Clash 代理的条目。'
    : '该来源没有明文分享链接，上游可能仅提供 Clash 配置。',
)
const skipped = computed(() => share.value?.skipped ?? [])

watch(
  () => [props.open, props.node?.id] as const,
  ([open]) => {
    shareRequest++
    format.value = 'clash'
    share.value = null
    loading.value = false
    error.value = ''
    copiedFormat.value = ''
    if (open) void loadShare()
  },
  { flush: 'sync' },
)

async function loadShare() {
  const node = props.node
  if (!props.open || !node) return
  const request = ++shareRequest
  const current = () => request === shareRequest && props.open && props.node?.id === node.id
  loading.value = true
  error.value = ''
  share.value = null
  try {
    const result = await getCustomNodeShare(node.id)
    if (!current()) return
    share.value = result
  } catch (err) {
    if (current()) error.value = errorMessage(err)
  } finally {
    if (current()) loading.value = false
  }
}

async function copyCurrent() {
  if (isEmpty.value) return
  const target = format.value
  if (!(await copyText(currentText.value))) return
  copiedFormat.value = target
  setTimeout(() => {
    if (copiedFormat.value === target) copiedFormat.value = ''
  }, 1500)
}
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="`分享 · ${props.node?.name ?? '自定义节点'}`"
    :subtitle="subtitle"
    :width="640"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="loading"
      class="loading"
    >
      <LoadingSpinner />
    </div>

    <template v-else-if="share">
      <SegmentedControl
        v-model="format"
        :items="formatOptions"
        aria-label="分享格式"
      />

      <div class="format-block">
        <div class="format-head">
          <span class="format-title">{{ currentTitle }}</span>
          <button
            type="button"
            class="btn small"
            :disabled="isEmpty"
            @click="copyCurrent"
          >
            {{ copiedFormat === format ? '已复制' : '复制' }}
          </button>
        </div>
        <pre
          v-if="!isEmpty"
          class="code-block mono"
        >{{ currentText }}</pre>
        <EmptyState
          v-else
          :title="emptyTitle"
          :hint="emptyHint"
        />
      </div>

      <div
        v-if="skipped.length > 0"
        class="skipped"
        role="alert"
      >
        <strong>以下条目无法转换为 Clash 节点，已跳过（不影响其余条目）：</strong>
        <ul>
          <li
            v-for="(item, index) in skipped"
            :key="`${index}-${item}`"
            class="mono"
          >
            {{ item }}
          </li>
        </ul>
      </div>
    </template>

    <template #footer>
      <button
        type="button"
        class="btn secondary"
        @click="emit('close')"
      >
        关闭
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.loading {
  display: flex;
  justify-content: center;
  padding: var(--spacing-xl) 0;
}

.format-block {
  margin-top: var(--spacing-md);
}

.format-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}

.format-title {
  font-size: var(--font-size-sm);
  font-weight: 600;
  color: var(--color-text-secondary);
}

.code-block {
  max-height: 320px;
  margin: 0;
  overflow: auto;
  padding: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  line-height: 1.5;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.skipped {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
  padding: var(--spacing-md);
  border: 1px solid var(--color-warning-border);
  border-radius: var(--radius-md);
  background: var(--color-warning-soft);
  color: var(--color-warning);
  font-size: var(--font-size-sm);
}

.skipped ul {
  margin: 0;
  padding-left: var(--spacing-lg);
  color: var(--color-text);
}

.skipped li {
  word-break: break-all;
}
</style>
