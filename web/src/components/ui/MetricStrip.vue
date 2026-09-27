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
  margin-bottom: 14px;
  overflow: hidden;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-lg);
  background: var(--color-surface);
  box-shadow: var(--shadow-card);
}

.metric-strip.count-1 { grid-template-columns: 1fr; }
.metric-strip.count-2 { grid-template-columns: repeat(2, minmax(0, 1fr)); }
.metric-strip.count-3 { grid-template-columns: repeat(3, minmax(0, 1fr)); }
.metric-strip.count-4 { grid-template-columns: repeat(4, minmax(0, 1fr)); }
.metric-strip.count-5 { grid-template-columns: repeat(5, minmax(0, 1fr)); }

.metric-item {
  min-width: 0;
  padding: 15px 16px 14px;
  border-left: 1px solid var(--color-border);
}

.metric-item:first-child {
  border-left: 0;
}

.metric-topline {
  display: flex;
  align-items: center;
  gap: 8px;
}

.metric-icon {
  display: grid;
  width: 28px;
  height: 28px;
  flex: none;
  place-items: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface-muted);
  color: var(--color-primary);
}

.metric-label {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  font-weight: 600;
}

.metric-value-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
  min-width: 0;
  min-height: 28px;
  margin-top: 8px;
}

.metric-value-row strong {
  overflow: hidden;
  color: var(--color-text);
  font-size: 24px;
  font-variant-numeric: tabular-nums;
  font-weight: 750;
  line-height: 1.3;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.metric-hint {
  overflow: hidden;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tone-success .metric-icon { color: var(--color-success); background: var(--color-success-soft); border-color: var(--color-success-border); }
.tone-warning .metric-icon { color: var(--color-warning); background: var(--color-warning-soft); border-color: var(--color-warning-border); }
.tone-danger .metric-icon { color: var(--color-danger); background: var(--color-danger-soft); border-color: var(--color-danger-border); }

.metric-skeleton {
  display: block;
  width: 64%;
  height: 20px;
}

.metric-bar {
  height: 4px;
  margin-top: 12px;
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

@media (max-width: 900px) {
  .metric-strip.count-4,
  .metric-strip.count-5 {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .metric-strip.count-4 .metric-item:nth-child(4),
  .metric-strip.count-5 .metric-item:nth-child(4) {
    border-left: 0;
  }

  .metric-strip.count-4 .metric-item:nth-child(n + 4),
  .metric-strip.count-5 .metric-item:nth-child(n + 4) {
    border-top: 1px solid var(--color-border);
  }
}

@media (max-width: 600px) {
  .metric-strip.count-3,
  .metric-strip.count-4,
  .metric-strip.count-5 {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .metric-strip .metric-item:nth-child(odd) {
    border-left: 0;
  }

  .metric-strip .metric-item:nth-child(even) {
    border-left: 1px solid var(--color-border);
  }

  .metric-strip .metric-item:nth-child(n + 3) {
    border-top: 1px solid var(--color-border);
  }
}

@media (max-width: 375px) {
  .metric-strip.count-2,
  .metric-strip.count-3,
  .metric-strip.count-4,
  .metric-strip.count-5 {
    grid-template-columns: 1fr;
  }

  .metric-strip .metric-item {
    border-left: 0;
    border-top: 1px solid var(--color-border);
  }

  .metric-strip .metric-item:first-child {
    border-top: 0;
  }
}
</style>
