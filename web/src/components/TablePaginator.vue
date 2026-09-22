<script setup lang="ts">
import { computed } from 'vue'
import { ChevronLeft, ChevronRight, ChevronsLeft, ChevronsRight } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    page: number
    pageSize: number
    total: number
    pageSizeOptions?: number[]
  }>(),
  {
    pageSizeOptions: () => [10, 20, 50, 100],
  },
)

const emit = defineEmits<{
  (e: 'change', page: number, pageSize: number): void
}>()

const totalPages = computed(() => Math.max(1, Math.ceil(props.total / props.pageSize)))

const pages = computed<Array<number | 'ellipsis'>>(() => {
  const count = totalPages.value
  const current = props.page
  if (count <= 7) return Array.from({ length: count }, (_, i) => i + 1)
  const windowPages = new Set<number>([1, count, current - 1, current, current + 1])
  const sorted = Array.from(windowPages)
    .filter((p) => p >= 1 && p <= count)
    .sort((a, b) => a - b)
  const result: Array<number | 'ellipsis'> = []
  let prev = 0
  for (const p of sorted) {
    if (prev && p - prev > 1) result.push('ellipsis')
    result.push(p)
    prev = p
  }
  return result
})

function changePage(page: number) {
  const clamped = Math.min(Math.max(1, page), totalPages.value)
  if (clamped !== props.page) emit('change', clamped, props.pageSize)
}

function changePageSize(size: number) {
  if (size !== props.pageSize) emit('change', 1, size)
}
</script>

<template>
  <div
    v-if="props.total > 0 || props.page > 1"
    class="paginator"
  >
    <span class="text-secondary">共 {{ props.total }} 条</span>
    <div class="spacer" />
    <label class="size">
      每页显示
      <select
        :value="props.pageSize"
        @change="changePageSize(Number(($event.target as HTMLSelectElement).value))"
      >
        <option
          v-for="size in props.pageSizeOptions"
          :key="size"
          :value="size"
        >{{ size }}</option>
      </select>
    </label>
    <nav
      class="pager"
      aria-label="分页"
    >
      <button
        type="button"
        class="page-btn"
        aria-label="第一页"
        :disabled="props.page <= 1"
        @click="changePage(1)"
      >
        <ChevronsLeft :size="14" />
      </button>
      <button
        type="button"
        class="page-btn"
        aria-label="上一页"
        :disabled="props.page <= 1"
        @click="changePage(props.page - 1)"
      >
        <ChevronLeft :size="14" />
      </button>
      <template
        v-for="(item, index) in pages"
        :key="index"
      >
        <span
          v-if="item === 'ellipsis'"
          class="ellipsis"
        >…</span>
        <button
          v-else
          type="button"
          class="page-btn"
          :class="{ current: item === props.page }"
          :aria-current="item === props.page ? 'page' : undefined"
          @click="changePage(item)"
        >
          {{ item }}
        </button>
      </template>
      <button
        type="button"
        class="page-btn"
        aria-label="下一页"
        :disabled="props.page >= totalPages"
        @click="changePage(props.page + 1)"
      >
        <ChevronRight :size="14" />
      </button>
      <button
        type="button"
        class="page-btn"
        aria-label="最后一页"
        :disabled="props.page >= totalPages"
        @click="changePage(totalPages)"
      >
        <ChevronsRight :size="14" />
      </button>
    </nav>
  </div>
</template>

<style scoped>
.paginator {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  padding-top: var(--spacing-md);
  font-size: var(--font-size-sm);
  flex-wrap: wrap;
}

.spacer {
  flex: 1;
}

.size {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  color: var(--color-text-secondary);
}

.size select {
  min-height: 30px;
  padding: 3px 26px 3px 8px;
  font-size: var(--font-size-sm);
  background-position: right 6px center;
}

.pager {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
}

.page-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 30px;
  height: 30px;
  padding: 0 6px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.page-btn:hover:not(:disabled):not(.current) {
  background: var(--color-primary-soft);
}

.page-btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.page-btn.current {
  background: var(--color-primary);
  border-color: var(--color-primary);
  color: var(--color-on-primary);
}

.ellipsis {
  color: var(--color-text-secondary);
  padding: 0 2px;
}

@media (max-width: 560px) {
  .spacer {
    display: none;
  }

  .paginator > :first-child {
    width: 100%;
  }
}
</style>
