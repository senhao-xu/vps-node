<script setup lang="ts">
import { computed } from 'vue'
import type { LucideIcon } from 'lucide-vue-next'

export type MetricStripBar = {
  percent?: number
  tone?: 'primary' | 'success' | 'warning' | 'danger' | 'purple'
}

export type MetricStripItem = {
  key: string
  label: string
  value: string | number
  hint?: string
  icon?: LucideIcon
  tone?: 'default' | 'success' | 'warning' | 'danger'
  bar?: MetricStripBar
}

const props = withDefaults(
  defineProps<{
    items: MetricStripItem[]
    ariaLabel?: string
    loading?: boolean
  }>(),
  {
    ariaLabel: '关键指标',
    loading: false,
  },
)

const countClass = computed(() => 'count-' + Math.min(Math.max(props.items.length, 1), 5))

function barWidth(bar: MetricStripBar): string {
  const percent = bar.percent
  if (percent === undefined || !Number.isFinite(percent)) return '100%'
  return Math.min(100, Math.max(0, percent)) + '%'
}
</script>

<template>
  <section
    class="metric-strip"
    :class="countClass"
    role="group"
    :aria-label="ariaLabel"
    :aria-busy="loading"
  >
    <div
      v-for="item in items"
      :key="item.key"
      class="metric-item"
      :class="'tone-' + (item.tone ?? 'default')"
    >
      <div class="metric-topline">
        <span
          v-if="item.icon"
          class="metric-icon"
        >
          <component
            :is="item.icon"
            :size="16"
            aria-hidden="true"
          />
        </span>
        <span class="metric-label">{{ item.label }}</span>
      </div>
      <div class="metric-value-row">
        <span
          v-if="loading"
          class="metric-skeleton skeleton"
        />
        <strong v-else>{{ item.value }}</strong>
        <span
          v-if="!loading && item.hint"
          class="metric-hint"
        >{{ item.hint }}</span>
      </div>
      <div
        v-if="item.bar && !loading"
        class="metric-bar"
        aria-hidden="true"
      >
        <span
          class="metric-bar-fill"
          :class="'bar-' + (item.bar.tone ?? 'primary')"
          :style="{ width: barWidth(item.bar) }"
        />
      </div>
    </div>
  </section>
</template>

<style scoped>
.metric-strip {
  display: grid;
  gap: 12px;
  margin-bottom: 16px;
}

.metric-strip.count-1 { grid-template-columns: minmax(0, 1fr); max-width: 300px; }
.metric-strip.count-2 { grid-template-columns: repeat(2, minmax(0, 1fr)); max-width: 612px; }
.metric-strip.count-3 { grid-template-columns: repeat(3, minmax(0, 1fr)); max-width: 936px; }
.metric-strip.count-4 { grid-template-columns: repeat(4, minmax(0, 1fr)); max-width: 1236px; }
.metric-strip.count-5 { grid-template-columns: repeat(5, minmax(0, 1fr)); max-width: 1548px; }

.metric-item {
  display: flex;
  min-width: 0;
  min-height: 112px;
  flex-direction: column;
  padding: 14px 16px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}

.metric-topline {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.metric-icon {
  display: grid;
  flex: none;
  place-items: center;
  order: 2;
  color: var(--color-text-secondary);
}

.metric-label {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.metric-value-row {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: flex-start;
  gap: 2px;
  margin-top: 8px;
}

.metric-value-row strong {
  max-width: 100%;
  overflow: hidden;
  color: var(--color-text);
  font-size: 24px;
  font-variant-numeric: tabular-nums;
  font-weight: 760;
  letter-spacing: -0.02em;
  line-height: 1.25;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-hint {
  max-width: 100%;
  overflow: hidden;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tone-success .metric-icon { color: var(--color-success); }
.tone-warning .metric-icon { color: var(--color-warning); }
.tone-danger .metric-icon { color: var(--color-danger); }

.metric-skeleton {
  display: block;
  width: 64%;
  height: 24px;
}

.metric-bar {
  height: 4px;
  margin-top: auto;
  overflow: hidden;
  border-radius: var(--radius-full);
  background: var(--color-muted-soft);
}

.metric-bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-full);
  transition: width 0.2s ease;
}

.metric-bar-fill.bar-primary { background: var(--color-primary); }
.metric-bar-fill.bar-success { background: var(--color-success); }
.metric-bar-fill.bar-warning { background: var(--color-warning); }
.metric-bar-fill.bar-danger { background: var(--color-danger); }
.metric-bar-fill.bar-purple { background: var(--color-badge-purple); }

@media (max-width: 1280px) {
  .metric-strip.count-5 { grid-template-columns: repeat(3, minmax(0, 1fr)); max-width: 936px; }
  .metric-strip.count-4 { grid-template-columns: repeat(2, minmax(0, 1fr)); max-width: 612px; }
}

@media (max-width: 800px) {
  .metric-strip.count-3,
  .metric-strip.count-5 { grid-template-columns: repeat(2, minmax(0, 1fr)); max-width: 612px; }
}

@media (max-width: 560px) {
  .metric-strip.count-1 { max-width: none; }
  .metric-strip.count-2,
  .metric-strip.count-3,
  .metric-strip.count-4,
  .metric-strip.count-5 { grid-template-columns: repeat(2, minmax(0, 1fr)); max-width: none; }

  .metric-strip.count-3 .metric-item:last-child,
  .metric-strip.count-5 .metric-item:last-child { grid-column: 1 / -1; }
}

@media (max-width: 360px) {
  .metric-strip.count-2,
  .metric-strip.count-3,
  .metric-strip.count-4,
  .metric-strip.count-5 { grid-template-columns: minmax(0, 1fr); }

  .metric-strip.count-3 .metric-item:last-child,
  .metric-strip.count-5 .metric-item:last-child { grid-column: auto; }
}
</style>
