<script setup lang="ts">
import { computed } from 'vue'
import type { TrafficPoint } from '@/api/types'
import { formatBytes, formatDateTimeShort } from '@/utils/format'

const props = defineProps<{
  series: TrafficPoint[]
}>()

const WIDTH = 640
const HEIGHT = 200
const PADDING = 4

const bars = computed(() => {
  const points = props.series
  if (points.length === 0) return []
  const totals = points.map((p) => p.upload_bytes + p.download_bytes)
  const max = Math.max(...totals, 1)
  const slot = WIDTH / points.length
  const barWidth = Math.max(2, Math.min(28, slot * 0.6))
  return points.map((point, index) => {
    const upHeight = (point.upload_bytes / max) * (HEIGHT - PADDING * 2)
    const downHeight = (point.download_bytes / max) * (HEIGHT - PADDING * 2)
    const x = index * slot + (slot - barWidth) / 2
    const downY = HEIGHT - PADDING - downHeight
    return {
      x,
      width: barWidth,
      downY,
      downHeight,
      upY: downY - upHeight,
      upHeight,
      point,
    }
  })
})

const hasData = computed(() => props.series.some((p) => p.upload_bytes > 0 || p.download_bytes > 0))
</script>

<template>
  <div class="chart">
    <div class="legend">
      <span class="item"><i class="dot upload" />上行</span>
      <span class="item"><i class="dot download" />下行</span>
    </div>
    <div
      v-if="bars.length === 0 || !hasData"
      class="empty-tip"
    >
      所选时间范围内暂无流量数据
    </div>
    <template v-else>
      <svg
        class="svg"
        :viewBox="`0 0 ${WIDTH} ${HEIGHT}`"
        preserveAspectRatio="none"
        role="img"
        aria-label="流量柱状图"
      >
        <g
          v-for="(bar, index) in bars"
          :key="index"
        >
          <title>
            {{ formatDateTimeShort(bar.point.bucket_start) }}
            上行 {{ formatBytes(bar.point.upload_bytes) }}
            下行 {{ formatBytes(bar.point.download_bytes) }}
          </title>
          <rect
            :x="bar.x"
            :y="bar.downY"
            :width="bar.width"
            :height="bar.downHeight"
            class="rect download"
          />
          <rect
            :x="bar.x"
            :y="bar.upY"
            :width="bar.width"
            :height="bar.upHeight"
            class="rect upload"
          />
        </g>
      </svg>
      <div class="axis">
        <span>{{ formatDateTimeShort(props.series[0]?.bucket_start) }}</span>
        <span>{{ formatDateTimeShort(props.series[props.series.length - 1]?.bucket_start) }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.chart {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
}

.legend {
  display: flex;
  gap: var(--spacing-md);
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.legend .item {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.dot {
  width: 10px;
  height: 10px;
  border-radius: 2px;
  display: inline-block;
}

.dot.upload {
  background: var(--color-primary);
}

.dot.download {
  background: var(--color-success);
}

.svg {
  width: 100%;
  height: 200px;
  display: block;
}

.rect.upload {
  fill: var(--color-primary);
}

.rect.download {
  fill: var(--color-success);
}

.axis {
  display: flex;
  justify-content: space-between;
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}
</style>
