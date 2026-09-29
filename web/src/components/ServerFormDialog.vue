<script setup lang="ts">
import { ref, useId, watch } from 'vue'
import { createServer, updateServer } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { Server, ServerStatus, TrafficAccounting } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'

const props = withDefaults(defineProps<{
  open: boolean
  server?: Server | null
}>(), { server: null })
const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
  (e: 'created', id: number): void
}>()

const fieldPrefix = `server-form-${useId()}`
const name = ref('')
const notes = ref('')
const status = ref<ServerStatus>('active')
const publicVisible = ref(true)
const offlineNotify = ref(false)
const ip = ref('')
const ipv6 = ref('')
const region = ref('')
const trafficLimit = ref<number | null>(null)
const accounting = ref<TrafficAccounting>('max')
const resetDay = ref(1)
const submitting = ref(false)
const error = ref('')

const accountingOptions: Array<{ value: TrafficAccounting; label: string }> = [
  { value: 'max', label: '取较大值' },
  { value: 'sum', label: '上行 + 下行' },
]

watch(() => props.open, (open) => {
  if (!open) return
  const server = props.server
  error.value = ''
  name.value = server?.name ?? ''
  notes.value = server?.notes ?? ''
  status.value = server?.status === 'disabled' ? 'disabled' : 'active'
  publicVisible.value = server?.public_visible ?? true
  offlineNotify.value = server?.offline_notify ?? false
  ip.value = server?.ip ?? ''
  ipv6.value = server?.ipv6 ?? ''
  region.value = server?.region ?? ''
  trafficLimit.value = server?.traffic_limit_bytes ? server.traffic_limit_bytes / 1024 ** 3 : null
  accounting.value = server?.traffic_accounting ?? 'max'
  resetDay.value = server?.traffic_reset_day ?? 1
})

