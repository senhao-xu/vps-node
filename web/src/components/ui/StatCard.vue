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

/* TDesign stat tile: tinted rounded icon chip inside the card */
.stat-icon {
  flex: none;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  border-radius: var(--radius-sm);
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.stat-icon.tone-success {
  background: var(--color-success-soft);
  color: var(--color-success);
}

.stat-icon.tone-warning {
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.stat-icon.tone-danger {
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
  font-variant-numeric: tabular-nums;
}

.stat-hint {
  font-size: var(--font-size-xs);
  color: var(--color-text-secondary);
}
</style>
