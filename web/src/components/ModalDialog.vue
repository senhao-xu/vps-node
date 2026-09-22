<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useId, watch } from 'vue'
import { X } from 'lucide-vue-next'
import {
  isTopDialog,
  pushDialog,
  removeDialog,
  useFocusTrap,
  useScrollLock,
} from '@/components/ui/composables'

const props = withDefaults(
  defineProps<{
    open: boolean
    title: string
    subtitle?: string
    width?: number
  }>(),
  {
    subtitle: undefined,
    width: undefined,
  },
)

const emit = defineEmits<{
  (e: 'close'): void
}>()

const titleId = `modal-title-${useId()}`
const dialogId = Symbol('modal-dialog')
const dialogEl = ref<HTMLElement | null>(null)

const isOpen = computed(() => props.open)
useFocusTrap(dialogEl, isOpen)
useScrollLock(isOpen)

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  if (!isTopDialog(dialogId)) return
  event.stopPropagation()
  emit('close')
}

watch(
  () => props.open,
  (open) => {
    if (open) {
      pushDialog(dialogId)
      window.addEventListener('keydown', onKeydown)
    } else {
      removeDialog(dialogId)
      window.removeEventListener('keydown', onKeydown)
    }
  },
)

onBeforeUnmount(() => {
  removeDialog(dialogId)
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      class="overlay"
      @mousedown.self="emit('close')"
    >
      <div
        ref="dialogEl"
        class="dialog"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="titleId"
        tabindex="-1"
        :style="{ width: width ? `${width}px` : '480px' }"
      >
        <div class="dialog-header">
          <div class="dialog-heading">
            <h2
              :id="titleId"
              class="dialog-title"
            >
              {{ title }}
            </h2>
            <p
              v-if="props.subtitle"
              class="dialog-subtitle"
            >
              {{ props.subtitle }}
            </p>
          </div>
          <button
            type="button"
            class="close"
            aria-label="关闭"
            @click="emit('close')"
          >
            <X :size="16" />
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
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-dialog);
  overflow: hidden;
  outline: none;
}

.dialog-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding: 20px var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
}

.dialog-title {
  margin: 0;
  font-size: var(--font-size-lg);
  letter-spacing: -0.01em;
}

.dialog-subtitle {
  margin: var(--spacing-xs) 0 0;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.close {
  background: var(--color-muted-soft);
  border: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  display: inline-flex;
  width: 32px;
  height: 32px;
  flex: none;
  align-items: center;
  justify-content: center;
  padding: 0;
  border-radius: 50%;
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
