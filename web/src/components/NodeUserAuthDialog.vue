<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { getNodeUsers, putNodeUsers } from '@/api/nodes'
import { errorMessage } from '@/api/http'
import type { NodeBrief, User } from '@/api/types'
import EmptyState from '@/components/ui/EmptyState.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { displayUserStatus } from '@/utils/labels'
import { formatDateTime } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    open: boolean
    node?: NodeBrief | null
  }>(),
  { node: null },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const users = ref<User[]>([])
const authorizedIds = ref<number[]>([])
const selected = ref<number[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const savedTip = ref(false)

const selectedCount = computed(() => selected.value.length)
const allSelected = computed(
  () => users.value.length > 0 && selected.value.length === users.value.length,
)
const someSelected = computed(() => selected.value.length > 0 && !allSelected.value)
const dirty = computed(() => !sameIds(selected.value, authorizedIds.value))

function sameIds(a: number[], b: number[]): boolean {
  return a.length === b.length && [...a].sort().join(',') === [...b].sort().join(',')
}

function toggleAll(checked: boolean) {
  selected.value = checked ? users.value.map((u) => u.id) : []
  savedTip.value = false
}

function toggleUser(user: User, checked: boolean) {
  selected.value = checked
    ? [...selected.value, user.id]
    : selected.value.filter((id) => id !== user.id)
  savedTip.value = false
}

async function load() {
  const node = props.node
  if (!node) return
  loading.value = true
  error.value = ''
  try {
    const result = await getNodeUsers(node.id)
    users.value = result.users
    authorizedIds.value = [...result.user_ids]
    selected.value = [...result.user_ids]
    savedTip.value = false
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function save() {
  const node = props.node
  if (!node || !dirty.value) return
  saving.value = true
  error.value = ''
  try {
    const result = await putNodeUsers(node.id, selected.value)
    authorizedIds.value = [...result.user_ids]
    selected.value = [...result.user_ids]
    savedTip.value = true
    emit('saved')
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}

watch(
  () => props.open,
  (open) => {
    if (!open) return
    users.value = []
    authorizedIds.value = []
    selected.value = []
    void load()
  },
)
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="`节点用户授权 · ${props.node?.name ?? ''}`"
    subtitle="勾选可访问该节点的用户；保存后立即生效，Agent 将在下次同步时应用。"
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

    <template v-else-if="users.length === 0">
      <EmptyState
        title="还没有用户"
        hint="先在用户页创建用户，再回到这里授权。"
      />
    </template>

    <template v-else>
      <div class="list-head">
        <label class="toggle-all">
          <input
            type="checkbox"
            :checked="allSelected"
            :indeterminate="someSelected"
            aria-label="全选用户"
            @change="toggleAll(($event.target as HTMLInputElement).checked)"
          >
          <span>全选</span>
        </label>
        <span class="count chip">已选 {{ selectedCount }} / {{ users.length }}</span>
        <span
          v-if="savedTip"
          class="saved-tip"
        >已保存</span>
      </div>
      <div class="user-list">
        <label
          v-for="user in users"
          :key="user.id"
          class="user-item"
          :class="{ checked: selected.includes(user.id) }"
        >
          <input
            type="checkbox"
            :checked="selected.includes(user.id)"
            @change="toggleUser(user, ($event.target as HTMLInputElement).checked)"
          >
          <span
            class="name"
            :title="user.username"
          >{{ user.username }}</span>
          <StatusBadge
            :label="displayUserStatus(user).label"
            :tone="displayUserStatus(user).tone"
          />
          <span
            v-if="user.expires_at"
            class="meta chip"
          >{{ formatDateTime(user.expires_at) }} 到期</span>
          <span class="meta chip">{{ user.node_count }} 节点</span>
        </label>
      </div>
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
        :disabled="!dirty || saving"
        :title="dirty ? '' : '勾选变化后可保存'"
        @click="save"
      >
        {{ saving ? '保存中…' : '保存授权' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.loading {
  display: flex;
  justify-content: center;
  padding: var(--spacing-lg) 0;
}

.list-head {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-sm);
}

.toggle-all {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
}

.count {
  white-space: nowrap;
}

.saved-tip {
  margin-left: auto;
  color: var(--color-success);
  font-size: var(--font-size-sm);
}

.user-list {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.user-item {
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

.user-item:hover {
  border-color: var(--color-border-strong);
}

.user-item.checked {
  border-color: var(--color-primary);
  background: var(--color-primary-soft);
}

.user-item .name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 500;
}

.user-item .meta {
  white-space: nowrap;
}

@media (max-width: 560px) {
  .user-item {
    flex-wrap: wrap;
  }

  .saved-tip {
    margin-left: 0;
  }
}
</style>
