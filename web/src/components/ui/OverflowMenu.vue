<script setup lang="ts">
import { ref, watch, nextTick } from 'vue'
import { MoreHorizontal } from 'lucide-vue-next'
import { useClickOutside, useFloatingPanel } from './composables'

export interface OverflowMenuItem {
  label: string
  danger?: boolean
  onSelect: () => void
}

const props = withDefaults(
  defineProps<{
    items: OverflowMenuItem[]
    label?: string
  }>(),
  {
    label: '更多操作',
  },
)

const open = ref(false)
const activeIndex = ref(-1)
const { triggerRef, panelRef, panelStyle } = useFloatingPanel(open, { align: 'end' })

useClickOutside([triggerRef, panelRef], () => {
  open.value = false
}, open)

function togglePanel() {
  if (props.items.length === 0) return
  activeIndex.value = open.value ? -1 : 0
  open.value = !open.value
}

function choose(index: number) {
  const item = props.items[index]
  open.value = false
  triggerRef.value?.focus()
  item?.onSelect()
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    if (!open.value) togglePanel()
    else activeIndex.value = Math.min(props.items.length - 1, activeIndex.value + 1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value = Math.max(0, activeIndex.value - 1)
  } else if (event.key === 'Enter' || event.key === ' ') {
    if (open.value) {
      event.preventDefault()
      choose(activeIndex.value)
    }
  } else if (event.key === 'Escape' && open.value) {
    event.preventDefault()
    open.value = false
    triggerRef.value?.focus()
  }
}

watch(open, async (isOpen) => {
  if (isOpen) {
    await nextTick()
    panelRef.value?.focus()
  }
})
</script>

<template>
  <button
    ref="triggerRef"
    type="button"
    class="menu-trigger"
    :aria-label="props.label"
    aria-haspopup="menu"
    :aria-expanded="open"
    @click="togglePanel"
    @keydown="onKeydown"
  >
    <MoreHorizontal :size="16" />
  </button>
  <Teleport to="body">
    <div
      v-if="open"
      ref="panelRef"
      class="menu-panel"
      role="menu"
      tabindex="-1"
      :style="panelStyle"
      @keydown="onKeydown"
    >
      <button
        v-for="(item, index) in props.items"
        :key="item.label"
        type="button"
        class="menu-item"
        :class="{ active: index === activeIndex, danger: item.danger }"
        role="menuitem"
        @mouseenter="activeIndex = index"
        @click="choose(index)"
      >
        {{ item.label }}
      </button>
    </div>
  </Teleport>
</template>

<style scoped>
.menu-trigger {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.menu-trigger:hover {
  background: var(--color-primary-soft);
  color: var(--color-text);
}

.menu-panel {
  position: fixed;
  z-index: 200;
  min-width: 128px;
  padding: var(--spacing-xs);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-dialog);
}

.menu-item {
  display: block;
  width: 100%;
  padding: 7px 10px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--color-text);
  font-size: var(--font-size-md);
  cursor: pointer;
  text-align: left;
  white-space: nowrap;
}

.menu-item.active {
  background: var(--color-primary-soft);
}

.menu-item.danger {
  color: var(--color-danger);
}

.menu-item.danger.active {
  background: var(--color-danger-soft);
}
</style>
