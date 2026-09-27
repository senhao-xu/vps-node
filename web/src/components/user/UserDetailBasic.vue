<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { toDataURL } from 'qrcode'
import { Box, Copy, QrCode, Rocket, Send } from 'lucide-vue-next'
import { createUserSubscription, expireUserNow, getUserSubscription, resetUserToken, rotateUserSubscription, updateUser } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { UserDetail, UserStatus, Subscription } from '@/api/types'
import ConfirmDialog from '@/components/ConfirmDialog.vue'
import CopyText from '@/components/CopyText.vue'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import OneTimeSecret from '@/components/OneTimeSecret.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import { useCopyFeedback } from '@/utils/clipboard'
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
const showQrDialog = ref(false)
const qrDataUrl = ref('')
const qrError = ref('')
const { copied: subCopied, copy: copySub } = useCopyFeedback()
const { copied: linkCopied, copy: copyLink } = useCopyFeedback()

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

const subscriptionUrl = computed(() => subscription.value.url ?? '')

watch(
  subscriptionUrl,
  async (url) => {
    qrError.value = ''
    if (!url) {
      qrDataUrl.value = ''
      return
    }
    try {
      qrDataUrl.value = await toDataURL(url, { margin: 1, width: 264 })
    } catch {
      qrDataUrl.value = ''
      qrError.value = '二维码生成失败，请复制链接手动导入'
    }
  },
  { immediate: true },
)

function toBase64(text: string): string {
  const bytes = new TextEncoder().encode(text)
  let binary = ''
  for (const byte of bytes) binary += String.fromCharCode(byte)
  return btoa(binary)
}

const clashLink = computed(() => `clash://install-config?url=${encodeURIComponent(subscriptionUrl.value)}`)
const shadowrocketLink = computed(() => `shadowrocket://add/sub://${toBase64(subscriptionUrl.value)}`)
const singBoxLink = computed(
  () =>
    `sing-box://import-remote-profile?url=${encodeURIComponent(subscriptionUrl.value)}#${encodeURIComponent(props.user.username)}`,
)

function openDeepLink(link: string) {
  window.location.href = link
}

async function copySubscriptionLink() {
  await copyLink(subscriptionUrl.value)
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
      <template v-else-if="subscription.url">
        <span class="subscription-row">
          <span class="sub-url-bar mono">{{ subscription.url }}</span>
          <span class="sub-actions">
            <button
              type="button"
              class="btn small"
              @click="copySub(subscription.url ?? '')"
            >
              {{ subCopied ? '已复制' : '复制订阅' }}
            </button>
            <button
              type="button"
              class="btn secondary small"
              @click="showQrDialog = true"
            >
              <QrCode :size="14" />
              扫码导入
            </button>
            <button
              type="button"
              class="btn secondary small"
              :disabled="rotatingSubscription"
              @click="showRotateConfirm = true"
            >
              {{ rotatingSubscription ? '轮换中…' : '轮换' }}
            </button>
          </span>
        </span>
        <div class="client-grid-head">
          <span>快捷一键导入到客户端：</span>
          <span class="client-grid-hint">点击直接唤起客户端</span>
        </div>
        <div class="client-grid">
          <div class="client-card accent-info">
            <span class="client-icon">
              <Send :size="18" />
            </span>
            <div class="client-info">
              <span class="client-name">Clash / Verge</span>
              <span class="client-desc">一键导入配置</span>
            </div>
            <button
              type="button"
              class="client-btn"
              @click="openDeepLink(clashLink)"
            >
              导入
            </button>
          </div>
          <div class="client-card accent-purple">
            <span class="client-icon">
              <Rocket :size="18" />
            </span>
            <div class="client-info">
              <span class="client-name">小火箭 Shadowrocket</span>
              <span class="client-desc">iOS 专属导入</span>
            </div>
            <button
              type="button"
              class="client-btn"
              @click="openDeepLink(shadowrocketLink)"
            >
              导入
            </button>
          </div>
          <div class="client-card accent-warning">
            <span class="client-icon">
              <Box :size="18" />
            </span>
            <div class="client-info">
              <span class="client-name">Sing-box</span>
              <span class="client-desc">跨平台通用核心</span>
            </div>
            <button
              type="button"
              class="client-btn"
              @click="openDeepLink(singBoxLink)"
            >
              导入
            </button>
          </div>
          <div class="client-card accent-success">
            <span class="client-icon">
              <Copy :size="18" />
            </span>
            <div class="client-info">
              <span class="client-name">v2rayN / Nekobox</span>
              <span class="client-desc">点击复制订阅链接</span>
            </div>
            <button
              type="button"
              class="client-btn"
              @click="copySubscriptionLink"
            >
              {{ linkCopied ? '已复制' : '复制' }}
            </button>
          </div>
        </div>
      </template>
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

    <ModalDialog
      :open="showQrDialog"
      title="扫码导入订阅"
      subtitle="使用客户端扫描下方二维码导入订阅链接"
      :width="360"
      @close="showQrDialog = false"
    >
      <div class="qr-body">
        <img
          v-if="qrDataUrl"
          class="qr-image"
          :src="qrDataUrl"
          alt="订阅链接二维码"
          width="264"
          height="264"
        >
        <ErrorBanner
          v-else-if="qrError"
          :message="qrError"
          @dismiss="qrError = ''"
        />
        <span v-else>
          <LoadingSpinner />
        </span>
        <span class="mono qr-url">{{ subscriptionUrl }}</span>
      </div>
      <template #footer>
        <button
          type="button"
          class="btn secondary"
          @click="showQrDialog = false"
        >
          关闭
        </button>
      </template>
    </ModalDialog>
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

.sub-url-bar {
  flex: 1;
  min-width: 200px;
  overflow: hidden;
  padding: 8px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sub-actions {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
  flex-wrap: wrap;
}

.client-grid-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.client-grid-hint {
  font-size: var(--font-size-xs);
}

.client-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: var(--spacing-sm);
}

.client-card {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-width: 0;
  padding: var(--spacing-sm) var(--spacing-md);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
}

.client-btn {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  min-height: 28px;
  padding: 3px 14px;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  font-size: var(--font-size-sm);
  font-weight: 600;
  cursor: pointer;
  white-space: nowrap;
}

.accent-info .client-icon {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.accent-info .client-btn {
  background: var(--color-info-soft);
  color: var(--color-info);
}

.accent-purple .client-icon {
  background: var(--color-badge-purple-soft);
  color: var(--color-badge-purple);
}

.accent-purple .client-btn {
  background: var(--color-badge-purple-soft);
  color: var(--color-badge-purple);
}

.accent-warning .client-icon {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.accent-warning .client-btn {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.accent-success .client-icon {
  background: var(--color-success-soft);
  color: var(--color-success);
}

.accent-success .client-btn {
  background: var(--color-success-soft);
  color: var(--color-success);
}

.client-icon {
  display: inline-flex;
  width: 34px;
  height: 34px;
  flex: none;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  background: var(--color-surface-muted);
  color: var(--color-text-secondary);
}

.client-info {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 1px;
}

.client-name {
  font-size: var(--font-size-sm);
  font-weight: 500;
}

.client-desc {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

.qr-body {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-md);
}

.qr-image {
  display: block;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
}

.qr-url {
  max-width: 100%;
  overflow-wrap: anywhere;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-align: center;
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
