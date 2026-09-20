<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail } from '@/api/types'
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
  <div class="card">
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
    <p
      v-if="error"
      class="text-danger form-error"
    >
      {{ error }}
    </p>

    <div
      v-if="!editing"
      class="expiry-view"
    >
      <div class="expiry-row">
        <span class="text-secondary">开始时间</span>
        <span>{{ props.user.started_at ? formatDate(props.user.started_at) : '—' }}</span>
      </div>
      <div class="expiry-row">
        <span class="text-secondary">到期时间</span>
        <span>
          {{ props.user.expires_at ? formatDate(props.user.expires_at) : '永不过期' }}
          <span class="text-secondary remaining">（{{ remaining }}）</span>
        </span>
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
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.card-head .card-title {
  margin-bottom: 0;
}

.form-error {
  margin: var(--spacing-sm) 0;
  font-size: var(--font-size-sm);
}

.expiry-view {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
}

.expiry-row {
  display: flex;
  gap: var(--spacing-md);
}

.remaining {
  font-size: var(--font-size-sm);
}

.expiry-editor {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-top: var(--spacing-md);
}
</style>
