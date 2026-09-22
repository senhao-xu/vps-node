<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { Search } from 'lucide-vue-next'

const props = withDefaults(
  defineProps<{
    modelValue: string
    placeholder?: string
  }>(),
  {
    placeholder: '搜索…',
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'search'): void
}>()

let timer: ReturnType<typeof setTimeout> | null = null
let lastInput = ''

function cancelPending() {
  if (timer) clearTimeout(timer)
  timer = null
}

// Debounce only on user input. Programmatic `modelValue` changes (reset, URL
// restore) must not trigger a request — callers drive those explicitly.
function onInput(value: string) {
  lastInput = value
  emit('update:modelValue', value)
  cancelPending()
  timer = setTimeout(() => emit('search'), 300)
}

// An external value change (reset button, filter click, route restore) cancels
// any pending debounce so it cannot fire a redundant follow-up request.
watch(
  () => props.modelValue,
  (value) => {
    if (value !== lastInput) cancelPending()
  },
)

function onEnter() {
  cancelPending()
  emit('search')
}

onBeforeUnmount(() => {
  cancelPending()
})
</script>

<template>
  <div class="search-input">
    <Search
      :size="14"
      class="search-icon"
    />
    <input
      type="text"
      :value="props.modelValue"
      :placeholder="props.placeholder"
      @input="onInput(($event.target as HTMLInputElement).value)"
      @keyup.enter="onEnter"
    >
  </div>
</template>

<style scoped>
.search-input {
  position: relative;
  display: inline-flex;
  align-items: center;
  width: min(280px, 100%);
}

.search-icon {
  position: absolute;
  left: 10px;
  color: var(--color-text-secondary);
  pointer-events: none;
}

.search-input input {
  width: 100%;
  padding-left: 30px;
}

@media (max-width: 700px) {
  .search-input {
    width: 100%;
  }
}
</style>
