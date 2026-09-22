<script setup lang="ts" generic="T extends string | number">
import { computed, nextTick, ref, watch } from 'vue'
import { Check, ChevronDown } from 'lucide-vue-next'
import { useClickOutside, useFloatingPanel } from './composables'

const props = withDefaults(
  defineProps<{
    modelValue: T
    options: Array<{ value: T; label: string; dot?: string }>
    disabled?: boolean
    label?: string
  }>(),
  {
    disabled: false,
    label: undefined,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: T): void
}>()

const open = ref(false)
const activeIndex = ref(-1)
const { triggerRef, panelRef, panelStyle } = useFloatingPanel(open, { matchWidth: true })

useClickOutside([triggerRef, panelRef], () => {
  open.value = false
}, open)

const current = computed(() => props.options.find((option) => option.value === props.modelValue))

function openPanel() {
  if (props.disabled) return
  activeIndex.value = props.options.findIndex((option) => option.value === props.modelValue)
  open.value = true
}

function selectOption(index: number) {
  const option = props.options[index]
  if (!option) return
  emit('update:modelValue', option.value)
  open.value = false
  triggerRef.value?.focus()
}

function onTriggerKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown' || event.key === 'ArrowUp' || event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    if (open.value) return
    openPanel()
  } else if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    open.value = false
  }
}

function onPanelKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    activeIndex.value = Math.min(props.options.length - 1, activeIndex.value + 1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value = Math.max(0, activeIndex.value - 1)
  } else if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    selectOption(activeIndex.value)
  } else if (event.key === 'Escape') {
    event.preventDefault()
    open.value = false
    triggerRef.value?.focus()
  }
}

function togglePanel() {
  if (open.value) {
    open.value = false
    triggerRef.value?.focus()
  } else {
    openPanel()
  }
}

watch(
  open,
  async (isOpen) => {
    if (isOpen) {
      await nextTick()
      panelRef.value?.focus()
    }
  },
)
</script>

<template>
  <button
    ref="triggerRef"
    type="button"
    class="app-select"
    :class="{ open }"
    :disabled="props.disabled"
    :aria-label="props.label"
    aria-haspopup="listbox"
    :aria-expanded="open"
    @click="togglePanel"
    @keydown="onTriggerKeydown"
  >
    <span
      v-if="current?.dot"
      class="dot"
      :class="`dot-${current.dot}`"
    />
    <span class="value">{{ current?.label ?? '—' }}</span>
    <ChevronDown
      :size="14"
      class="chevron"
    />
  </button>
  <Teleport to="body">
    <div
      v-if="open"
      ref="panelRef"
      class="app-select-panel"
      role="listbox"
      tabindex="-1"
      :style="panelStyle"
      @keydown="onPanelKeydown"
    >
      <button
        v-for="(option, index) in props.options"
        :key="option.value"
        type="button"
        class="option"
        :class="{ active: index === activeIndex, selected: option.value === props.modelValue }"
        role="option"
        :aria-selected="option.value === props.modelValue"
        @mouseenter="activeIndex = index"
        @click="selectOption(index)"
      >
        <span
          v-if="option.dot"
          class="dot"
          :class="`dot-${option.dot}`"
        />
        <span class="option-label">{{ option.label }}</span>
        <Check
          v-if="option.value === props.modelValue"
          :size="14"
          class="check"
        />
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.app-select {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  width: 100%;
  min-height: var(--control-height);
  padding: 7px 11px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: var(--font-size-md);
  cursor: pointer;
  text-align: left;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}

.app-select:hover:not(:disabled) {
  border-color: var(--color-border-strong);
}

.app-select:disabled {
  background: var(--color-bg);
  color: var(--color-text-secondary);
  cursor: not-allowed;
}

.value {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chevron {
  flex: none;
  color: var(--color-text-secondary);
}

.app-select-panel {
  position: fixed;
  z-index: 200;
  min-width: 140px;
  max-height: 260px;
  overflow-y: auto;
  padding: var(--spacing-xs);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-dialog);
}

.option {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--color-text);
  font-size: var(--font-size-md);
  cursor: pointer;
  text-align: left;
}

.option.active {
  background: var(--color-primary-soft);
}

.option.selected {
  font-weight: 500;
}

.option-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.check {
  flex: none;
  color: var(--color-primary);
}

.dot {
  flex: none;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--color-text-secondary);
}

.dot.dot-primary {
  background: var(--color-primary);
}

.dot.dot-success {
  background: var(--color-success);
}

.dot.dot-warning {
  background: var(--color-warning);
}

.dot.dot-danger {
  background: var(--color-danger);
}

.dot.dot-muted {
  background: var(--color-offline);
}
</style>
