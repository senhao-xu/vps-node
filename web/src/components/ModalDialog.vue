<script setup lang="ts">
defineProps<{
  open: boolean
  title: string
  width?: number
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="overlay"
      @mousedown.self="emit('close')"
    >
      <div
        class="dialog"
        :style="{ width: width ? `${width}px` : '480px' }"
      >
        <div class="dialog-header">
          <h2 class="dialog-title">
            {{ title }}
          </h2>
          <button
            type="button"
            class="close"
            aria-label="关闭"
            @click="emit('close')"
          >
            ×
          </button>
        </div>
        <div class="dialog-body">
          <slot />
        </div>
        <div
          v-if="$slots.footer"
          class="dialog-footer"
        >
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.4);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 80px var(--spacing-md) var(--spacing-lg);
  z-index: 100;
  overflow-y: auto;
}

.dialog {
  max-width: 100%;
  background: var(--color-surface);
  border-radius: var(--radius-md);
  box-shadow: 0 8px 30px rgba(0, 0, 0, 0.18);
}

.dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--spacing-md) var(--spacing-lg) 0;
}

.dialog-title {
  margin: 0;
  font-size: var(--font-size-lg);
}

.close {
  background: none;
  border: none;
  font-size: 20px;
  line-height: 1;
  color: var(--color-text-secondary);
  cursor: pointer;
  padding: 0;
}

.dialog-body {
  padding: var(--spacing-md) var(--spacing-lg);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  padding: 0 var(--spacing-lg) var(--spacing-md);
}
</style>
