<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import { formatDate, formatRemaining, isoToLocalInput, localInputToIso } from '@/utils/format'

const props = defineProps<{
  user: UserDetail
}>()

const emit = defineEmits<{
  (e: 'updated', user: UserDetail): void
}>()

const editing = ref(false)
const noExpiry = ref(false)
const expiresInput = ref('')
const saving = ref(false)
const error = ref('')

watch(editing, (editingNow) => {
  if (!editingNow) return
  error.value = ''
  noExpiry.value = props.user.expires_at === null
  expiresInput.value = isoToLocalInput(props.user.expires_at)
})

const remaining = computed(() => formatRemaining(props.user.expires_at))

const remainingTone = computed<'normal' | 'warning' | 'danger'>(() => {
  if (!props.user.expires_at) return 'normal'
  const time = new Date(props.user.expires_at).getTime()
  if (Number.isNaN(time)) return 'normal'
  const diff = time - Date.now()
  if (diff <= 0) return 'danger'
  if (diff <= 3 * 24 * 3600 * 1000) return 'warning'
  return 'normal'
})

async function save() {
  const iso = noExpiry.value ? null : localInputToIso(expiresInput.value)
  if (!noExpiry.value && iso === null) {
    error.value = '请选择有效的到期时间，或勾选永不过期'
    return
  }
  saving.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, { expires_at: iso })
    editing.value = false
    emit('updated', updated)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card compact">
    <div class="card-head">
      <h2 class="card-title">
        有效期
      </h2>
      <button
        type="button"
        class="btn secondary small"
        @click="editing = !editing"
      >
        {{ editing ? '取消编辑' : '修改到期时间' }}
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="!editing"
      class="expiry-view"
    >
      <div
        class="expiry-headline"
        :class="remainingTone"
      >
        {{ remaining }}
      </div>
      <div class="expiry-rows">
        <div class="expiry-row">
          <span class="expiry-label text-secondary">开始时间</span>
          <strong>{{ props.user.started_at ? formatDate(props.user.started_at) : '未设置' }}</strong>
        </div>
        <div class="expiry-row">
          <span class="expiry-label text-secondary">到期时间</span>
          <strong>{{ props.user.expires_at ? formatDate(props.user.expires_at) : '不设到期时间' }}</strong>
        </div>
      </div>
    </div>

    <div
      v-else
      class="expiry-editor"
    >
      <label class="checkbox-label">
        <input
          v-model="noExpiry"
          type="checkbox"
        >
        永不过期
      </label>
      <input
        v-if="!noExpiry"
        v-model="expiresInput"
        type="datetime-local"
      >
      <button
        type="button"
        class="btn small"
        :disabled="saving"
        @click="save"
      >
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.expiry-view {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}

.expiry-headline {
  font-size: 24px;
  font-weight: 700;
  line-height: 1.2;
}

.expiry-headline.warning {
  color: var(--color-warning);
}

.expiry-headline.danger {
  color: var(--color-danger);
}

.expiry-rows {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0;
  border-top: 1px solid var(--color-border);
}

.expiry-row {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
  padding: var(--spacing-sm) var(--spacing-md) 0 0;
}

.expiry-row + .expiry-row {
  padding-left: var(--spacing-md);
  border-left: 1px solid var(--color-border);
}

.expiry-label {
  font-size: var(--font-size-xs);
}

.expiry-row strong {
  overflow: hidden;
  font-size: var(--font-size-md);
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.expiry-editor {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-top: var(--spacing-md);
}
@media (max-width: 560px) {
  .expiry-rows {
    grid-template-columns: minmax(0, 1fr);
  }

  .expiry-row + .expiry-row {
    padding-left: 0;
    border-top: 1px solid var(--color-border);
    border-left: 0;
  }
}
</style>
