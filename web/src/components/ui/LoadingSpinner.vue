<script setup lang="ts">
withDefaults(
  defineProps<{
    size?: 'sm' | 'md' | 'lg'
    label?: string
  }>(),
  {
    size: 'md',
    label: '加载中',
  },
)
</script>

<template>
  <span
    class="spinner"
    :class="`size-${size}`"
    role="status"
    :aria-label="label"
  />
</template>

<style scoped>
.spinner {
  display: inline-block;
  flex: none;
  border: 2px solid color-mix(in srgb, currentColor 25%, transparent);
  border-top-color: currentColor;
  border-radius: 50%;
  animation: spin 0.7s linear infinite;
}

.spinner.size-sm {
  width: 14px;
  height: 14px;
}

.spinner.size-md {
  width: 20px;
  height: 20px;
}

.spinner.size-lg {
  width: 28px;
  height: 28px;
  border-width: 3px;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
</style>
