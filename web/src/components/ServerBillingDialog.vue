<script setup lang="ts">
import { ref, useId, watch } from 'vue'
import { updateServer } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { BillingCycle, Server } from '@/api/types'
import { formatDate } from '@/utils/format'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'

const props = defineProps<{
  open: boolean
  server: Server | null
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const fieldPrefix = `server-billing-${useId()}`
const price = ref<number | null>(null)
const currency = ref('USD')
const cycle = ref<BillingCycle>('monthly')
const expiry = ref('')
const submitting = ref(false)
const error = ref('')

const currencyOptions = [
  { value: 'USD', label: 'USD · 美元' },
  { value: 'EUR', label: 'EUR · 欧元' },
  { value: 'CNY', label: 'CNY · 人民币' },
  { value: 'HKD', label: 'HKD · 港币' },
  { value: 'JPY', label: 'JPY · 日元' },
  { value: 'SGD', label: 'SGD · 新加坡元' },
  { value: 'GBP', label: 'GBP · 英镑' },
] satisfies Array<{ value: string; label: string }>

const cycleOptions: Array<{ value: BillingCycle; label: string }> = [
  { value: 'monthly', label: '月付' },
  { value: 'quarterly', label: '季付' },
  { value: 'semiannual', label: '半年付' },
  { value: 'yearly', label: '年付' },
  { value: 'one_time', label: '一次性' },
]

watch(
  () => props.open,
  (open) => {
    if (!open || !props.server) return
    price.value = props.server.price_cents ? props.server.price_cents / 100 : null
    currency.value = props.server.price_currency || 'USD'
    cycle.value = props.server.billing_cycle || 'monthly'
    expiry.value = props.server.expires_at ? formatDate(props.server.expires_at) : ''
    error.value = ''
  },
)

async function submit() {
  if (!props.server) return
  const amount = Number(price.value ?? 0)
  if (!Number.isFinite(amount) || amount < 0) {
    error.value = '请填写有效的价格'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    await updateServer(props.server.id, {
      price_cents: Math.round(amount * 100),
      price_currency: currency.value,
      billing_cycle: cycle.value,
      expires_at: expiry.value ? new Date(`${expiry.value}T23:59:59`).toISOString() : '',
    })
    emit('saved')
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
    :title="props.server?.name ?? '费用与到期'"
    :width="620"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="billing-grid">
      <div class="field">
        <label :for="`${fieldPrefix}-price`">价格</label>
        <input
          :id="`${fieldPrefix}-price`"
          v-model.number="price"
          type="number"
          min="0"
          step="0.01"
          placeholder="0.00"
        >
        <span class="field-hint">留空或 0 为免费</span>
      </div>
      <div class="field">
        <label>货币</label>
        <AppSelect
          v-model="currency"
          :options="currencyOptions"
          label="货币"
        />
      </div>
      <div class="field">
        <label>付款周期</label>
        <AppSelect
          v-model="cycle"
          :options="cycleOptions"
          label="付款周期"
        />
      </div>
      <div class="field">
        <label :for="`${fieldPrefix}-expiry`">到期时间</label>
        <input
          :id="`${fieldPrefix}-expiry`"
          v-model="expiry"
          type="date"
        >
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
.billing-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--spacing-md);
}

.field-hint {
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
}

@media (max-width: 520px) {
  .billing-grid {
    grid-template-columns: 1fr;
  }
}
</style>
