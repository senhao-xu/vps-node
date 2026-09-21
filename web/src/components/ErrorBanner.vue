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
  background: var(--color-danger-soft);
  border: 1px solid #fecaca;
  color: var(--color-danger);
  border-radius: var(--radius-md);
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
  flex: none;
  width: 28px;
  height: 28px;
  border-radius: 50%;
}
</style>
