<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getCustomNodeEntries } from '@/api/customNodes'
import { errorMessage } from '@/api/http'
import type { CustomNode, CustomNodeEntry } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import EmptyState from '@/components/ui/EmptyState.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import SegmentedControl from '@/components/ui/SegmentedControl.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    node?: CustomNode | null
    /** Current entry whitelist; empty means "all entries". */
    selectedKeys?: string[]
  }>(),
  { node: null, selectedKeys: () => [] },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'confirm', entryKeys: string[]): void
}>()

type Scope = 'all' | 'custom'

const scopeOptions: Array<{ value: Scope; label: string }> = [
  { value: 'all', label: '全部线路' },
  { value: 'custom', label: '仅所选线路' },
]

const entries = ref<CustomNodeEntry[]>([])
const hasCache = ref(false)
const loaded = ref(false)
const loading = ref(false)
const error = ref('')
const scope = ref<Scope>('all')
const selected = ref<string[]>([])

// Unparseable links have no key and are absent from entries; entries with
// identical connection params collapse onto one key, so dedupe by key.
const visibleEntries = computed(() => {
  const seen = new Set<string>()
  const out: CustomNodeEntry[] = []
  for (const entry of entries.value) {
    if (seen.has(entry.key)) continue
    seen.add(entry.key)
    out.push(entry)
  }
  return out
})

const isSubscription = computed(() => props.node?.source_type === 'subscription')
const notFetched = computed(
  () => loaded.value && isSubscription.value && !hasCache.value,
)
// A stale key (authorized before upstream changed the connection params) is
// kept inert by the backend. It must not satisfy the "select at least one"
// rule, otherwise confirming would save a whitelist that matches nothing and
// silently hides the source.
const visibleKeys = computed(() => new Set(visibleEntries.value.map((entry) => entry.key)))
const selectedVisibleCount = computed(
  () => selected.value.filter((key) => visibleKeys.value.has(key)).length,
)
const canConfirm = computed(() => scope.value === 'all' || selectedVisibleCount.value > 0)

async function load() {
  const node = props.node
  if (!node) return
  loading.value = true
  error.value = ''
  try {
    const result = await getCustomNodeEntries(node.id)
    entries.value = result.entries
    hasCache.value = result.has_cache
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
    loaded.value = true
  }
}

function toggleEntry(key: string, checked: boolean) {
  selected.value = checked
    ? [...selected.value, key]
    : selected.value.filter((item) => item !== key)
}

function confirm() {
  if (!canConfirm.value) return
  emit('confirm', scope.value === 'all' ? [] : [...selected.value])
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    error.value = ''
    loaded.value = false
    hasCache.value = false
    entries.value = []
    selected.value = [...props.selectedKeys]
    scope.value = props.selectedKeys.length > 0 ? 'custom' : 'all'
    void load()
  },
)
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="`线路授权 · ${props.node?.name ?? ''}`"
    subtitle="仅所选线路时，该用户订阅只输出勾选的线路；全部线路则不受白名单限制。"
    :width="560"
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

    <template v-else-if="notFetched">
      <EmptyState
        title="未拉取，请先更新订阅"
        hint="查看线路不会触发上游请求；请先在自定义节点列表点击“更新订阅”拉取缓存。"
      />
    </template>

    <template v-else-if="visibleEntries.length === 0">
      <EmptyState
        title="没有可授权的线路"
        hint="该来源未解析出可单独授权的线路；无法解析的分享链接不能单独授权。"
      />
    </template>

    <template v-else>
      <SegmentedControl
        v-model="scope"
        :items="scopeOptions"
        aria-label="授权范围"
      />
      <div
        v-if="scope === 'custom'"
        class="entry-list"
      >
        <label
          v-for="entry in visibleEntries"
          :key="entry.key"
          class="entry-item"
          :class="{ checked: selected.includes(entry.key) }"
        >
          <input
            type="checkbox"
            :checked="selected.includes(entry.key)"
            @change="toggleEntry(entry.key, ($event.target as HTMLInputElement).checked)"
          >
          <span class="name">{{ entry.name }}</span>
          <span class="meta chip">{{ entry.type }} · {{ entry.server }}:{{ entry.port }}</span>
        </label>
      </div>
      <p
        v-if="scope === 'custom' && selectedVisibleCount === 0"
        class="hint"
        role="alert"
      >
        请至少选择一条线路
      </p>
    </template>

    <template #footer>
      <button
        type="button"
        class="btn secondary"
        @click="emit('close')"
      >
        取消
      </button>
      <button
        type="button"
        class="btn"
        :disabled="!canConfirm"
        @click="confirm"
      >
        确定
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

.entry-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  max-height: 320px;
  overflow-y: auto;
  margin-top: var(--spacing-md);
}

.entry-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  cursor: pointer;
  transition: border-color 0.15s ease, background 0.15s ease;
}

.entry-item:hover {
  border-color: var(--color-border-strong);
}

.entry-item.checked {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.entry-item .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.entry-item .meta {
  white-space: nowrap;
}

.hint {
  margin: var(--spacing-sm) 0 0;
  color: var(--color-danger);
  font-size: var(--font-size-sm);
}
</style>
