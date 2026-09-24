<script setup lang="ts" generic="T extends Record<string, unknown>">
import { computed } from 'vue'
import { ArrowDown, ArrowUp, ArrowUpDown } from 'lucide-vue-next'
import EmptyState from '@/components/ui/EmptyState.vue'

export interface Column {
  key: string
  label: string
  align?: 'left' | 'right' | 'center'
  width?: string
  sortable?: boolean
  divider?: boolean
}

const props = withDefaults(
  defineProps<{
    columns: Column[]
    rows: T[]
    rowKey: (row: T) => string | number
    loading?: boolean
    selectable?: boolean
    selected?: Array<string | number>
    sortKey?: string
    sortDir?: 'asc' | 'desc'
    skeletonRows?: number
    totalCount?: number
    bordered?: boolean
    ariaLabel?: string
  }>(),
  {
    loading: false,
    selectable: false,
    selected: () => [],
    sortKey: undefined,
    sortDir: undefined,
    skeletonRows: 5,
    totalCount: undefined,
    bordered: true,
    ariaLabel: '数据表格',
  },
)

const emit = defineEmits<{
  (e: 'update:selected', keys: Array<string | number>): void
  (e: 'sort', key: string): void
  (e: 'row-click', row: T, index: number): void
}>()

defineSlots<{
  [K in `cell-${string}`]?: (props: { row: T; index: number }) => unknown
} & {
  empty?: () => unknown
  'row-extra'?: (props: { row: T; index: number; colspan: number }) => unknown
}>()

const columnSpan = computed(() => props.columns.length + (props.selectable ? 1 : 0))

const selectedSet = computed(() => new Set(props.selected))
const rowKeys = computed(() => props.rows.map((row) => props.rowKey(row)))
const allSelected = computed(
  () => props.rows.length > 0 && rowKeys.value.every((key) => selectedSet.value.has(key)),
)
const someSelected = computed(
  () => !allSelected.value && rowKeys.value.some((key) => selectedSet.value.has(key)),
)

const total = computed(() => props.totalCount ?? props.rows.length)
const showFooter = computed(() => props.selectable)

function toggleAll(checked: boolean) {
  const next = new Set(props.selected)
  for (const key of rowKeys.value) {
    if (checked) next.add(key)
    else next.delete(key)
  }
  emit('update:selected', Array.from(next))
}

function toggleRow(key: string | number, checked: boolean) {
  const next = new Set(props.selected)
  if (checked) next.add(key)
  else next.delete(key)
  emit('update:selected', Array.from(next))
}

function alignStyle(column: Column): Record<string, string> {
  return column.align ? { textAlign: column.align } : {}
}

function ariaSort(column: Column): 'ascending' | 'descending' | undefined {
  if (!column.sortable || props.sortKey !== column.key) return undefined
  return props.sortDir === 'desc' ? 'descending' : 'ascending'
}

function display(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}
</script>

