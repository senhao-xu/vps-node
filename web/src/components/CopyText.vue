<script setup lang="ts">
import { useCopyFeedback } from '@/utils/clipboard'

const props = withDefaults(
  defineProps<{
    text: string
    display?: string
    buttonVariant?: 'link' | 'secondary'
  }>(),
  { display: '', buttonVariant: 'link' },
)

const { copied, copy } = useCopyFeedback()

async function onCopy() {
  await copy(props.text)
}
</script>

<template>
  <span class="copy-text">
    <span class="value">{{ props.display || props.text }}</span>
    <button
      type="button"
      :class="['btn', props.buttonVariant, 'small']"
      @click="onCopy"
    >
      {{ copied ? '已复制' : '复制' }}
    </button>
  </span>
</template>

<style scoped>
.copy-text {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  min-width: 0;
}

.value {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
</style>
