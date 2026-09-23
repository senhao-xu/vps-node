<script setup lang="ts">
import { ref, type Component } from 'vue'
import { ChevronDown } from 'lucide-vue-next'
import { useClickOutside, useFloatingPanel } from './composables'

withDefaults(
  defineProps<{
    label: string
    icon?: Component
    active?: boolean
  }>(),
  {
    icon: undefined,
    active: false,
  },
)

const open = ref(false)
const { triggerRef, panelRef, panelStyle } = useFloatingPanel(open)

useClickOutside([triggerRef, panelRef], () => {
  open.value = false
}, open)

function close() {
  open.value = false
  triggerRef.value?.focus()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    close()
  }
}
</script>

<template>
  <button
    ref="triggerRef"
    type="button"
    class="filter-chip"
    :class="{ active, open }"
    aria-haspopup="true"
    :aria-expanded="open"
    @click="open = !open"
    @keydown="onKeydown"
  >
    <component
      :is="icon"
      v-if="icon"
      :size="13"
    />
    <span>{{ label }}</span>
    <ChevronDown
      :size="13"
      class="chevron"
    />
  </button>
  <Teleport to="body">
    <div
      v-if="open"
      ref="panelRef"
      class="chip-panel"
      :style="panelStyle"
      @keydown="onKeydown"
    >
      <slot :close="close" />
    </div>
  </Teleport>
</template>

<style scoped>
.filter-chip {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-height: 32px;
  padding: 4px 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  cursor: pointer;
  white-space: nowrap;
  transition: border-color 0.15s ease, background 0.15s ease, color 0.15s ease;
}

.filter-chip:hover {
  border-color: var(--color-border-strong);
}

.filter-chip.active,
.filter-chip.open {
  border-color: var(--color-border-strong);
  background: var(--color-muted-soft);
  color: var(--color-text);
}

.chevron {
  flex: none;
  color: var(--color-text-secondary);
  transition: transform 0.15s ease;
}

.filter-chip.open .chevron {
  transform: rotate(180deg);
}

.chip-panel {
  position: fixed;
  z-index: 200;
  min-width: 160px;
  padding: 6px;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-dialog);
}
</style>
