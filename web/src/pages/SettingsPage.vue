<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Database, Globe, Radio, FileCode2, type LucideIcon } from 'lucide-vue-next'
import { getSettings, updateSettings } from '@/api/settings'
import { errorMessage } from '@/api/http'
import ErrorBanner from '@/components/ErrorBanner.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import PageHeader from '@/components/ui/PageHeader.vue'
import ToggleSwitch from '@/components/ui/ToggleSwitch.vue'
import { DefaultClashMetaTemplate } from '@/utils/defaults'

type SectionKey = 'retention' | 'subscribe' | 'template' | 'freshness'

const sections: Array<{ key: SectionKey; label: string; icon: LucideIcon }> = [
  { key: 'retention', label: '数据保留', icon: Database },
  { key: 'subscribe', label: '订阅设置', icon: Globe },
  { key: 'template', label: 'Clash Meta 模板', icon: FileCode2 },
  { key: 'freshness', label: '离线判定', icon: Radio },
]

const activeSection = ref<SectionKey>('retention')

const form = reactive({
  retention_aggregate_days: 90,
  retention_visit_days: 7,
  retention_visit_aggregate_days: 90,
  collection_visits: true,
  server_offline_after_seconds: 60,
  subscribe_urls: '',
  subscribe_path: 's',
  subscribe_name: '',
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
    | 'retention_aggregate_days'
    | 'retention_visit_days'
    | 'retention_visit_aggregate_days'
    | 'server_offline_after_seconds'
  const numbers: [NumericKey, number, number, string][] = [
    ['retention_aggregate_days', 1, 3650, '聚合统计保留天数'],
    ['retention_visit_days', 1, 3650, '访问记录保留天数'],
    ['retention_visit_aggregate_days', 1, 3650, '访问站点聚合保留天数'],
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
      retention_aggregate_days: form.retention_aggregate_days,
      retention_visit_days: form.retention_visit_days,
      retention_visit_aggregate_days: form.retention_visit_aggregate_days,
      collection_visits: form.collection_visits,
      server_offline_after_seconds: form.server_offline_after_seconds,
      subscribe_urls: form.subscribe_urls,
      subscribe_path: form.subscribe_path,
      subscribe_name: form.subscribe_name,
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
    <PageHeader
      title="设置"
      subtitle="数据保留、订阅分发与离线判定参数"
    />
    <ErrorBanner
      :message="error"
      @dismiss="error = ''"
    />

    <div
      v-if="loading"
      class="loading-block"
    >
      <LoadingSpinner size="lg" />
    </div>
    <div
      v-else
      class="settings-layout"
    >
      <nav
        class="settings-nav"
        aria-label="设置分组"
      >
        <button
          v-for="section in sections"
          :key="section.key"
          type="button"
          class="settings-nav-item"
          :class="{ active: activeSection === section.key }"
          @click="activeSection = section.key"
        >
          <component
            :is="section.icon"
            :size="15"
          />
          <span>{{ section.label }}</span>
        </button>
      </nav>

      <div class="settings-body">
        <div
          v-if="activeSection === 'retention'"
          class="card"
        >
          <h2 class="card-title">
            数据保留
          </h2>
          <div class="form-grid">
            <div class="field">
              <label for="retention-aggregate">流量记录保留天数</label>
              <input
                id="retention-aggregate"
                v-model.number="form.retention_aggregate_days"
                type="number"
                min="1"
                max="3650"
              >
              <p class="form-help">
                范围 1-3650 天，超过后自动清理
              </p>
            </div>
            <div class="field">
              <label for="retention-visit">访问记录保留天数</label>
              <input
                id="retention-visit"
                v-model.number="form.retention_visit_days"
                type="number"
                min="1"
                max="3650"
              >
              <p class="form-help">
                原始访问记录（含来源 IP 与目标站点），范围 1-3650 天
              </p>
            </div>
            <div class="field">
              <label for="retention-visit-aggregate">访问站点聚合保留天数</label>
              <input
                id="retention-visit-aggregate"
                v-model.number="form.retention_visit_aggregate_days"
                type="number"
                min="1"
                max="3650"
              >
              <p class="form-help">
                每日站点聚合数据，范围 1-3650 天
              </p>
            </div>
          </div>
          <div class="toggle-row">
            <div class="toggle-row-text">
              <span class="toggle-row-label">采集访问站点</span>
              <span class="toggle-row-desc">
                关闭后 Agent 上报将被拒绝，不再写入新的访问记录（历史数据仍按保留策略清理）
              </span>
            </div>
            <ToggleSwitch
              v-model="form.collection_visits"
              label="采集访问站点"
            />
          </div>
        </div>

        <div
          v-if="activeSection === 'subscribe'"
          class="card"
        >
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
              <p class="form-help">
                多个地址使用英文逗号分隔；必须是无路径、无凭据的 HTTP(S) Origin。
              </p>
            </div>
            <div class="field">
              <label for="subscribe-path">订阅路径</label>
              <input
                id="subscribe-path"
                v-model="form.subscribe_path"
                type="text"
                maxlength="32"
              >
              <p class="form-help">
                安全单路径段，修改后立即生效，例如 s 或 subscribe。
              </p>
            </div>
            <div class="field">
              <label for="subscribe-name">订阅名称</label>
              <input
                id="subscribe-name"
                v-model="form.subscribe_name"
                type="text"
                maxlength="64"
                placeholder="例如：我的节点"
              >
              <p class="form-help">
                展示在客户端订阅列表中的名称，留空则由客户端自行决定。
              </p>
            </div>
          </div>
        </div>

        <div
          v-if="activeSection === 'template'"
          class="card"
        >
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
            <p class="form-help">
              允许 DNS、代理组、规则、规则集和基础运行参数。代理组使用 __ALL_PROXIES__、__SHADOWSOCKS_PROXIES__、__VLESS_PROXIES__、__HYSTERIA2_PROXIES__、__ANYTLS_PROXIES__ 注入动态节点；不得添加 proxies、proxy-providers、监听器或控制接口。
            </p>
          </div>
          <div>
            <button
              type="button"
              class="btn secondary small"
              @click="form.clash_meta_template = defaultClashTemplate"
            >
              恢复默认模板
            </button>
          </div>
        </div>

        <div
          v-if="activeSection === 'freshness'"
          class="card"
        >
          <h2 class="card-title">
            离线判定
          </h2>
          <div class="form-grid">
            <div class="field">
              <label for="offline-after">Server 离线判定时间（秒）</label>
              <input
                id="offline-after"
                v-model.number="form.server_offline_after_seconds"
                type="number"
                min="10"
                max="86400"
              >
              <p class="form-help">
                范围 10-86400；心跳超过该间隔未到达即判定为离线
              </p>
            </div>
          </div>
        </div>

        <div class="save-row">
          <button
            type="button"
            class="btn"
            :class="{ 'is-loading': saving }"
            :disabled="saving || loading"
            @click="save"
          >
            <LoadingSpinner
              v-if="saving"
              size="sm"
            />
            {{ saving ? '保存中…' : '保存设置' }}
          </button>
          <span
            v-if="saved"
            class="saved-tip"
          >已保存</span>
        </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.loading-block {
  display: flex;
  justify-content: center;
  padding: var(--spacing-xl) 0;
}

.settings-layout {
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  gap: var(--spacing-lg);
  align-items: start;
}

.settings-nav {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-xs);
  position: sticky;
  top: 88px;
}

.settings-nav-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: 8px 12px;
  border: none;
  border-radius: var(--radius-sm);
  background: none;
  color: var(--color-text-secondary);
  font-size: var(--font-size-md);
  font-weight: 500;
  cursor: pointer;
  text-align: left;
  transition: background 0.15s ease, color 0.15s ease;
}

.settings-nav-item:hover {
  color: var(--color-text);
  background: var(--color-muted-soft);
}

.settings-nav-item.active {
  color: var(--color-text);
  background: var(--color-shell-active);
  font-weight: 600;
}

.settings-body .card {
  margin-top: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: var(--spacing-md) var(--spacing-lg);
  align-items: start;
  margin-bottom: var(--spacing-md);
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
  .settings-layout {
    grid-template-columns: minmax(0, 1fr);
  }

  .settings-nav {
    position: static;
    flex-direction: row;
    overflow-x: auto;
    padding-bottom: var(--spacing-xs);
  }

  .settings-nav-item {
    flex: none;
    white-space: nowrap;
  }

  .form-grid {
    grid-template-columns: minmax(0, 1fr);
  }

  .save-row .btn {
    flex: 1;
  }
}
</style>
