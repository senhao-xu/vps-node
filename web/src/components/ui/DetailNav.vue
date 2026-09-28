<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import type { LucideIcon } from 'lucide-vue-next'

export type DetailNavItem = {
  id: string
  label: string
  hint?: string
  icon: LucideIcon
}

const props = defineProps<{
  items: DetailNavItem[]
  ariaLabel?: string
}>()

const activeId = ref('')
function syncHash() {
  const hash = window.location.hash.slice(1)
  activeId.value = props.items.some((item) => item.id === hash) ? hash : (props.items[0]?.id ?? '')
}
onMounted(() => {
  syncHash()
  window.addEventListener('hashchange', syncHash)
})
onBeforeUnmount(() => window.removeEventListener('hashchange', syncHash))
</script>

<template>
  <nav
    class="detail-nav"
    :aria-label="ariaLabel ?? '详情分区'"
  >
    <a
      v-for="item in items"
      :key="item.id"
      class="detail-nav-item"
      :class="{ active: activeId === item.id }"
      :aria-current="activeId === item.id ? 'location' : undefined"
      :href="`#${item.id}`"
      @click="activeId = item.id"
    >
      <component
        :is="item.icon"
        :size="16"
        aria-hidden="true"
      />
      <strong>{{ item.label }}</strong>
    </a>
  </nav>
</template>

<style scoped>
.detail-nav {
  position: sticky;
  top: calc(var(--shell-toolbar-height) + 8px);
  z-index: 20;
  display: flex;
  min-width: 0;
  margin-bottom: 14px;
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--color-surface) 94%, transparent);
  box-shadow: var(--shadow-sm);
  scrollbar-width: thin;
  backdrop-filter: blur(10px);
}

.detail-nav-item {
  display: flex;
  min-width: max-content;
  flex: 1 0 auto;
  align-items: center;
  gap: 9px;
  padding: 12px 18px;
  border-left: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  transition: background 0.15s ease, color 0.15s ease;
}

.detail-nav-item:first-child {
  border-left: 0;
}

.detail-nav-item:hover,
.detail-nav-item.active {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.detail-nav-item svg {
  flex: none;
}

.detail-nav-item strong {
  font-size: var(--font-size-sm);
  font-weight: 700;
  line-height: 1.35;
  white-space: nowrap;
}

.detail-nav-item.active {
  box-shadow: inset 0 -3px var(--color-primary);
}

@media (max-width: 700px) {
  .detail-nav {
    top: calc(var(--shell-toolbar-height) + 4px);
    margin-inline: -4px;
  }

  .detail-nav-item {
    padding-inline: 14px;
  }
}
</style>
