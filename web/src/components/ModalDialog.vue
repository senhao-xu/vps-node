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
        role="dialog"
        aria-modal="true"
        aria-labelledby="modal-dialog-title"
        :style="{ width: width ? `${width}px` : '480px' }"
      >
        <div class="dialog-header">
          <h2
            id="modal-dialog-title"
            class="dialog-title"
          >
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
  background: var(--color-overlay);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 64px var(--spacing-md) var(--spacing-lg);
  z-index: 100;
  overflow-y: auto;
  backdrop-filter: blur(3px);
}

.dialog {
  max-width: 100%;
  background: var(--color-surface);
  border: 1px solid rgba(255, 255, 255, 0.72);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-dialog);
  overflow: hidden;
}

.dialog-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 20px var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.dialog-title {
  margin: 0;
  font-size: var(--font-size-lg);
  letter-spacing: -0.01em;
}

.close {
  background: none;
  border: none;
  font-size: 20px;
  line-height: 1;
  color: var(--color-text-secondary);
  cursor: pointer;
  display: inline-flex;
  width: 32px;
  height: 32px;
  align-items: center;
  justify-content: center;
  padding: 0;
  border-radius: 50%;
  background: var(--color-muted-soft);
  transition: color 0.15s ease, background 0.15s ease;
}

.close:hover {
  color: var(--color-text);
  background: var(--color-border);
}

.dialog-body {
  padding: var(--spacing-lg);
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  border-top: 1px solid var(--color-border);
  background: var(--color-surface-muted);
}

@media (max-width: 560px) {
  .overlay {
    padding: var(--spacing-sm);
  }

  .dialog {
    width: 100% !important;
  }

  .dialog-header,
  .dialog-body,
  .dialog-footer {
    padding-right: var(--spacing-md);
    padding-left: var(--spacing-md);
  }

  .dialog-footer {
    flex-direction: column-reverse;
  }

  .dialog-footer :deep(.btn) {
    width: 100%;
  }
}
</style>
