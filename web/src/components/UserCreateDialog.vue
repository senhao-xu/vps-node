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

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'created', user: UserCreated): void
}>()

const nodes = ref<NodeBrief[]>([])
const servers = ref<Server[]>([])
const unlimited = ref(true)
const quotaValue = ref<number | null>(null)
const quotaUnit = ref<QuotaUnit>('GB')
const startedAt = ref('')
const expiresAt = ref('')
const selectedNodeIds = ref<number[]>([])
const submitting = ref(false)
const error = ref('')
const created = ref<UserCreated | null>(null)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    created.value = null
    error.value = ''
    unlimited.value = true
    quotaValue.value = null
    quotaUnit.value = 'GB'
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

const validationMessage = computed(() => {
  if (!unlimited.value && quotaBytes.value === null) return '请输入有效的流量额度'
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
      quota_bytes: quotaBytes.value ?? 0,
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
    :width="560"
    @close="emit('close')"
  >
    <template v-if="created">
      <p>用户已创建，请立即保存以下一次性 Token：</p>
      <div class="created-info">
        <span class="text-secondary">UUID：</span>
        <code class="mono">{{ created.uuid }}</code>
      </div>
      <OneTimeSecret
        label="用户 Token"
        :value="created.token"
      />
    </template>
    <template v-else>
      <ErrorBanner
        :message="error"
        @dismiss="error = ''"
      />
      <div class="form">
        <div class="field">
          <label>流量额度</label>
          <div class="quota-row">
            <label class="checkbox-label">
              <input
                v-model="unlimited"
                type="checkbox"
              >
              不限流量
            </label>
            <template v-if="!unlimited">
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
            </template>
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
          :disabled="submitting"
          @click="submit"
        >
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
