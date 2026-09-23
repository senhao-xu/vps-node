<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createUserSubscription, expireUserNow, getUserSubscription, resetUserToken, rotateUserSubscription, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail, UserStatus, Subscription } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import CopyText from '@/components/CopyText.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import OneTimeSecret from '@/components/OneTimeSecret.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
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
const usernameDraft = ref('')
const editingUsername = ref(false)
const savingUsername = ref(false)
const error = ref('')
const subscription = ref<Subscription>({ configured: false, url: null })
const loadingSubscription = ref(false)
const rotatingSubscription = ref(false)

const showResetTokenConfirm = ref(false)
const resettingToken = ref(false)
const showExpireConfirm = ref(false)
const expiring = ref(false)
const showRotateConfirm = ref(false)

const usernamePattern = /^[A-Za-z0-9_.-]{1,64}$/

watch(
  () => props.user.id,
  async () => {
    tokenRevealed.value = null
    statusDraft.value = props.user.status === 'disabled' ? 'disabled' : 'active'
    usernameDraft.value = props.user.username
    editingUsername.value = false
    loadingSubscription.value = true
    try { subscription.value = await getUserSubscription(props.user.id) } catch (err) { error.value = errorMessage(err) } finally { loadingSubscription.value = false }
  },
  { immediate: true },
)

async function provisionSubscription() {
  loadingSubscription.value = true
  try { subscription.value = await createUserSubscription(props.user.id) } catch (err) { error.value = errorMessage(err) } finally { loadingSubscription.value = false }
}

async function rotateSubscription() {
  rotatingSubscription.value = true
  try {
    subscription.value = await rotateUserSubscription(props.user.id)
    showRotateConfirm.value = false
  } catch (err) { error.value = errorMessage(err) } finally { rotatingSubscription.value = false }
}

watch(
  () => props.user.username,
  (username) => {
    if (!editingUsername.value) usernameDraft.value = username
  },
)

const status = computed(() => displayUserStatus(props.user))

async function saveUsername() {
  const next = usernameDraft.value.trim()
  if (next === props.user.username) {
    editingUsername.value = false
    return
  }
  if (!usernamePattern.test(next)) {
    error.value = '用户名需为 1-64 个字符，仅限字母、数字、下划线、中划线或点'
    return
  }
  savingUsername.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, { username: next })
    editingUsername.value = false
    emit('updated', updated)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    savingUsername.value = false
  }
}

async function saveStatus(enabled: boolean) {
  const next: UserStatus = enabled ? 'active' : 'disabled'
  if (next === props.user.status) return
  statusDraft.value = next
  savingStatus.value = true
  error.value = ''
  try {
    const updated = await updateUser(props.user.id, { status: next })
    emit('updated', updated)
  } catch (err) {
    statusDraft.value = props.user.status === 'disabled' ? 'disabled' : 'active'
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
  <div class="card basic-card">
    <h2 class="card-title">
      基本信息
    </h2>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="info-item subscription-item">
      <span class="info-label">订阅链接</span>
      <span v-if="loadingSubscription">
        <LoadingSpinner size="sm" />
      </span>
      <span
        v-else-if="subscription.url"
        class="subscription-row"
      >
        <CopyText
          class="mono subscription-url"
          :text="subscription.url"
        />
        <button
          type="button"
          class="btn link small"
          :disabled="rotatingSubscription"
          @click="showRotateConfirm = true"
        >
          {{ rotatingSubscription ? '轮换中…' : '轮换' }}
        </button>
      </span>
      <button
        v-else
        type="button"
        class="btn small"
        @click="provisionSubscription"
      >
        生成订阅链接
      </button>
    </div>
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
        <span class="info-label">用户名</span>
        <span
          v-if="!editingUsername"
          class="status-row"
        >
          <span>{{ props.user.username }}</span>
          <button
            type="button"
            class="btn link small"
            @click="editingUsername = true"
          >
            编辑
          </button>
        </span>
        <span
          v-else
          class="username-edit"
        >
          <input
            v-model="usernameDraft"
            type="text"
            maxlength="64"
            :disabled="savingUsername"
            @keyup.enter="saveUsername"
            @keyup.esc="editingUsername = false"
          >
          <button
            type="button"
            class="btn small"
            :disabled="savingUsername"
            @click="saveUsername"
          >
            保存
          </button>
          <button
            type="button"
            class="btn secondary small"
            :disabled="savingUsername"
            @click="editingUsername = false; usernameDraft = props.user.username"
          >
            取消
          </button>
        </span>
      </div>
      <div class="info-item">
        <span class="info-label">状态</span>
        <span class="status-row">
          <StatusBadge v-bind="status" />
          <ToggleSwitch
            :model-value="statusDraft === 'active'"
            :disabled="savingStatus"
            label="启用用户"
            @update:model-value="saveStatus"
          />
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
        </span>
      </div>
      <div class="info-item actions-item">
        <span class="info-label">操作</span>
        <span class="actions-row">
          <button
            type="button"
            class="btn secondary small"
            @click="showResetTokenConfirm = true"
          >
            重置 Token
          </button>
          <button
            type="button"
            class="btn danger secondary small"
            @click="showExpireConfirm = true"
          >
            立即过期
          </button>
        </span>
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

    <ConfirmDialog
      :open="showRotateConfirm"
      title="轮换订阅链接"
      message="轮换后旧订阅链接将立即失效，确定继续吗？"
      confirm-text="轮换"
      :loading="rotatingSubscription"
      @cancel="showRotateConfirm = false"
      @confirm="rotateSubscription"
    />
  </div>
</template>

<style scoped>
.subscription-item {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-sm);
  margin-bottom: var(--spacing-lg);
  padding: var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.subscription-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
  min-width: 0;
}

.subscription-url {
  max-width: min(520px, 100%);
}

.basic-card .info-label {
  font-weight: 500;
}

@media (min-width: 701px) {
  .basic-card .info-grid {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--spacing-lg) var(--spacing-xl);
  }
}

.actions-item {
  grid-column: 1 / -1;
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding-top: var(--spacing-md);
  border-top: 1px solid var(--color-border);
}

.actions-item .actions-row {
  margin-left: auto;
}

.status-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.actions-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.username-edit {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.username-edit input {
  width: 200px;
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
