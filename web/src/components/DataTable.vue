<script setup lang="ts" generic="T extends Record<string, unknown>">
export interface Column {
  key: string
  label: string
  align?: 'left' | 'right' | 'center'
  width?: string
}

const props = defineProps<{
  columns: Column[]
  rows: T[]
  loading?: boolean
}>()

defineSlots<{
  [K in `cell-${string}`]?: (props: { row: T; index: number }) => unknown
} & {
  empty?: () => unknown
}>()

function alignStyle(column: Column): Record<string, string> {
  return column.align ? { textAlign: column.align } : {}
}

function display(value: unknown): string {
  if (value === null || value === undefined || value === '') return '—'
  return String(value)
}
</script>

<template>
  <div class="table-wrap">
    <table class="data-table">
      <thead>
        <tr>
          <th
            v-for="column in props.columns"
            :key="column.key"
            :style="{ ...alignStyle(column), width: column.width }"
          >
            {{ column.label }}
          </th>
        </tr>
      </thead>
      <tbody>
        <template v-if="props.loading && props.rows.length === 0">
          <tr>
            <td
              :colspan="props.columns.length"
              class="empty-tip"
            >
              加载中…
            </td>
          </tr>
        </template>
        <template v-else-if="props.rows.length === 0">
          <tr>
            <td
              :colspan="props.columns.length"
              class="empty-tip"
            >
              <slot name="empty">
                暂无数据
              </slot>
            </td>
          </tr>
        </template>
        <template v-else>
          <tr
            v-for="(row, index) in props.rows"
            :key="index"
          >
            <td
              v-for="column in props.columns"
              :key="column.key"
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
        </template>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.table-wrap {
  overflow-x: auto;
  margin-inline: calc(-1 * var(--card-padding, var(--spacing-lg)));
  scrollbar-color: var(--color-border-strong) transparent;
}

.data-table {
  width: 100%;
  border-collapse: collapse;
  font-size: var(--font-size-md);
  min-width: 760px;
}

.data-table th,
.data-table td {
  padding: 14px var(--spacing-md);
  text-align: left;
  border-bottom: 1px solid var(--color-border);
  vertical-align: middle;
}

.data-table th:first-child,
.data-table td:first-child {
  padding-left: var(--card-padding, var(--spacing-lg));
}

.data-table th:last-child,
.data-table td:last-child {
  padding-right: var(--card-padding, var(--spacing-lg));
}

.data-table th {
  font-weight: 600;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  white-space: nowrap;
  background: var(--color-surface-muted);
  position: sticky;
  top: 0;
  z-index: 1;
  letter-spacing: 0.02em;
}

.data-table tbody tr:hover {
  background: var(--color-table-hover);
}

.data-table tbody tr:last-child td {
  border-bottom: none;
}
</style>
