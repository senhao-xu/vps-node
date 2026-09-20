<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { getSettings, updateSettings } from '@/api/settings'
import { errorMessage } from '@/api/http'
import ErrorBanner from '@/components/ErrorBanner.vue'

const form = reactive({
  retention_raw_log_days: 7,
  retention_aggregate_days: 90,
  collection_connection_logs: true,
  session_freshness_seconds: 300,
  server_offline_after_seconds: 60,
})

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const saved = ref(false)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const data = await getSettings()
    Object.assign(form, data)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    loading.value = false
  }
}

async function save() {
  error.value = ''
  saved.value = false
  type NumericKey =
    | 'retention_raw_log_days'
    | 'retention_aggregate_days'
    | 'session_freshness_seconds'
    | 'server_offline_after_seconds'
  const numbers: [NumericKey, number, number, string][] = [
    ['retention_raw_log_days', 1, 3650, '原始日志保留天数'],
    ['retention_aggregate_days', 1, 3650, '聚合统计保留天数'],
    ['session_freshness_seconds', 10, 86400, '会话新鲜度（秒）'],
    ['server_offline_after_seconds', 10, 86400, '离线判定时间（秒）'],
  ]
  for (const [key, min, max, label] of numbers) {
    const value = form[key]
    if (!Number.isInteger(value) || value < min || value > max) {
      error.value = `${label}必须是 ${min}-${max} 之间的整数`
      return
    }
  }
  saving.value = true
  try {
    const data = await updateSettings({
      retention_raw_log_days: form.retention_raw_log_days,
      retention_aggregate_days: form.retention_aggregate_days,
      collection_connection_logs: form.collection_connection_logs,
      session_freshness_seconds: form.session_freshness_seconds,
      server_offline_after_seconds: form.server_offline_after_seconds,
    })
    Object.assign(form, data)
    saved.value = true
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  void load()
})
</script>

<template>
  <section class="page">
    <h1 class="page-title">
      设置
    </h1>
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div class="card">
      <h2 class="card-title">
        数据保留
      </h2>
      <div class="form-grid">
        <div class="field">
          <label for="retention-raw">原始连接日志保留天数（1-3650）</label>
          <input
            id="retention-raw"
            v-model.number="form.retention_raw_log_days"
            type="number"
            min="1"
            max="3650"
          >
        </div>
        <div class="field">
          <label for="retention-aggregate">聚合统计保留天数（1-3650）</label>
          <input
            id="retention-aggregate"
            v-model.number="form.retention_aggregate_days"
            type="number"
            min="1"
            max="3650"
          >
        </div>
        <label class="checkbox-label toggle">
          <input
            v-model="form.collection_connection_logs"
            type="checkbox"
          >
          采集连接日志（关闭后不再记录新的连接日志）
        </label>
      </div>
    </div>

    <div class="card">
      <h2 class="card-title">
        在线判定
      </h2>
      <div class="form-grid">
        <div class="field">
          <label for="freshness">会话新鲜度（秒，10-86400）</label>
          <input
            id="freshness"
            v-model.number="form.session_freshness_seconds"
            type="number"
            min="10"
            max="86400"
          >
          <span class="text-secondary hint">超过该时间没有活动的连接不再计为在线</span>
        </div>
        <div class="field">
          <label for="offline-after">Server 离线判定时间（秒，10-86400）</label>
          <input
            id="offline-after"
            v-model.number="form.server_offline_after_seconds"
            type="number"
            min="10"
            max="86400"
          >
          <span class="text-secondary hint">心跳超过该间隔未到达即判定为离线</span>
        </div>
      </div>
    </div>

    <div class="save-row">
      <button
        type="button"
        class="btn"
        :disabled="saving || loading"
        @click="save"
      >
        {{ saving ? '保存中…' : '保存设置' }}
      </button>
      <span
        v-if="saved"
        class="saved-tip"
      >已保存</span>
    </div>
  </section>
</template>

<style scoped>
.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--spacing-md) var(--spacing-lg);
  align-items: start;
}

.toggle {
  margin-top: var(--spacing-lg);
  align-self: center;
}

.hint {
  font-size: var(--font-size-sm);
}

.save-row {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  margin-top: var(--spacing-md);
}

.saved-tip {
  color: var(--color-success);
  font-size: var(--font-size-sm);
}
</style>
