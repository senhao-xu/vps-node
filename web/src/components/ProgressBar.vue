<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    percent: number
    compact?: boolean
  }>(),
  { compact: false },
)

const clamped = computed(() => {
  if (!Number.isFinite(props.percent) || props.percent < 0) return 0
  return Math.min(100, props.percent)
})

const tone = computed(() => {
  if (clamped.value >= 90) return 'danger'
  if (clamped.value >= 70) return 'warning'
  return 'primary'
})
</script>

<template>
  <div
    class="progress"
    :class="{ compact: props.compact }"
  >
    <div class="track">
      <div
        class="fill"
        :class="tone"
        :style="{ width: `${clamped}%` }"
      />
    </div>
    <span
      v-if="!props.compact"
      class="percent"
    >{{ clamped.toFixed(clamped % 1 === 0 ? 0 : 1) }}%</span>
  </div>
</template>

<style scoped>
.progress {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.track {
  flex: 1;
  min-width: 60px;
  height: 8px;
  border-radius: 999px;
  background: rgba(144, 147, 153, 0.2);
  overflow: hidden;
}

.compact .track {
  height: 6px;
  min-width: 40px;
}

.fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.2s ease;
}

.fill.primary {
  background: var(--color-primary);
}

.fill.warning {
  background: var(--color-warning);
}

.fill.danger {
  background: var(--color-danger);
}

.percent {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
  min-width: 42px;
  text-align: right;
}
</style>
