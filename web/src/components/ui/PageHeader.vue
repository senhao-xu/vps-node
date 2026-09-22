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
</script>

<template>
  <header class="page-header">
    <div class="heading">
      <RouterLink
        v-if="backTo"
        class="back-link"
        :to="backTo"
      >
        <ArrowLeft :size="13" />
        {{ backTitle }}
      </RouterLink>
      <h1 class="title">
        {{ title }}
      </h1>
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
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--spacing-md);
  margin-bottom: var(--spacing-lg);
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  margin-bottom: 2px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.back-link:hover {
  color: var(--color-text);
}

.title {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.02em;
}

.subtitle {
  margin: var(--spacing-xs) 0 0;
  font-size: var(--font-size-md);
  color: var(--color-text-secondary);
}

.header-actions {
  display: inline-flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--spacing-sm);
  flex: none;
}

@media (max-width: 900px) {
  .page-header {
    margin-bottom: var(--spacing-md);
  }
}

@media (max-width: 700px) {
  .page-header {
    flex-direction: column;
    align-items: stretch;
  }

  .header-actions {
    align-self: flex-start;
  }
}
</style>
