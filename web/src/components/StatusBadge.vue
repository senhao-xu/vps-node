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
  padding: 2px 9px;
  border: 1px solid transparent;
  border-radius: 999px;
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.badge.success {
  color: var(--color-success);
  border-color: var(--color-success-border);
  background: var(--color-success-soft);
}

.badge.warning {
  color: var(--color-warning);
  border-color: var(--color-warning-border);
  background: var(--color-warning-soft);
}

.badge.danger {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
  background: var(--color-danger-soft);
}

.badge.primary {
  color: var(--color-primary);
  border-color: var(--color-primary-border);
  background: var(--color-primary-soft);
}

.badge.muted {
  color: var(--color-text-secondary);
  border-color: var(--color-border);
  background: var(--color-muted-soft);
}
</style>
