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
  padding: 2px 10px;
  border: 1px solid transparent;
  border-radius: var(--radius-full);
  background: var(--color-surface);
  font-size: var(--font-size-sm);
  white-space: nowrap;
}

.badge.success {
  color: var(--color-success);
  border-color: var(--color-success-border);
}

.badge.warning {
  color: var(--color-warning);
  border-color: var(--color-warning-border);
}

.badge.danger {
  color: var(--color-danger);
  border-color: var(--color-danger-border);
}

.badge.primary {
  color: var(--color-primary);
  border-color: var(--color-primary-border);
}

.badge.muted {
  color: var(--color-text);
  border-color: var(--color-border-strong);
}
</style>
