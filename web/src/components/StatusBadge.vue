<script setup lang="ts">
import { computed } from 'vue'
import type { Tone } from '@/utils/labels'

const props = defineProps<{
  label: string
  tone?: Tone
}>()

const toneClass = computed(() => props.tone ?? 'muted')
</script>

<template>
  <span
    class="badge"
    :class="toneClass"
  >{{ props.label }}</span>
</template>

<style scoped>
.badge {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 2px 7px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-xs);
  background: var(--color-surface-muted);
  font-size: var(--font-size-xs);
  font-weight: 650;
  white-space: nowrap;
}

.badge.success {
  border-color: var(--color-success-border);
  background: var(--color-success-soft);
  color: var(--color-success);
}

.badge.warning {
  border-color: var(--color-warning-border);
  background: var(--color-warning-soft);
  color: var(--color-warning);
}

.badge.danger {
  border-color: var(--color-danger-border);
  background: var(--color-danger-soft);
  color: var(--color-danger);
}

.badge.primary {
  border-color: var(--color-primary-border);
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.badge.muted {
  border-color: var(--color-border);
  color: var(--color-text-secondary);
}
</style>
