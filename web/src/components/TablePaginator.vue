<script setup lang="ts">
import { computed } from 'vue'

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
    <span class="text-secondary">共 {{ props.total }} 条</span>
    <div class="spacer" />
    <label class="size">
      每页
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
      条
    </label>
    <span class="text-secondary">{{ props.page }} / {{ totalPages }}</span>
    <button
      type="button"
      class="btn secondary small"
      :disabled="props.page <= 1"
      @click="changePage(props.page - 1)"
    >
      上一页
    </button>
    <button
      type="button"
      class="btn secondary small"
      :disabled="props.page >= totalPages"
      @click="changePage(props.page + 1)"
    >
      下一页
    </button>
  </div>
</template>

<style scoped>
.paginator {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
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

@media (max-width: 560px) {
  .spacer {
    display: none;
  }

  .paginator > :first-child {
    width: 100%;
  }
}
</style>
