<script setup lang="ts">
import type { Component } from 'vue'

withDefaults(
  defineProps<{
    title: string
    hint?: string
    icon?: Component
  }>(),
  {
    hint: undefined,
    icon: undefined,
  },
)
</script>

<template>
  <div class="empty-state">
    <component
      :is="icon"
      v-if="icon"
      :size="28"
      class="empty-icon"
    />
    <p class="empty-title">
      {{ title }}
    </p>
    <p
      v-if="hint"
      class="empty-hint"
    >
      {{ hint }}
    </p>
    <div
      v-if="$slots.action"
      class="empty-action"
    >
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
  padding: var(--spacing-xl) var(--spacing-md);
  text-align: center;
}

.empty-icon {
  color: var(--color-text-secondary);
  opacity: 0.6;
  margin-bottom: var(--spacing-xs);
}

.empty-title {
  margin: 0;
  font-size: var(--font-size-md);
  font-weight: 500;
  color: var(--color-text);
}

.empty-hint {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.empty-action {
  margin-top: var(--spacing-sm);
}
</style>
