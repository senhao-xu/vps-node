<script setup lang="ts">
import { RouterLink } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'

withDefaults(
  defineProps<{
    title: string
    subtitle?: string
    backTo?: string
    backTitle?: string
  }>(),
  {
    subtitle: undefined,
    backTo: undefined,
    backTitle: undefined,
  },
)
defineSlots<{
  'title-extra'?: () => unknown
  actions?: () => unknown
}>()

</script>

<template>
  <header class="page-header">
    <div class="heading">
      <RouterLink
        v-if="backTo"
        class="back-command"
        :to="backTo"
      >
        <ArrowLeft
          :size="15"
          aria-hidden="true"
        />
        <span>{{ backTitle || '返回' }}</span>
      </RouterLink>
      <div class="title-row">
        <h1 class="title">
          {{ title }}
        </h1>
        <span
          v-if="$slots['title-extra']"
          class="title-extra"
        >
          <slot name="title-extra" />
        </span>
      </div>
      <p
        v-if="subtitle"
        class="subtitle"
      >
        {{ subtitle }}
      </p>
    </div>
    <div
      v-if="$slots.actions"
      class="header-actions"
    >
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--spacing-md);
  margin-bottom: 18px;
  padding-bottom: 16px;
  border-bottom: 1px solid var(--color-border);
}

.heading {
  min-width: 0;
}

.back-command {
  display: inline-flex;
  max-width: 100%;
  min-height: 32px;
  align-items: center;
  gap: 6px;
  margin-bottom: 9px;
  padding: 4px 10px;
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  box-shadow: var(--shadow-sm);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  font-weight: 650;
  line-height: 1.35;
}

.back-command svg {
  flex: none;
}

.back-command span {
  min-width: 0;
  overflow-wrap: anywhere;
}

.back-command:hover {
  border-color: var(--color-primary-border);
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.title-row {
  display: flex;
  min-width: 0;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

.title {
  min-width: 0;
  margin: 0;
  overflow-wrap: anywhere;
  font-size: 25px;
  font-weight: 780;
  letter-spacing: 0;
  line-height: 1.25;
}

.title-extra {
  display: inline-flex;
  flex: none;
  align-items: center;
}

.subtitle {
  max-width: 100%;
  margin: 6px 0 0;
  overflow-wrap: anywhere;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.header-actions {
  display: inline-flex;
  align-items: center;
  flex: none;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: var(--spacing-sm);
}

@media (max-width: 560px) {
  .page-header {
    align-items: stretch;
    flex-direction: column;
  }

  .header-actions {
    align-self: flex-start;
    justify-content: flex-start;
  }
}
</style>