<template>
  <div
    class="table-box"
    :class="{ plain: !props.bordered }"
    role="region"
    tabindex="0"
    :aria-label="props.ariaLabel"
  >
    <table class="data-table">
      <thead>
        <tr>
          <th
            v-if="props.selectable"
            class="select-cell"
          >
            <input
              type="checkbox"
              aria-label="全选"
              :checked="allSelected"
              :indeterminate="someSelected"
              @change="toggleAll(($event.target as HTMLInputElement).checked)"
            >
          </th>
          <th
            v-for="column in props.columns"
            :key="column.key"
            :class="{ 'col-divider': column.divider }"
            :style="{ ...alignStyle(column), width: column.width }"
            :aria-sort="ariaSort(column)"
          >
            <button
              v-if="column.sortable"
              type="button"
              class="th-sort"
              :class="{ active: props.sortKey === column.key }"
              @click="emit('sort', column.key)"
            >
              <span>{{ column.label }}</span>
              <ArrowUp
                v-if="props.sortKey === column.key && props.sortDir === 'asc'"
                :size="13"
              />
              <ArrowDown
                v-else-if="props.sortKey === column.key"
                :size="13"
              />
              <ArrowUpDown
                v-else
                :size="13"
                class="sort-idle"
              />
            </button>
            <template v-else>
              {{ column.label }}
            </template>
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-if="props.loading && props.rows.length === 0">
          <tr
            v-for="n in props.skeletonRows"
            :key="`skeleton-${n}`"
            class="skeleton-row"
          >
            <td
              v-if="props.selectable"
              class="select-cell"
            >
              <span class="skeleton skeleton-box" />
            </td>
            <td
              v-for="column in props.columns"
              :key="column.key"
              :style="alignStyle(column)"
            >
              <span class="skeleton skeleton-line" />
            </td>
          </tr>
        </template>
        <template v-else-if="props.rows.length === 0">
          <tr>
            <td
              :colspan="columnSpan"
              class="empty-cell"
            >
              <slot name="empty">
                <EmptyState title="暂无数据" />
              </slot>
            </td>
          </tr>
        </template>
        <template v-else>
          <template
            v-for="(row, index) in props.rows"
            :key="props.rowKey(row)"
          >
            <tr
              :class="{ selected: props.selectable && selectedSet.has(props.rowKey(row)) }"
              @click="emit('row-click', row, index)"
            >
              <td
                v-if="props.selectable"
                class="select-cell"
              >
                <input
                  type="checkbox"
                  aria-label="选择行"
                  :checked="selectedSet.has(props.rowKey(row))"
                  @click.stop
                  @change="toggleRow(props.rowKey(row), ($event.target as HTMLInputElement).checked)"
                >
              </td>
              <td
                v-for="column in props.columns"
                :key="column.key"
                :class="{ 'col-divider': column.divider }"
                :style="alignStyle(column)"
              >
                <slot
                  :name="`cell-${column.key}`"
                  :row="row"
                  :index="index"
                >
                  {{ display(row[column.key]) }}
                </slot>
              </td>
            </tr>
            <slot
              name="row-extra"
              :row="row"
              :index="index"
              :colspan="columnSpan"
            />
          </template>
        </template>
      </tbody>
    </table>
  </div>
  <div
    v-if="showFooter"
    class="table-footer"
  >
    已选择 {{ props.selected.length }} 项，共 {{ total }} 项
  </div>
</template>

<style scoped>
.table-box {
  overflow-x: auto;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-md);
  background: var(--color-surface);
  scrollbar-color: var(--color-border-strong) transparent;
}

.table-box.plain {
  border: none;
  border-radius: 0;
}

.table-box:focus-visible {
  box-shadow: inset 0 0 0 2px var(--color-focus-ring);
  outline: none;
}

.data-table {
  width: 100%;
  min-width: 720px;
  border-collapse: collapse;
  font-size: var(--font-size-md);
  font-variant-numeric: tabular-nums;
}

.data-table th,
.data-table td {
  border-bottom: 1px solid var(--color-border);
  text-align: left;
  vertical-align: middle;
}

.data-table th {
  height: 36px;
  padding: 0 12px;
  background: var(--color-surface-muted);
  color: var(--color-text-secondary);
  font-size: var(--font-size-xs);
  font-weight: 700;
  letter-spacing: 0;
  white-space: nowrap;
}

.data-table td {
  height: 44px;
  padding: 9px 12px;
}

.data-table .col-divider {
  border-left: 1px solid var(--color-border);
}

.data-table tbody tr {
  transition: background-color 0.12s ease;
}

.data-table tbody tr:hover {
  background: var(--color-surface-muted);
}

.data-table tbody tr.selected {
  background: var(--color-primary-soft);
}

.data-table tbody tr:last-child td {
  border-bottom: none;
}

.select-cell {
  width: 42px;
  padding-right: 0;
}

.select-cell input {
  width: 15px;
  height: 15px;
  accent-color: var(--color-primary);
  cursor: pointer;
  vertical-align: middle;
}

.th-sort {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-xs);
  padding: 0;
  border: none;
  background: none;
  color: inherit;
  font: inherit;
  letter-spacing: inherit;
  cursor: pointer;
}

.th-sort:hover,
.th-sort.active {
  color: var(--color-text);
}

.sort-idle {
  opacity: 0.45;
}

.empty-cell {
  text-align: center;
}

.empty-cell :deep(.empty-tip) {
  padding: var(--spacing-lg) 0;
}

.skeleton-row td {
  border-bottom: 1px solid var(--color-border);
}

.skeleton-line {
  display: block;
  width: 70%;
  max-width: 140px;
  height: 12px;
}

.skeleton-box {
  display: block;
  width: 15px;
  height: 15px;
}

.table-footer {
  padding: 9px 12px;
  border-top: 1px solid var(--color-border);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}
</style>
