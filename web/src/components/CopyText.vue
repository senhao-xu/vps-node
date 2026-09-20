<script setup lang="ts">
import { ref } from 'vue'
import { copyText } from '@/utils/clipboard'

const props = withDefaults(
  defineProps<{
    text: string
    display?: string
  }>(),
  { display: '' },
)

const copied = ref(false)
let timer: ReturnType<typeof setTimeout> | null = null

async function copy() {
  const ok = await copyText(props.text)
  copied.value = ok
  if (timer) clearTimeout(timer)
  timer = setTimeout(() => {
    copied.value = false
  }, 1500)
}
</script>

<template>
  <span class="copy-text">
    <span class="value">{{ props.display || props.text }}</span>
    <button
      type="button"
      class="btn link small"
      @click="copy"
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
