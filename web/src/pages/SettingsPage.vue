<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { getSettings, updateSettings } from '@/api/settings'
import { errorMessage } from '@/api/http'
import ErrorBanner from '@/components/ErrorBanner.vue'
import { DefaultClashMetaTemplate } from '@/utils/defaults'

const form = reactive({
  retention_raw_log_days: 7,
  retention_aggregate_days: 90,
  collection_connection_logs: true,
  session_freshness_seconds: 300,
  server_offline_after_seconds: 60,
  subscribe_urls: '',
  subscribe_path: 's',
  clash_meta_template: DefaultClashMetaTemplate,
})

const loading = ref(false)
const saving = ref(false)
const error = ref('')
const saved = ref(false)
const defaultClashTemplate = DefaultClashMetaTemplate

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
      subscribe_urls: form.subscribe_urls,
      subscribe_path: form.subscribe_path,
      clash_meta_template: form.clash_meta_template,
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
    <div class="page-header">
      <div>
        <p class="eyebrow">
          SYSTEM PREFERENCES
        </p>
        <h1 class="page-title">
          设置
        </h1>
      </div>
    </div>
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
        订阅设置
      </h2>
      <div class="form-grid">
        <div class="field">
          <label for="subscribe-urls">订阅基地址</label>
          <input
            id="subscribe-urls"
            v-model="form.subscribe_urls"
            type="text"
            placeholder="https://panel.example.com,https://backup.example.com"
          >
          <span class="text-secondary hint">多个地址使用英文逗号分隔；必须是无路径、无凭据的 HTTP(S) Origin。</span>
        </div>
        <div class="field">
          <label for="subscribe-path">订阅路径</label>
          <input
            id="subscribe-path"
            v-model="form.subscribe_path"
            type="text"
            maxlength="32"
          >
          <span class="text-secondary hint">安全单路径段，修改后立即生效，例如 s 或 subscribe。</span>
        </div>
      </div>
    </div>

    <div class="card">
      <h2 class="card-title">
        Clash Meta 覆写模板
      </h2>
      <div class="field">
        <label for="clash-template">受限 YAML</label>
        <textarea
          id="clash-template"
          v-model="form.clash_meta_template"
          rows="18"
          spellcheck="false"
        />
        <span class="text-secondary hint">允许 DNS、代理组、规则、规则集和基础运行参数。代理组使用 __ALL_PROXIES__、__SHADOWSOCKS_PROXIES__、__VLESS_PROXIES__、__HYSTERIA2_PROXIES__、__ANYTLS_PROXIES__ 注入动态节点；不得添加 proxies、proxy-providers、监听器或控制接口。</span>
      </div>
      <button
        type="button"
        class="btn secondary small"
        @click="form.clash_meta_template = defaultClashTemplate"
      >
        恢复默认模板
      </button>
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

.eyebrow {
  margin: 0 0 var(--spacing-xs);
  color: var(--color-primary);
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.1em;
}

.card-title {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
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

@media (max-width: 700px) {
  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .toggle {
    align-self: start;
    margin-top: 0;
  }

  .save-row .btn {
    flex: 1;
  }
}
</style>
