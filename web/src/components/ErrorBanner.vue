<script setup lang="ts">
defineProps<{
  message: string
}>()

const emit = defineEmits<{
  (e: 'dismiss'): void
}>()
</script>

<template>
  <div
    v-if="message"
    class="error-banner"
  >
    <span>{{ message }}</span>
    <button
      type="button"
      class="close"
      aria-label="关闭"
      @click="emit('dismiss')"
    >
      ×
    </button>
  </div>
</template>

<style scoped>
.error-banner {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  background: rgba(220, 38, 38, 0.08);
  border: 1px solid rgba(220, 38, 38, 0.35);
  color: var(--color-danger);
  border-radius: var(--radius-sm);
  padding: var(--spacing-sm) var(--spacing-md);
  margin-bottom: var(--spacing-md);
}

.close {
  background: none;
  border: none;
  color: inherit;
  font-size: 16px;
  cursor: pointer;
  line-height: 1;
  padding: 0;
}
</style>
