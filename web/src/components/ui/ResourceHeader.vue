<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ArrowLeft, type LucideIcon } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    title: string
    eyebrow: string
    subtitle?: string
    backTo: string
    backLabel: string
    icon: LucideIcon
    monogram?: string
  }>(),
  {
    subtitle: undefined,
    monogram: undefined,
  },
)

defineSlots<{
  status?: () => unknown
  meta?: () => unknown
  actions?: () => unknown
}>()
</script>

<template>
  <header class="resource-header">
    <RouterLink
      class="resource-back"
      :to="backTo"
    >
      <ArrowLeft :size="15" />
      <span>{{ backLabel }}</span>
    </RouterLink>

    <div class="resource-header-main">
      <div class="resource-identity">
        <span
          class="resource-icon"
          aria-hidden="true"
        >
          <component
            :is="icon"
            :size="22"
          />
          <span v-if="monogram">{{ monogram }}</span>
        </span>
        <div class="resource-copy">
          <span class="resource-eyebrow">{{ eyebrow }}</span>
          <div class="resource-title-row">
            <h1>{{ title }}</h1>
            <slot name="status" />
          </div>
          <p v-if="subtitle">
            {{ subtitle }}
          </p>
        </div>
      </div>

      <div
        v-if="$slots.actions"
        class="resource-actions"
      >
        <slot name="actions" />
      </div>
    </div>

    <div
      v-if="$slots.meta"
      class="resource-meta"
    >
      <slot name="meta" />
    </div>
  </header>
</template>

<style scoped>
.resource-header {
  position: relative;
  margin-bottom: 14px;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-top: 3px solid var(--color-primary);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}

.resource-back {
  display: inline-flex;
  min-height: 34px;
  align-items: center;
  gap: 6px;
  margin: 12px 18px 0;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.resource-back:hover {
  color: var(--color-primary);
}

.resource-header-main {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-lg);
  padding: 12px 20px 20px;
}

.resource-identity {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 14px;
}

.resource-icon {
  display: grid;
  width: 52px;
  height: 52px;
  flex: none;
  place-items: center;
  border: 1px solid var(--color-primary-border);
  border-radius: var(--radius-md);
  background: var(--color-primary-soft);
  color: var(--color-primary);
  font-size: 12px;
  font-weight: 800;
}

.resource-icon span {
  display: none;
}

.resource-copy {
  min-width: 0;
}

.resource-eyebrow {
  display: block;
  margin-bottom: 3px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 700;
  text-transform: uppercase;
}

.resource-title-row {
  display: flex;
  min-width: 0;
  align-items: center;
  flex-wrap: wrap;
  gap: 9px;
}

.resource-title-row h1 {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
  font-size: 27px;
  font-weight: 780;
  line-height: 1.2;
}

.resource-copy p {
  margin: 5px 0 0;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.resource-actions {
  display: inline-flex;
  flex: none;
  align-items: center;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}

.resource-meta {
  display: flex;
  min-width: 0;
  align-items: stretch;
  flex-wrap: wrap;
  border-top: 1px solid var(--color-border);
  background: var(--color-surface-muted);
}

.resource-meta :deep(.resource-meta-item) {
  display: flex;
  min-width: 150px;
  flex: 1 1 180px;
  align-items: center;
  gap: 8px;
  padding: 10px 18px;
  border-left: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.resource-meta :deep(.resource-meta-item:first-child) {
  border-left: 0;
}

.resource-meta :deep(.resource-meta-item svg) {
  flex: none;
  color: var(--color-primary);
}

.resource-meta :deep(.resource-meta-item strong) {
  color: var(--color-text);
  font-weight: 650;
}

@media (max-width: 700px) {
  .resource-header-main {
    align-items: stretch;
    flex-direction: column;
    padding-inline: var(--spacing-md);
  }

  .resource-back {
    margin-inline: var(--spacing-md);
  }

  .resource-title-row h1 {
    font-size: 23px;
  }

  .resource-actions {
    justify-content: flex-start;
  }

  .resource-meta :deep(.resource-meta-item) {
    min-width: 50%;
    padding-inline: var(--spacing-md);
  }
}

@media (max-width: 420px) {
  .resource-identity {
    align-items: flex-start;
  }

  .resource-icon {
    width: 42px;
    height: 42px;
  }

  .resource-meta :deep(.resource-meta-item) {
    min-width: 100%;
    border-top: 1px solid var(--color-border);
    border-left: 0;
  }

  .resource-meta :deep(.resource-meta-item:first-child) {
    border-top: 0;
  }
}
</style>
