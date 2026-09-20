<script setup lang="ts">
import { ref } from 'vue'
import { copyText } from '@/utils/clipboard'

const props = defineProps<{
  label: string
  value: string
  hint?: string
}>()

const copied = ref(false)

async function copy() {
  const ok = await copyText(props.value)
  copied.value = ok
}
</script>

<template>
  <div class="secret">
    <div class="secret-label">
      {{ props.label }}
    </div>
    <div class="secret-row">
      <code class="secret-value">{{ props.value }}</code>
      <button
        type="button"
        class="btn small"
        @click="copy"
      >
        {{ copied ? '已复制' : '复制' }}
      </button>
    </div>
    <p class="secret-hint">
      {{ props.hint ?? '仅显示这一次，请立即复制保存，关闭后无法再次查看。' }}
    </p>
  </div>
</template>

<style scoped>
.secret {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  padding: var(--spacing-md);
  border: 1px dashed var(--color-warning);
  border-radius: var(--radius-sm);
  background: rgba(217, 119, 6, 0.06);
}

.secret-label {
  font-size: var(--font-size-sm);
  color: var(--color-text-secondary);
}

.secret-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
}

.secret-value {
  flex: 1;
  font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
  font-size: var(--font-size-md);
  word-break: break-all;
  background: var(--color-surface);
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
}

.secret-hint {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--color-warning);
}
</style>
