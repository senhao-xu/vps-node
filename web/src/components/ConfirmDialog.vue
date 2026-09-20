<script setup lang="ts">
import { computed } from 'vue'
import ModalDialog from '@/components/ModalDialog.vue'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    message: string
    danger?: boolean
    confirmText?: string
    loading?: boolean
  }>(),
  {
    danger: false,
    confirmText: '确认',
    loading: false,
  },
)

const emit = defineEmits<{
  (e: 'confirm'): void
  (e: 'cancel'): void
}>()

const lines = computed(() => props.message.split('\n'))
</script>

<template>
  <ModalDialog
    :open="props.open"
    :title="props.title"
    :width="420"
    @close="emit('cancel')"
  >
    <div class="message">
      <p
        v-for="(line, index) in lines"
        :key="index"
        class="line"
      >
        {{ line }}
      </p>
    </div>
    <template #footer>
      <button
        type="button"
        class="btn secondary"
        :disabled="props.loading"
        @click="emit('cancel')"
      >
        取消
      </button>
      <button
        type="button"
        class="btn"
        :class="props.danger ? 'danger' : ''"
        :disabled="props.loading"
        @click="emit('confirm')"
      >
        {{ props.loading ? '处理中…' : props.confirmText }}
      </button>
    </template>
  </ModalDialog>
</template>

<style scoped>
.message .line {
  margin: 0 0 var(--spacing-xs);
}
</style>
