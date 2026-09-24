<script setup lang="ts">
import type { LucideIcon } from 'lucide-vue-next'

export type DetailNavItem = {
  id: string
  label: string
  hint?: string
  icon: LucideIcon
}

defineProps<{
  items: DetailNavItem[]
  ariaLabel?: string
}>()
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
      :href="`#${item.id}`"
    >
      <component
        :is="item.icon"
        :size="16"
        aria-hidden="true"
      />
      <span>
        <strong>{{ item.label }}</strong>
        <small v-if="item.hint">{{ item.hint }}</small>
      </span>
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
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--color-surface) 94%, transparent);
  box-shadow: var(--shadow-sm);
  scrollbar-width: thin;
  backdrop-filter: blur(10px);
}

.detail-nav-item {
  display: flex;
  min-width: 150px;
  flex: 1 0 auto;
  align-items: center;
  gap: 9px;
  padding: 10px 14px;
  border-left: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  transition: background 0.15s ease, color 0.15s ease;
}

.detail-nav-item:first-child {
  border-left: 0;
}

.detail-nav-item:hover {
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.detail-nav-item svg {
  flex: none;
}

.detail-nav-item span {
  display: flex;
  min-width: 0;
  flex-direction: column;
}

.detail-nav-item strong {
  color: var(--color-text);
  font-size: var(--font-size-sm);
  font-weight: 700;
  line-height: 1.35;
  white-space: nowrap;
}

.detail-nav-item small {
  overflow: hidden;
  font-size: var(--font-size-xs);
  line-height: 1.35;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 700px) {
  .detail-nav {
    top: calc(var(--shell-toolbar-height) + 4px);
    margin-inline: -4px;
  }

  .detail-nav-item {
    min-width: 128px;
  }

  .detail-nav-item small {
    display: none;
  }
}
</style>
