<script setup lang="ts">
import { ref, watch } from 'vue'
import { createServer, updateServer } from '@/api/servers'
import { errorMessage } from '@/api/http'
import type { Server, ServerStatus } from '@/api/types'
import ErrorBanner from '@/components/ErrorBanner.vue'
import ModalDialog from '@/components/ModalDialog.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    server?: Server | null
  }>(),
  { server: null },
)

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'saved'): void
}>()

const name = ref('')
const address = ref('')
const status = ref<ServerStatus>('active')
const submitting = ref(false)
const error = ref('')

const isEdit = ref(false)

watch(
  () => props.open,
  (open) => {
    if (!open) return
    error.value = ''
    isEdit.value = props.server !== null
    name.value = props.server?.name ?? ''
    address.value = props.server?.address ?? ''
    status.value = props.server?.status === 'disabled' ? 'disabled' : 'active'
  },
)

async function submit() {
  if (!name.value.trim() || !address.value.trim()) {
    error.value = '请填写名称和地址'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    if (isEdit.value && props.server) {
      await updateServer(props.server.id, {
        name: name.value.trim(),
        address: address.value.trim(),
        status: status.value,
      })
    } else {
      await createServer({ name: name.value.trim(), address: address.value.trim() })
    }
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
    :title="isEdit ? '编辑服务器' : '创建服务器'"
    :subtitle="isEdit ? '修改服务器名称、地址与同步状态' : '创建后在服务器上安装 Agent 完成接入'"
    :width="460"
    @close="emit('close')"
  >
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />
    <div class="form">
      <div class="field">
        <label for="server-name">名称</label>
        <input
          id="server-name"
          v-model="name"
          type="text"
          placeholder="例如 HK-01"
        >
      </div>
      <div class="field">
        <label for="server-address">地址</label>
        <input
          id="server-address"
          v-model="address"
          type="text"
          placeholder="例如 hk01.example.com"
        >
      </div>
      <div
        v-if="isEdit"
        class="toggle-row"
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
.form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
}
</style>
