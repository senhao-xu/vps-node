<script setup lang="ts" generic="T extends string">
const props = defineProps<{
  items: Array<{ value: T; label: string }>
  modelValue: T
  ariaLabel?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: T): void
}>()

function select(value: T) {
  if (value !== props.modelValue) emit('update:modelValue', value)
}
</script>

<template>
  <div
    class="segmented"
    role="group"
    :aria-label="props.ariaLabel"
  >
    <button
      v-for="item in props.items"
      :key="item.value"
      type="button"
      class="segment"
      :class="{ active: item.value === props.modelValue }"
      :aria-pressed="item.value === props.modelValue"
      @click="select(item.value)"
    >
      {{ item.label }}
    </button>
  </div>
</template>

<style scoped>
.segmented {
  display: inline-flex;
  padding: 2px;
  gap: 2px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-muted-soft);
}

.segment {
  padding: 4px 12px;
  border: none;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 500;
  cursor: pointer;
  white-space: nowrap;
  transition: background 0.15s ease, color 0.15s ease;
}

.segment:hover {
  color: var(--color-text);
}

.segment.active {
  background: var(--color-primary);
  color: var(--color-on-primary);
}
</style>
