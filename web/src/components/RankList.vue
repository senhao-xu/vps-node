<script setup lang="ts">
import ProgressBar from '@/components/ProgressBar.vue'
import EmptyState from '@/components/ui/EmptyState.vue'

export interface RankListItem {
  key: string | number
  name: string
  sub?: string
  value: string
  percent: number
}

const props = withDefaults(
  defineProps<{
    items: RankListItem[]
    loading?: boolean
    emptyText?: string
    skeletonRows?: number
  }>(),
  {
    loading: false,
    emptyText: '暂无数据',
    skeletonRows: 5,
  },
)

function formatPercent(percent: number): string {
  if (!Number.isFinite(percent)) return '0%'
  const clamped = Math.min(100, Math.max(0, percent))
  return clamped.toFixed(clamped % 1 === 0 ? 0 : 1) + '%'
}
</script>

<template>
  <div
    v-if="props.loading && props.items.length === 0"
    class="rank-loading"
    role="status"
    aria-label="正在加载排行数据"
  >
    <span
      v-for="n in props.skeletonRows"
      :key="n"
      class="skeleton rank-skeleton"
    />
  </div>
  <EmptyState
    v-else-if="props.items.length === 0"
    :title="props.emptyText"
  />
  <ol
    v-else
    class="rank-list"
  >
    <li
      v-for="(item, index) in props.items"
      :key="item.key"
      class="rank-item"
    >
      <span
        class="rank-badge"
        :class="{ top: index < 3 }"
        aria-hidden="true"
      >{{ index + 1 }}</span>
      <div class="rank-body">
        <div class="rank-row">
          <div class="rank-name">
            <span class="rank-name-text">{{ item.name }}</span>
            <span
              v-if="item.sub"
              class="rank-sub"
            >{{ item.sub }}</span>
          </div>
          <div class="rank-metric">
            <span class="rank-value">{{ item.value }}</span>
            <span class="rank-percent">{{ formatPercent(item.percent) }}</span>
          </div>
        </div>
        <ProgressBar
          :percent="item.percent"
          compact
        />
      </div>
    </li>
  </ol>
</template>

<style scoped>
.rank-loading {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.rank-skeleton {
  display: block;
  width: 100%;
  height: 44px;
}

.rank-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin: 0;
  padding: 0;
  list-style: none;
}

.rank-item {
  display: flex;
  min-width: 0;
  align-items: flex-start;
  gap: 10px;
}

.rank-badge {
  display: grid;
  width: 22px;
  height: 22px;
  flex-shrink: 0;
  place-items: center;
  margin-top: 1px;
  border-radius: var(--radius-full);
  background: var(--color-surface-muted);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}

.rank-badge.top {
  background: var(--color-primary);
  color: var(--color-on-primary);
}

.rank-body {
  display: flex;
  min-width: 0;
  flex: 1;
  flex-direction: column;
  gap: 6px;
}

.rank-row {
  display: flex;
  min-width: 0;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--spacing-sm);
}

.rank-name {
  display: flex;
  min-width: 0;
  align-items: baseline;
  gap: var(--spacing-xs);
}

.rank-name-text {
  overflow: hidden;
  min-width: 0;
  font-size: var(--font-size-sm);
  font-weight: 650;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rank-sub {
  overflow: hidden;
  min-width: 0;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rank-metric {
  display: flex;
  flex-shrink: 0;
  align-items: baseline;
  gap: var(--spacing-sm);
  font-variant-numeric: tabular-nums;
}

.rank-value {
  font-size: var(--font-size-sm);
  font-weight: 650;
}

.rank-percent {
  min-width: 44px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  text-align: right;
}
</style>
