<script setup lang="ts">
import type { Tone } from '@/utils/labels'

const props = defineProps<{
  label: string
  tone?: Tone
}>()

const toneClass = props.tone ?? 'muted'
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
  padding: 1px 8px;
  border-radius: 999px;
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.badge.success {
  color: var(--color-success);
  background: rgba(22, 163, 74, 0.1);
}

.badge.warning {
  color: var(--color-warning);
  background: rgba(217, 119, 6, 0.12);
}

.badge.danger {
  color: var(--color-danger);
  background: rgba(220, 38, 38, 0.1);
}

.badge.primary {
  color: var(--color-primary);
  background: rgba(37, 99, 235, 0.1);
}

.badge.muted {
  color: var(--color-text-secondary);
  background: rgba(144, 147, 153, 0.14);
}
</style>
