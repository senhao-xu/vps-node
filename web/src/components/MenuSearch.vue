<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { navigationItems, type NavigationItem } from '@/router/navigation'
import {
  isTopDialog,
  pushDialog,
  removeDialog,
  useFocusTrap,
  useScrollLock,
} from '@/components/ui/composables'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

const router = useRouter()
const query = ref('')
const activeIndex = ref(0)
const paletteRef = ref<HTMLElement | null>(null)
const isOpen = computed(() => props.open)
const dialogId = Symbol('menu-search')

useScrollLock(isOpen)
useFocusTrap(paletteRef, isOpen)

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return navigationItems
  return navigationItems.filter((item) => {
    const haystack = (item.label + ' ' + item.keywords).toLowerCase()
    return q.split(/\s+/).every((part) => haystack.includes(part))
  })
})

watch(
  () => props.open,
  (open) => {
    if (open) {
      pushDialog(dialogId)
      query.value = ''
      activeIndex.value = 0
    } else {
      removeDialog(dialogId)
    }
  },
)

watch(filtered, () => {
  activeIndex.value = 0
})

function go(item: NavigationItem) {
  emit('close')
  void router.push(item.to)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'ArrowDown') {
    event.preventDefault()
    activeIndex.value = Math.min(filtered.value.length - 1, activeIndex.value + 1)
  } else if (event.key === 'ArrowUp') {
    event.preventDefault()
    activeIndex.value = Math.max(0, activeIndex.value - 1)
  } else if (event.key === 'Enter') {
    event.preventDefault()
    const item = filtered.value[activeIndex.value]
    if (item) go(item)
  } else if (event.key === 'Escape') {
    if (!isTopDialog(dialogId)) return
    event.preventDefault()
    event.stopPropagation()
    emit('close')
  }
}

onBeforeUnmount(() => {
  removeDialog(dialogId)
})
</script>

<template>
  <Teleport to="body">
    <div
      v-if="props.open"
      class="palette-overlay"
      @mousedown.self="emit('close')"
    >
      <div
        ref="paletteRef"
        class="palette"
        role="dialog"
        aria-modal="true"
        aria-label="菜单搜索"
        tabindex="-1"
        @keydown="onKeydown"
      >
        <input
          v-model="query"
          type="text"
          class="palette-input"
          placeholder="搜索页面…"
          aria-label="搜索页面"
        >
        <div
          class="palette-list"
          role="listbox"
          aria-label="页面"
        >
          <button
            v-for="(item, index) in filtered"
            :key="item.to"
            type="button"
            class="palette-item"
            :class="{ active: index === activeIndex }"
            role="option"
            :aria-selected="index === activeIndex"
            @mouseenter="activeIndex = index"
            @click="go(item)"
          >
            <component
              :is="item.icon"
              :size="15"
              class="palette-icon"
            />
            <span>{{ item.label }}</span>
          </button>
          <p
            v-if="filtered.length === 0"
            class="palette-empty"
          >
            没有匹配的页面
          </p>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.palette-overlay {
  position: fixed;
  inset: 0;
  z-index: 300;
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding: 84px var(--spacing-md) var(--spacing-lg);
  background: var(--color-overlay);
}

.palette {
  width: min(440px, 100%);
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-dialog);
  background: var(--color-surface);
  box-shadow: var(--shadow-dialog);
  outline: none;
}

.palette-input {
  width: 100%;
  min-height: 48px;
  padding: 12px var(--spacing-md);
  border: none;
  border-bottom: 1px solid var(--color-border);
  border-radius: 0;
  background: var(--color-surface);
  font-size: var(--font-size-md);
}

.palette-list {
  max-height: 300px;
  overflow-y: auto;
  padding: var(--spacing-xs);
}

.palette-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  width: 100%;
  padding: 8px 10px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--color-text);
  font-size: var(--font-size-md);
  cursor: pointer;
  text-align: left;
}

.palette-item.active {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.palette-icon {
  flex: none;
  color: currentColor;
}

.palette-empty {
  margin: 0;
  padding: var(--spacing-lg);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-align: center;
}
</style>
