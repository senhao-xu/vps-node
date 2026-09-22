<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    subtitle?: string
  }>(),
  {
    subtitle: undefined,
  },
)
</script>

<template>
  <header class="page-header">
    <div class="heading">
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

  .heading {
    display: none;
  }

  .page-header:not(:has(.header-actions)) {
    display: none;
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