async function submit() {
  const quota = Number(trafficLimit.value ?? 0)
  if (!name.value.trim()) { error.value = '请填写名称'; return }
  if (!Number.isFinite(quota) || quota < 0 || !Number.isInteger(resetDay.value) || resetDay.value < 1 || resetDay.value > 31) {
    error.value = '请填写有效的流量额度和每月重置日'
    return
  }
  submitting.value = true
  error.value = ''
  const input = {
    name: name.value.trim(),
    notes: notes.value.trim(),
    status: status.value,
    public_visible: publicVisible.value,
    offline_notify: offlineNotify.value,
    ip: ip.value.trim(),
    ipv6: ipv6.value.trim(),
    region: region.value.trim().toUpperCase(),
    traffic_limit_bytes: Math.round(quota * 1024 ** 3),
    traffic_accounting: accounting.value,
    traffic_reset_day: resetDay.value,
  }
  try {
    if (props.server) {
      await updateServer(props.server.id, input)
      emit('saved')
    } else {
      const created = await createServer(input)
      emit('created', created.id)
    }
    emit('close')
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
    :title="props.server?.name ?? '创建服务器'"
    :width="840"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="server-form">
      <div class="top-grid">
        <div class="field">
          <label :for="`${fieldPrefix}-name`">名称</label>
          <input
            :id="`${fieldPrefix}-name`"
            v-model="name"
            type="text"
            placeholder="例如 HK-01"
          >
        </div>
        <div class="field">
          <label :for="`${fieldPrefix}-notes`">备注</label>
          <input
            :id="`${fieldPrefix}-notes`"
            v-model="notes"
            type="text"
            placeholder="商家、用途，仅管理员可见"
          >
        </div>
      </div>

      <div class="toggle-grid">
        <div class="toggle-card">
          <div class="toggle-row-text">
            <span class="toggle-row-label">公开显示</span>
            <span class="toggle-row-desc">保存公开偏好，公开页面接入后生效</span>
          </div>
          <ToggleSwitch
            v-model="publicVisible"
            label="公开显示"
          />
        </div>
        <div class="toggle-card">
          <div class="toggle-row-text">
            <span class="toggle-row-label">离线通知</span>
            <span class="toggle-row-desc">保存通知偏好，接入通知渠道后生效</span>
          </div>
          <ToggleSwitch
            v-model="offlineNotify"
            label="离线通知"
          />
        </div>
      </div>

      <section class="form-section">
        <h3>流量</h3>
        <div class="traffic-grid">
          <div class="field">
            <label :for="`${fieldPrefix}-traffic`">每月额度 (GB)</label>
            <input
              :id="`${fieldPrefix}-traffic`"
              v-model.number="trafficLimit"
              type="number"
              min="0"
              step="0.01"
              placeholder="不限"
            >
            <span class="field-hint">留空或 0 不限</span>
          </div>
          <div class="field">
            <label>计算方式</label>
            <AppSelect
              v-model="accounting"
              :options="accountingOptions"
              label="流量计算方式"
            />
          </div>
          <div class="field">
            <label :for="`${fieldPrefix}-reset`">每月重置日</label>
            <input
              :id="`${fieldPrefix}-reset`"
              v-model.number="resetDay"
              type="number"
              min="1"
              max="31"
              step="1"
            >
            <span class="field-hint">1–31，改后本月重算，总量不变</span>
          </div>
        </div>
      </section>

      <section class="form-section">
        <h3>地址与地区</h3>
        <div class="address-grid">
          <div class="field">
            <label :for="`${fieldPrefix}-ip`">IPv4</label>
            <input
              :id="`${fieldPrefix}-ip`"
              v-model="ip"
              type="text"
              :placeholder="`自动：${props.server?.observed_ip || '无'}`"
            >
          </div>
          <div class="field">
            <label :for="`${fieldPrefix}-ipv6`">IPv6</label>
            <input
              :id="`${fieldPrefix}-ipv6`"
              v-model="ipv6"
              type="text"
              placeholder="自动：无"
            >
          </div>
          <div class="field">
            <label :for="`${fieldPrefix}-region`">国家/地区</label>
            <input
              :id="`${fieldPrefix}-region`"
              v-model="region"
              type="text"
              maxlength="8"
              placeholder="自动：无"
            >
          </div>
        </div>
        <p class="field-hint">
          留空为自动。国家/地区填两位代码，如 CN；手填的值会一直显示，IP 变了要自己改。
        </p>
      </section>

      <div
        v-if="props.server"
        class="toggle-card"
      >
        <div class="toggle-row-text">
          <span class="toggle-row-label">启用状态</span>
          <span class="toggle-row-desc">禁用后停止向该服务器同步配置</span>
        </div>
        <ToggleSwitch
          :model-value="status === 'active'"
          label="启用服务器"
          @update:model-value="status = $event ? 'active' : 'disabled'"
        />
      </div>
    </div>
    <template #footer>
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
        {{ submitting ? '保存中…' : '保存' }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.server-form { display: flex; flex-direction: column; gap: var(--spacing-md); }
.top-grid, .toggle-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: var(--spacing-md); }
.traffic-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: var(--spacing-md); }
.address-grid { display: grid; grid-template-columns: 1fr 1.4fr 0.55fr; gap: var(--spacing-md); }
.form-section { padding-top: var(--spacing-md); border-top: 1px solid var(--color-border); }
.form-section h3 { margin: 0 0 var(--spacing-sm); font-size: var(--font-size-md); }
.toggle-card { display: flex; align-items: center; justify-content: space-between; gap: var(--spacing-sm); padding: var(--spacing-sm) var(--spacing-md); border: 1px solid var(--color-border); border-radius: var(--radius-lg); background: var(--color-surface-muted); }
.field-hint { color: var(--color-text-secondary); font-size: var(--font-size-xs); }
.form-section > .field-hint { margin: var(--spacing-sm) 0 0; }
@media (max-width: 700px) {
  .traffic-grid, .address-grid { grid-template-columns: 1fr 1fr; }
}
@media (max-width: 520px) {
  .top-grid, .toggle-grid, .traffic-grid, .address-grid { grid-template-columns: 1fr; }
}
</style>
