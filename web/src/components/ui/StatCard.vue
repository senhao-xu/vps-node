<script setup lang="ts">
import type { Component } from 'vue'

withDefaults(
  defineProps<{
    label: string
    value: string | number
    icon: Component
    hint?: string
    tone?: 'default' | 'success' | 'warning' | 'danger'
  }>(),
  {
    hint: undefined,
    tone: 'default',
  },
)
</script>

<template>
  <div class="stat-card card">
    <div class="stat-head">
      <span class="stat-label">{{ label }}</span>
      <component
        :is="icon"
        :size="16"
        class="stat-icon"
        :class="`tone-${tone}`"
      />
    </div>
    <div class="stat-value">
      {{ value }}
    </div>
    <div
      v-if="hint"
      class="stat-hint"
    >
      {{ hint }}
    </div>
  </div>
</template>

<style scoped>
.stat-card {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: 16px 20px;
}

.stat-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.stat-label {
  font-size: var(--font-size-sm);
  font-weight: 500;
  color: var(--color-text-secondary);
}

.stat-icon {
  flex: none;
  color: var(--color-text-secondary);
}

.stat-icon.tone-success {
  color: var(--color-success);
}

.stat-icon.tone-warning {
  color: var(--color-warning);
}

.stat-icon.tone-danger {
  color: var(--color-danger);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
}

.stat-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}
</style>
