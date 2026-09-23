<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  Globe,
  LayoutDashboard,
  Server,
  Settings,
  Users,
  Waypoints,
  type LucideIcon,
} from 'lucide-vue-next'
import { useScrollLock } from '@/components/ui/composables'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (e: 'close'): void
}>()

interface MenuItem {
  to: string
  label: string
  keywords: string
  icon: LucideIcon
}

const menuItems: MenuItem[] = [
  { to: '/', label: '仪表盘', keywords: 'dashboard yibiaopan home', icon: LayoutDashboard },
  { to: '/users', label: '用户', keywords: 'users yonghu', icon: Users },
  { to: '/servers', label: '服务器', keywords: 'servers fuwuqi', icon: Server },
  { to: '/nodes', label: '节点', keywords: 'nodes jiedian', icon: Waypoints },
  { to: '/visits', label: '访问记录', keywords: 'visits fangwen jilu sites', icon: Globe },
  { to: '/settings', label: '设置', keywords: 'settings shezhi', icon: Settings },
]

const router = useRouter()
const query = ref('')
const activeIndex = ref(0)
const inputRef = ref<HTMLInputElement | null>(null)

const isOpen = computed(() => props.open)
useScrollLock(isOpen)

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return menuItems
  return menuItems.filter((item) => {
    const haystack = `${item.label} ${item.keywords}`.toLowerCase()
    return q.split(/\s+/).every((part) => haystack.includes(part))
  })
})

watch(
  () => props.open,
  async (open) => {
    if (!open) return
    query.value = ''
    activeIndex.value = 0
    await nextTick()
    inputRef.value?.focus()
  },
)

watch(filtered, () => {
  activeIndex.value = 0
})

function go(item: MenuItem) {
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
    event.preventDefault()
    emit('close')
  }
}
</script>

<template>
  <Teleport to="body">
    <div
      v-if="props.open"
      class="palette-overlay"
      @mousedown.self="emit('close')"
      @keydown="onKeydown"
    >
      <div
        class="palette"
        role="dialog"
        aria-modal="true"
        aria-label="菜单搜索"
      >
        <input
          ref="inputRef"
          v-model="query"
          type="text"
          class="palette-input"
          placeholder="搜索页面…"
          aria-label="搜索页面"
        >
        <div
          class="palette-list"
          role="listbox"
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
  padding: 96px var(--spacing-md) var(--spacing-lg);
  background: var(--color-overlay);
}

.palette {
  width: min(440px, 100%);
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-dialog);
  box-shadow: var(--shadow-dialog);
  overflow: hidden;
}

.palette-input {
  width: 100%;
  border: none;
  border-bottom: 1px solid var(--color-border);
  border-radius: 0;
  padding: 14px var(--spacing-md);
  font-size: var(--font-size-md);
  background: var(--color-surface);
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
  padding: 9px 10px;
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
}

.palette-icon {
  flex: none;
  color: var(--color-text-secondary);
}

.palette-empty {
  margin: 0;
  padding: var(--spacing-lg);
  text-align: center;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}
</style>
