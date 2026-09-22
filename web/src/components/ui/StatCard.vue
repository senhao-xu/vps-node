<script setup lang="ts">
import type { Component } from 'vue'

withDefaults(
  defineProps<{
    label: string
    value: string | number
    icon: Component
    tone?: 'default' | 'success' | 'warning' | 'danger'
  }>(),
  {
    tone: 'default',
  },
)
</script>

<template>
  <div class="stat-card card">
    <div class="stat-text">
      <span class="stat-label">{{ label }}</span>
      <span class="stat-value">{{ value }}</span>
    </div>
    <span
      class="stat-icon"
      :class="`tone-${tone}`"
    >
      <component
        :is="icon"
        :size="18"
      />
    </span>
  </div>
</template>

<style scoped>
.stat-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.stat-text {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  min-width: 0;
}

.stat-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.stat-value {
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.02em;
  line-height: 1.2;
}

.stat-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  width: 40px;
  height: 40px;
  border-radius: var(--radius-md);
  color: var(--color-text-secondary);
  background: var(--color-muted-soft);
}

.stat-icon.tone-success {
  color: var(--color-success);
  background: var(--color-success-soft);
}

.stat-icon.tone-warning {
  color: var(--color-warning);
  background: var(--color-warning-soft);
}

.stat-icon.tone-danger {
  color: var(--color-danger);
  background: var(--color-danger-soft);
}
</style>
