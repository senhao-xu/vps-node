<script setup lang="ts">
const props = withDefaults(
  defineProps<{
    modelValue: boolean
    disabled?: boolean
    label?: string
  }>(),
  {
    disabled: false,
    label: undefined,
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
}>()

function toggle() {
  if (props.disabled) return
  emit('update:modelValue', !props.modelValue)
}
</script>

<template>
  <button
    type="button"
    role="switch"
    class="toggle"
    :class="{ on: props.modelValue }"
    :aria-checked="props.modelValue"
    :aria-label="props.label"
    :disabled="props.disabled"
    @click="toggle"
  >
    <span class="thumb" />
  </button>
</template>

<style scoped>
.toggle {
  position: relative;
  flex: none;
  width: 36px;
  height: 20px;
  padding: 2px;
  border: none;
  border-radius: var(--radius-full);
  background: var(--color-border-strong);
  cursor: pointer;
  transition: background 0.15s ease;
}

.toggle.on {
  background: var(--color-success);
}

.toggle:disabled {
  opacity: 0.52;
  cursor: not-allowed;
}

.thumb {
  display: block;
  width: 16px;
  height: 16px;
  border-radius: 50%;
  background: var(--color-toggle-knob);
  box-shadow: var(--shadow-card);
  transition: transform 0.15s ease;
}

.toggle.on .thumb {
  transform: translateX(16px);
}
</style>
