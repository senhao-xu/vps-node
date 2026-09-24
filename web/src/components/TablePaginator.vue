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
    <span class="total text-secondary">共 {{ props.total }} 条</span>
    <div class="right">
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
      <span class="page-info">第 {{ props.page }} / {{ totalPages }} 页</span>
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
  </div>
</template>

<style scoped>
.paginator {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
  padding-top: 12px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  flex-wrap: wrap;
}

.right {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-md);
}

.size {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--color-text-secondary);
}

.size select {
  min-height: 30px;
  padding: 2px 25px 2px 8px;
  background-position: right 6px center;
  font-size: var(--font-size-sm);
}

.page-info {
  color: var(--color-text);
  font-variant-numeric: tabular-nums;
  font-weight: 500;
  white-space: nowrap;
}

.pager {
  display: inline-flex;
  align-items: center;
  gap: 3px;
}

.page-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 30px;
  height: 30px;
  padding: 0 5px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, color 0.15s ease;
}

.page-btn:hover:not(:disabled) {
  border-color: var(--color-primary-border);
  background: var(--color-primary-soft);
  color: var(--color-primary);
}

.page-btn:disabled {
  opacity: 0.52;
  cursor: not-allowed;
}

@media (max-width: 640px) {
  .paginator,
  .right {
    width: 100%;
  }

  .right {
    justify-content: space-between;
  }

  .size {
    display: none;
  }
}
</style>
