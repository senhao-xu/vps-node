<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { resetUserTraffic, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ProgressBar from '@/components/ProgressBar.vue'
import type { QuotaUnit } from '@/utils/format'
import { formatBytes, quotaFromInput, splitQuota } from '@/utils/format'

const props = defineProps<{
  user: UserDetail
}>()

const emit = defineEmits<{
  (e: 'updated', user: UserDetail): void
}>()

const unlimited = computed(() => props.user.transfer_enable <= 0)

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
  const split = splitQuota(props.user.transfer_enable)
  editUnlimited.value = unlimited.value
  editValue.value = unlimited.value ? null : split.value
  editUnit.value = split.unit
})

async function saveQuota() {
  let transferEnable = 0
  if (!editUnlimited.value) {
    const value = editValue.value
    if (value === null || Number.isNaN(value) || value < 0) {
      error.value = '请输入有效的流量额度'
      return
    }
    transferEnable = quotaFromInput(value, editUnit.value)
  }
  saving.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, { transfer_enable: transferEnable })
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
  <div>
    <div class="card-head">
      <h2 class="card-title">
        流量
      </h2>
      <div class="card-head-actions">
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
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="!editing"
      class="traffic-view"
    >
      <div class="traffic-summary">
        <span class="traffic-caption">累计已用</span>
        <div class="traffic-total">
          <strong class="traffic-used">{{ formatBytes(props.user.used_bytes) }}</strong>
          <span class="traffic-quota">{{ unlimited ? '不限流量额度' : `总额度 ${formatBytes(props.user.transfer_enable)}` }}</span>
        </div>
      </div>
      <ProgressBar
        v-if="!unlimited"
        :percent="props.user.used_percent"
      />
      <div class="traffic-breakdown">
        <div class="traffic-stat">
          <span>上传</span>
          <strong>{{ formatBytes(props.user.u) }}</strong>
        </div>
        <div class="traffic-stat">
          <span>下载</span>
          <strong>{{ formatBytes(props.user.d) }}</strong>
        </div>
        <div class="traffic-stat">
          <span>剩余</span>
          <strong>{{ unlimited ? '不限' : formatBytes(props.user.remaining_bytes) }}</strong>
        </div>
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
          class="quota-input"
          type="number"
          min="0"
          step="any"
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
.quota-input {
  width: 140px;
}

.traffic-view {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}

.traffic-summary {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.traffic-caption {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.traffic-total {
  display: flex;
  align-items: baseline;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.traffic-used {
  font-size: 24px;
  font-weight: 700;
  line-height: 1.2;
}

.traffic-quota {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.traffic-breakdown {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 0;
  border-top: 1px solid var(--color-border);
}

.traffic-stat {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
  padding: var(--spacing-sm) var(--spacing-md) 0 0;
  border-right: 1px solid var(--color-border);
}

.traffic-stat:last-child {
  border-right: 0;
  padding-left: var(--spacing-md);
}

.traffic-stat:nth-child(2) {
  padding-left: var(--spacing-md);
}

.traffic-stat span {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.traffic-stat strong {
  overflow: hidden;
  font-size: var(--font-size-md);
  font-weight: 650;
  font-variant-numeric: tabular-nums;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.quota-editor {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  margin-top: var(--spacing-md);
}

@media (max-width: 560px) {
  .card-head-actions {
    width: 100%;
  }

  .card-head-actions .btn {
    flex: 1;
  }

  .traffic-breakdown {
    grid-template-columns: minmax(0, 1fr);
  }

  .traffic-stat,
  .traffic-stat:nth-child(2),
  .traffic-stat:last-child {
    flex-direction: row;
    align-items: center;
    justify-content: space-between;
    padding: var(--spacing-sm) 0;
    border-right: 0;
    border-bottom: 1px solid var(--color-border);
  }

  .traffic-stat:last-child {
    border-bottom: 0;
  }
}
</style>
