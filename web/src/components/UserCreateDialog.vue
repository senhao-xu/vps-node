<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { createUser } from '@/api/users'
import { listNodes } from '@/api/nodes'
import { listServers } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { NodeBrief, Server, UserCreated } from '@/api/types'
import type { QuotaUnit } from '@/utils/format'
import { localInputToIso, quotaFromInput } from '@/utils/format'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import NodeChecklist from '@/components/NodeChecklist.vue'
import OneTimeSecret from '@/components/OneTimeSecret.vue'
import CopyText from '@/components/CopyText.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', user: UserCreated): void
}>()

const nodes = ref<NodeBrief[]>([])
const servers = ref<Server[]>([])
const username = ref('')
const unlimited = ref(true)
const quotaValue = ref<number | null>(null)
const quotaUnit = ref<QuotaUnit>('GB')
const speedLimit = ref<number | null>(0)
const deviceLimit = ref<number | null>(0)
const startedAt = ref('')
const expiresAt = ref('')
const selectedNodeIds = ref<number[]>([])
const submitting = ref(false)
const error = ref('')
const created = ref<UserCreated | null>(null)

const usernamePattern = /^[A-Za-z0-9_.-]{1,64}$/

watch(
  () => props.open,
  (open) => {
    if (!open) return
    created.value = null
    error.value = ''
    username.value = ''
    unlimited.value = true
    quotaValue.value = null
    quotaUnit.value = 'GB'
    speedLimit.value = 0
    deviceLimit.value = 0
    startedAt.value = ''
    expiresAt.value = ''
    selectedNodeIds.value = []
    void loadRefs()
  },
)

async function loadRefs() {
  try {
    const [nodePage, serverPage] = await Promise.all([
      listNodes({ page: 1, pageSize: 100 }),
      listServers({ page: 1, pageSize: 100 }),
    ])
    nodes.value = nodePage.items
    servers.value = serverPage.items
  } catch (err) {
    error.value = errorMessage(err)
  }
}

const quotaBytes = computed(() => {
  if (unlimited.value) return 0
  const value = quotaValue.value
  if (value === null || Number.isNaN(value) || value < 0) return null
  return quotaFromInput(value, quotaUnit.value)
})

function validLimit(value: number | null): boolean {
  return value !== null && Number.isInteger(value) && value >= 0
}

const validationMessage = computed(() => {
  const name = username.value.trim()
  if (!name) return '请输入用户名'
  if (!usernamePattern.test(name)) return '用户名需为 1-64 个字符，仅限字母、数字、下划线、中划线或点'
  if (!unlimited.value && quotaBytes.value === null) return '请输入有效的流量额度'
  if (!validLimit(speedLimit.value)) return '限速必须是不小于 0 的整数（0 表示不限速）'
  if (!validLimit(deviceLimit.value)) return '设备数限制必须是不小于 0 的整数（0 表示不限制）'
  const start = localInputToIso(startedAt.value)
  const expire = localInputToIso(expiresAt.value)
  if (start && expire && expire <= start) return '到期时间必须晚于开始时间'
  return ''
})

async function submit() {
  const problem = validationMessage.value
  if (problem) {
    error.value = problem
    return
  }
  error.value = ''
  submitting.value = true
  try {
    const user = await createUser({
      username: username.value.trim(),
      transfer_enable: quotaBytes.value ?? 0,
      speed_limit: speedLimit.value ?? 0,
      device_limit: deviceLimit.value ?? 0,
      started_at: localInputToIso(startedAt.value),
      expires_at: localInputToIso(expiresAt.value),
      node_ids: selectedNodeIds.value,
    })
    created.value = user
    emit('created', user)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="created ? '用户创建成功' : '创建用户'"
    :subtitle="created ? undefined : '创建用户并分配节点授权，Token 仅显示一次'"
    :width="560"
    @close="emit('close')"
  >
    <template v-if="created">
      <p>用户已创建，请立即保存以下一次性 Token：</p>
      <div class="created-info">
        <span class="text-secondary">用户名：</span>
        <code class="mono">{{ created.username }}</code>
      </div>
      <div class="created-info">
        <span class="text-secondary">UUID：</span>
        <code class="mono">{{ created.uuid }}</code>
      </div>
      <OneTimeSecret
        label="用户 Token"
        :value="created.token"
      />
      <div class="subscription-secret">
        <span class="text-secondary">订阅链接（可复制）</span>
        <CopyText
          class="subscription-url"
          :text="created.subscription_url"
          :display="created.subscription_url"
        />
      </div>
    </template>
    <template v-else>
      <ErrorBanner
        :message="error"
        @dismiss="error = ''"
      />
      <div class="form">
        <div class="field">
          <label>用户名（必填）</label>
          <input
            v-model="username"
            type="text"
            maxlength="64"
            placeholder="1-64 个字符，仅限字母、数字、_ - ."
            @keyup.enter="submit"
          >
        </div>
        <div class="field">
          <label>流量额度</label>
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">不限流量</span>
              <span class="toggle-row-desc">关闭后可设置流量配额</span>
            </div>
            <ToggleSwitch
              v-model="unlimited"
              label="不限流量"
            />
          </div>
          <div
            v-if="!unlimited"
            class="quota-row"
          >
            <input
              v-model.number="quotaValue"
              type="number"
              min="0"
              step="any"
              placeholder="额度"
            >
            <select v-model="quotaUnit">
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
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>限速（Mbps，0 = 不限速）</label>
            <input
              v-model.number="speedLimit"
              type="number"
              min="0"
              step="1"
            >
          </div>
          <div class="field">
            <label>设备数限制（0 = 不限制）</label>
            <input
              v-model.number="deviceLimit"
              type="number"
              min="0"
              step="1"
            >
          </div>
        </div>
        <div class="form-row">
          <div class="field">
            <label>开始时间（可选）</label>
            <input
              v-model="startedAt"
              type="datetime-local"
            >
          </div>
          <div class="field">
            <label>到期时间（可选，留空为永不过期）</label>
            <input
              v-model="expiresAt"
              type="datetime-local"
            >
          </div>
        </div>
        <div class="field">
          <label>授权节点（可多选）</label>
          <NodeChecklist
            v-model="selectedNodeIds"
            :nodes="nodes"
            :servers="servers"
          />
        </div>
      </div>
    </template>
    <template #footer>
      <template v-if="created">
        <button
          type="button"
          class="btn"
          @click="emit('close')"
        >
          完成
        </button>
      </template>
      <template v-else>
        <button
          type="button"
          class="btn secondary"
          :disabled="submitting"
          @click="emit('close')"
        >
          取消
        </button>
        <button
          type="button"
          class="btn"
          :class="{ 'is-loading': submitting }"
          :disabled="submitting"
          @click="submit"
        >
          <LoadingSpinner
            v-if="submitting"
            size="sm"
          />
          {{ submitting ? '创建中…' : '创建' }}
        </button>
      </template>
    </template>
  </ModalDialog>
</template>

<style scoped>
.form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}

.quota-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.quota-row input[type='number'] {
  width: 120px;
}

.created-info {
  margin: 0 0 var(--spacing-sm);
}
</style>
