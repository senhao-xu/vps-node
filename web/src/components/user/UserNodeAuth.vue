<script setup lang="ts">
import { ref, watch } from 'vue'
import { putUserNodes } from '@/api/users'
import { errorMessage } from '@/api/http'
import type { NodeBrief, Server } from '@/api/types'
import NodeChecklist from '@/components/NodeChecklist.vue'

const props = defineProps<{
  userId: number
  nodes: NodeBrief[]
  servers: Server[]
  nodeIds: number[]
}>()

const emit = defineEmits<{
  (e: 'saved', nodeIds: number[]): void
}>()

const selected = ref<number[]>([])
const dirty = ref(false)
const saving = ref(false)
const error = ref('')
const savedTip = ref(false)

watch(
  () => [props.userId, props.nodeIds] as const,
  () => {
    selected.value = [...props.nodeIds]
    dirty.value = false
    savedTip.value = false
    error.value = ''
  },
  { immediate: true },
)

function onSelectionChange(value: number[]) {
  selected.value = value
  dirty.value =
    value.length !== props.nodeIds.length ||
    [...value].sort().join(',') !== [...props.nodeIds].sort().join(',')
  savedTip.value = false
}

async function save() {
  saving.value = true
  error.value = ''
  try {
    const result = await putUserNodes(props.userId, selected.value)
    emit('saved', result.node_ids)
    dirty.value = false
    savedTip.value = true
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="card-head">
      <h2 class="card-title">
        节点授权
      </h2>
      <span
        v-if="savedTip"
        class="saved-tip"
      >已保存</span>
      <button
        type="button"
        class="btn small"
        :disabled="!dirty || saving"
        :title="dirty ? '' : '勾选变化后可保存'"
        @click="save"
      >
        {{ saving ? '保存中…' : '保存授权' }}
      </button>
    </div>
    <p class="text-secondary tip">
      用户只能使用被授权的节点；保存后立即生效，Agent 将在下次同步时应用。
    </p>
    <p
      v-if="error"
      class="text-danger form-error"
    >
      {{ error }}
    </p>
    <NodeChecklist
      :nodes="props.nodes"
      :servers="props.servers"
      :model-value="selected"
      @update:model-value="onSelectionChange"
    />
  </div>
</template>

<style scoped>
.card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--spacing-md);
}

.card-head .card-title {
  margin-bottom: 0;
}

.saved-tip {
  color: var(--color-success);
  font-size: var(--font-size-sm);
}

.tip {
  margin: var(--spacing-xs) 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
}

.form-error {
  margin: 0 0 var(--spacing-sm);
  font-size: var(--font-size-sm);
}

@media (max-width: 560px) {
  .card-head {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
