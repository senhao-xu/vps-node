<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { expireUserNow, resetUserToken, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail, UserStatus } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import CopyText from '@/components/CopyText.vue'
import OneTimeSecret from '@/components/OneTimeSecret.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import { formatDateTime } from '@/utils/format'
import { displayUserStatus } from '@/utils/labels'

const props = defineProps<{
  user: UserDetail
}>()

const emit = defineEmits<{
  (e: 'updated', user: UserDetail): void
}>()

const tokenRevealed = ref<string | null>(null)
const statusDraft = ref<UserStatus>('active')
const savingStatus = ref(false)
const error = ref('')

const showResetTokenConfirm = ref(false)
const resettingToken = ref(false)
const showExpireConfirm = ref(false)
const expiring = ref(false)

watch(
  () => props.user.id,
  () => {
    tokenRevealed.value = null
    statusDraft.value = props.user.status === 'disabled' ? 'disabled' : 'active'
  },
  { immediate: true },
)

const status = computed(() => displayUserStatus(props.user))

async function saveStatus() {
  if (statusDraft.value === props.user.status) return
  savingStatus.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, { status: statusDraft.value })
    emit('updated', updated)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    savingStatus.value = false
  }
}

async function resetToken() {
  resettingToken.value = true
  error.value = ''
  try {
    const result = await resetUserToken(props.user.id)
    tokenRevealed.value = result.token
    showResetTokenConfirm.value = false
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    resettingToken.value = false
  }
}

async function expireNow() {
  expiring.value = true
  error.value = ''
  try {
    const updated = await expireUserNow(props.user.id)
    showExpireConfirm.value = false
    emit('updated', updated)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    expiring.value = false
  }
}
</script>

<template>
  <div class="card">
    <h2 class="card-title">
      基本信息
    </h2>
    <p
      v-if="error"
      class="text-danger form-error"
    >
      {{ error }}
    </p>
    <div class="info-grid">
      <div class="info-item">
        <span class="info-label">用户 ID</span>
        <span>{{ props.user.id }}</span>
      </div>
      <div class="info-item">
        <span class="info-label">UUID</span>
        <CopyText
          class="mono"
          :text="props.user.uuid"
        />
      </div>
      <div class="info-item">
        <span class="info-label">状态</span>
        <span class="status-row">
          <StatusBadge v-bind="status" />
          <select
            v-model="statusDraft"
            :disabled="savingStatus"
            @change="saveStatus"
          >
            <option value="active">正常</option>
            <option value="disabled">禁用</option>
          </select>
        </span>
      </div>
      <div class="info-item">
        <span class="info-label">创建时间</span>
        <span>{{ formatDateTime(props.user.created_at) }}</span>
      </div>
      <div class="info-item token-item">
        <span class="info-label">Token</span>
        <span
          v-if="tokenRevealed"
          class="token-body"
        >
          <OneTimeSecret
            label="新 Token（旧 Token 已失效）"
            :value="tokenRevealed"
          />
        </span>
        <span
          v-else
          class="token-body"
        >
          <span class="masked">••••••••••••</span>
          <button
            type="button"
            class="btn secondary small"
            @click="showResetTokenConfirm = true"
          >
            重置 Token
          </button>
        </span>
      </div>
      <div class="info-item">
        <span class="info-label">操作</span>
        <button
          type="button"
          class="btn danger secondary small"
          @click="showExpireConfirm = true"
        >
          立即过期
        </button>
      </div>
    </div>

    <ConfirmDialog
      :open="showResetTokenConfirm"
      title="重置 Token"
      :message="`确定重置用户 #${props.user.id} 的 Token 吗？\n旧 Token 将立即失效。新 Token 仅显示一次。`"
      confirm-text="重置"
      :loading="resettingToken"
      @cancel="showResetTokenConfirm = false"
      @confirm="resetToken"
    />

    <ConfirmDialog
      :open="showExpireConfirm"
      title="立即过期"
      :message="`确定让用户 #${props.user.id} 立即过期吗？\n到期时间将设为当前时间，该用户将无法使用任何节点。`"
      danger
      confirm-text="立即过期"
      :loading="expiring"
      @cancel="showExpireConfirm = false"
      @confirm="expireNow"
    />
  </div>
</template>

<style scoped>
.form-error {
  margin: 0 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
}

.info-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--spacing-md) var(--spacing-lg);
}

.info-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.info-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.status-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.token-body {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.token-item .token-body {
  width: 100%;
}

.masked {
  letter-spacing: 2px;
  color: var(--color-text-secondary);
}
</style>
