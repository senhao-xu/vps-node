<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { resetUserTraffic, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import type { QuotaUnit } from '@/utils/format'
import { formatBytes, formatPercent, quotaFromInput, splitQuota } from '@/utils/format'

const props = defineProps<{
  user: UserDetail
}>()

const emit = defineEmits<{
  (e: 'updated', user: UserDetail): void
}>()

const unlimited = computed(() => props.user.quota_bytes <= 0)

const editing = ref(false)
const editUnlimited = ref(false)
const editValue = ref<number | null>(null)
const editUnit = ref<QuotaUnit>('GB')
const saving = ref(false)
const error = ref('')
const showResetConfirm = ref(false)
const resetting = ref(false)

watch(editing, (editingNow) => {
  if (!editingNow) return
  error.value = ''
  const split = splitQuota(props.user.quota_bytes)
  editUnlimited.value = unlimited.value
  editValue.value = unlimited.value ? null : split.value
  editUnit.value = split.unit
})

async function saveQuota() {
  let quota: number | null = 0
  if (!editUnlimited.value) {
    const value = editValue.value
    if (value === null || Number.isNaN(value) || value < 0) {
      error.value = '请输入有效的流量额度'
      return
    }
    quota = quotaFromInput(value, editUnit.value)
  }
  saving.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, { quota_bytes: quota })
    editing.value = false
    emit('updated', updated)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}

async function resetTraffic() {
  resetting.value = true
  error.value = ''
  try {
    const updated = await resetUserTraffic(props.user.id)
    showResetConfirm.value = false
    emit('updated', updated)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="card-head">
      <h2 class="card-title">
        流量
      </h2>
      <div class="head-actions">
        <button
          type="button"
          class="btn secondary small"
          @click="editing = !editing"
        >
          {{ editing ? '取消编辑' : '修改额度' }}
        </button>
        <button
          type="button"
          class="btn danger secondary small"
          @click="showResetConfirm = true"
        >
          重置流量
        </button>
      </div>
    </div>
    <p
      v-if="error"
      class="text-danger form-error"
    >
      {{ error }}
    </p>

    <div
      v-if="!editing"
      class="traffic-view"
    >
      <div class="traffic-summary">
        <span>已使用 {{ formatBytes(props.user.used_bytes) }}</span>
        <span class="text-secondary">/ {{ unlimited ? '不限' : formatBytes(props.user.quota_bytes) }}</span>
      </div>
      <ProgressBar :percent="props.user.used_percent" />
      <div class="traffic-detail text-secondary">
        <span>剩余 {{ unlimited ? '不限' : formatBytes(props.user.remaining_bytes) }}</span>
        <span>{{ unlimited ? '不限额度' : formatPercent(props.user.used_percent) }}</span>
      </div>
    </div>

    <div
      v-else
      class="quota-editor"
    >
      <label class="checkbox-label">
        <input
          v-model="editUnlimited"
          type="checkbox"
        >
        不限流量
      </label>
      <template v-if="!editUnlimited">
        <input
          v-model.number="editValue"
          type="number"
          min="0"
          step="any"
          style="width: 140px"
        >
        <select v-model="editUnit">
          <option value="MB">
            MB
          </option>
          <option value="GB">
            GB
          </option>
          <option value="TB">
            TB
          </option>
        </select>
      </template>
      <button
        type="button"
        class="btn small"
        :disabled="saving"
        @click="saveQuota"
      >
        {{ saving ? '保存中…' : '保存' }}
      </button>
    </div>

    <ConfirmDialog
      :open="showResetConfirm"
      title="重置流量"
      :message="`确定重置用户 #${props.user.id} 的已用流量吗？\n已用流量将清零，该操作不影响流量额度。`"
      confirm-text="重置"
      :loading="resetting"
      @cancel="showResetConfirm = false"
      @confirm="resetTraffic"
    />
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

.head-actions {
  display: flex;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-md);
}

.form-error {
  margin: var(--spacing-sm) 0;
  font-size: var(--font-size-sm);
}

.traffic-view {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-top: var(--spacing-md);
}

.traffic-summary {
  display: flex;
  gap: var(--spacing-xs);
}

.traffic-detail {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-sm);
}

.quota-editor {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-top: var(--spacing-md);
}
</style>
