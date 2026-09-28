<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { RefreshCw } from 'lucide-vue-next'
import { getCustomNodeEntries, refreshCustomNode } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, CustomNodeEntries, CustomNodeEntry } from '@/api/types'
import DataTable, { type Column } from '@/components/DataTable.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import { formatDateTime } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    open: boolean
    node?: CustomNode | null
    /** When true the dialog force-refreshes the subscription on open. */
    refreshOnOpen?: boolean
  }>(),
  { node: null, refreshOnOpen: false },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'refreshed'): void
}>()

type EntryRow = CustomNodeEntry & { row_key: string }

const columns: Column[] = [
  { key: 'name', label: '名称', width: '220px' },
  { key: 'type', label: '协议', width: '90px' },
  { key: 'server', label: '服务器', width: '180px' },
  { key: 'port', label: '端口', width: '80px', align: 'right' },
]

const result = ref<CustomNodeEntries | null>(null)
const loading = ref(false)
const refreshing = ref(false)
const error = ref('')

const isSubscription = computed(() => props.node?.source_type === 'subscription')
const notFetched = computed(
  () => isSubscription.value && result.value !== null && !result.value.has_cache,
)

// DataTable forbids index keys; disambiguate exact duplicates from upstream.
const entryRows = computed<EntryRow[]>(() => {
  const seen = new Map<string, number>()
  return (result.value?.entries ?? []).map((entry) => {
    const base = `${entry.name}|${entry.type}|${entry.server}|${entry.port}`
    const count = (seen.get(base) ?? 0) + 1
    seen.set(base, count)
    return { ...entry, row_key: count > 1 ? `${base}#${count}` : base }
  })
})

async function loadEntries(preserveError = false) {
  const node = props.node
  if (!node) return
  loading.value = true
  if (!preserveError) error.value = ''
  try {
    result.value = await getCustomNodeEntries(node.id)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function doRefresh() {
  const node = props.node
  if (!node) return
  refreshing.value = true
  error.value = ''
  try {
    result.value = await refreshCustomNode(node.id)
    emit('refreshed')
  } catch (err) {
    error.value = errorMessage(err)
    // A failed refresh keeps the old cache: show it without discarding the
    // refresh error, so the admin sees why the upstream fetch failed.
    if (result.value === null) await loadEntries(true)
  } finally {
    refreshing.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    result.value = null
    error.value = ''
    if (props.refreshOnOpen) void doRefresh()
    else void loadEntries()
  },
)
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="`节点列表 · ${props.node?.name ?? ''}`"
    :subtitle="
      isSubscription
        ? result?.has_cache
          ? `上游订阅 · 缓存于 ${formatDateTime(result.fetched_at)}`
          : '上游订阅 · 尚未拉取'
        : '分享链接 · 本地解析，不联网'
    "
    :width="760"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="loading || (refreshing && result === null)"
      class="loading"
    >
      <LoadingSpinner />
    </div>

    <template v-else-if="notFetched">
      <EmptyState
        title="未拉取，请先更新订阅"
        hint="查看节点不会触发上游请求；点击“更新订阅”强制拉取并刷新缓存。"
      />
    </template>

    <template v-else>
      <div class="entries-scroll">
        <DataTable
          :columns="columns"
          :rows="entryRows"
          :row-key="(row) => row.row_key"
          aria-label="自定义节点条目"
        >
          <template #cell-type="{ row }">
            <span class="chip">{{ row.type }}</span>
          </template>
          <template #empty>
            {{ isSubscription ? '订阅已缓存但未解析出节点' : '未解析出节点' }}
          </template>
        </DataTable>
      </div>

      <div
        v-if="result?.skipped && result.skipped.length > 0"
        class="skipped"
        role="alert"
      >
        <strong>以下条目无法解析，已被跳过（不影响其余节点）：</strong>
        <ul>
          <li
            v-for="(item, index) in result.skipped"
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
      <button
        v-if="isSubscription"
        type="button"
        class="btn"
        :class="{ 'is-loading': refreshing }"
        :disabled="refreshing"
        @click="doRefresh"
      >
        <LoadingSpinner
          v-if="refreshing"
          size="sm"
        />
        <RefreshCw
          v-else
          :size="15"
        />
        {{ refreshing ? '更新中…' : '更新订阅' }}
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

.entries-scroll {
  max-height: 360px;
  overflow-y: auto;
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
