<script setup lang="ts">
import ProgressBar from '@/components/ProgressBar.vue'

defineProps<{
  label: string
  percent: number
}>()
</script>

<template>
  <div class="metric">
    <span class="metric-label">{{ label }}</span>
    <ProgressBar :percent="percent" />
    <span class="metric-value">{{ percent.toFixed(0) }}%</span>
  </div>
</template>

<style scoped>
.metric {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.metric-label {
  width: 48px;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  flex-shrink: 0;
}

.metric-value {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  width: 42px;
  text-align: right;
  font-variant-numeric: tabular-nums;
}
</style>
