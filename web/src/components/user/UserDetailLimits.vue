<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{
  user: UserDetail
}>()

const emit = defineEmits<{
  (e: 'updated', user: UserDetail): void
}>()

const editing = ref(false)
const speedLimit = ref<number | null>(0)
const deviceLimit = ref<number | null>(0)
const saving = ref(false)
const error = ref('')

function validLimit(value: number | null): value is number {
  return value !== null && Number.isInteger(value) && value >= 0
}

watch(
  () => props.user.id,
  () => {
    editing.value = false
  },
)

watch(editing, (editingNow) => {
  if (!editingNow) return
  error.value = ''
  speedLimit.value = props.user.speed_limit
  deviceLimit.value = props.user.device_limit
})

const speedText = computed(() =>
  props.user.speed_limit > 0 ? `${props.user.speed_limit} Mbps` : '不限速',
)

const deviceText = computed(() =>
  props.user.device_limit > 0 ? `${props.user.device_limit} 台` : '不限制',
)

async function save() {
  if (!validLimit(speedLimit.value) || !validLimit(deviceLimit.value)) {
    error.value = '限速与设备数必须是不小于 0 的整数（0 表示不限制）'
    return
  }
  saving.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, {
      speed_limit: speedLimit.value,
      device_limit: deviceLimit.value,
    })
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
  <div>
    <div class="card-head">
      <h2 class="card-title">
        连接概况
      </h2>
      <button
        type="button"
        class="btn secondary small"
        @click="editing = !editing"
      >
        {{ editing ? '取消编辑' : '修改限制' }}
      </button>
    </div>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="!editing"
      class="info-grid limits-grid"
    >
      <div class="info-item">
        <span class="info-label">限速</span>
        <strong>{{ speedText }}</strong>
      </div>
      <div class="info-item">
        <span class="info-label">设备数限制</span>
        <strong>{{ deviceText }}</strong>
      </div>
      <div class="info-item">
        <span class="info-label">在线设备</span>
        <strong>{{ props.user.online_count }} 台</strong>
      </div>
      <div class="info-item">
        <span class="info-label">最后在线</span>
        <strong>{{ props.user.last_online_at ? formatDateTime(props.user.last_online_at) : '—' }}</strong>
      </div>
    </div>

    <div
      v-else
      class="limits-editor"
    >
      <div class="field">
        <label for="user-speed-limit">限速（Mbps，0 = 不限速）</label>
        <input
          id="user-speed-limit"
          v-model.number="speedLimit"
          type="number"
          min="0"
          step="1"
        >
      </div>
      <div class="field">
        <label for="user-device-limit">设备数限制（0 = 不限制）</label>
        <input
          id="user-device-limit"
          v-model.number="deviceLimit"
          type="number"
          min="0"
          step="1"
        >
      </div>
      <button
        type="button"
        class="btn small"
        :class="{ 'is-loading': saving }"
        :disabled="saving"
        @click="save"
      >
        <LoadingSpinner
          v-if="saving"
          size="sm"
        />
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>
  </div>
</template>

<style scoped>
.limits-grid {
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0 var(--spacing-lg);
  margin-top: var(--spacing-md);
}

.limits-grid .info-item {
  min-height: 60px;
  justify-content: center;
  padding: var(--spacing-sm) 0;
  border-bottom: 1px solid var(--color-border);
}

.limits-grid .info-item strong {
  overflow-wrap: anywhere;
  font-size: var(--font-size-md);
  font-weight: 650;
}

.limits-editor {
  display: flex;
  align-items: flex-end;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-top: var(--spacing-md);
}

.limits-editor .field {
  flex: 1 1 160px;
}

@media (max-width: 560px) {
  .limits-editor .btn {
    width: 100%;
  }

  .limits-grid {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
