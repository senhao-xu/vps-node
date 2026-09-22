<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Monitor, Moon, Sun } from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type ThemeMode } from '@/stores/theme'
import { errorMessage } from '@/api/http'
import ErrorBanner from '@/components/ErrorBanner.vue'
import LoadingSpinner from '@/components/ui/LoadingSpinner.vue'
import type { LucideIcon } from 'lucide-vue-next'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const username = ref('')
const password = ref('')
const submitting = ref(false)
const error = ref('')

const themeOptions: Array<{ mode: ThemeMode; label: string; icon: LucideIcon }> = [
  { mode: 'light', label: '浅色', icon: Sun },
  { mode: 'dark', label: '深色', icon: Moon },
  { mode: 'system', label: '跟随系统', icon: Monitor },
]

async function submit() {
  if (!username.value.trim() || !password.value) {
    error.value = '请输入用户名和密码'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    await auth.login(username.value.trim(), password.value)
    const redirect = route.query.redirect
    const target = typeof redirect === 'string' && redirect.startsWith('/') ? redirect : '/'
    await router.replace(target)
  } catch (err) {
    error.value = errorMessage(err)
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <section class="login-page">
    <div
      class="theme-switch"
      role="group"
      aria-label="主题"
    >
      <button
        v-for="option in themeOptions"
        :key="option.mode"
        type="button"
        class="theme-option"
        :class="{ active: theme.mode === option.mode }"
        :title="option.label"
        :aria-label="option.label"
        :aria-pressed="theme.mode === option.mode"
        @click="theme.setMode(option.mode)"
      >
        <component
          :is="option.icon"
          :size="15"
        />
      </button>
    </div>
    <div class="login-wrap">
      <div class="login-heading">
        <span class="brand-mark">V</span>
        <h1 class="login-title">
          VPS Node
        </h1>
        <p class="login-sub text-secondary">
          管理面板
        </p>
      </div>
      <form
        class="card login-form"
        @submit.prevent="submit"
      >
        <h2 class="login-card-title">
          登录
        </h2>
        <p class="login-card-desc text-secondary">
          请使用管理员账号登录
        </p>
        <ErrorBanner
          :message="error"
          @dismiss="error = ''"
        />
        <div class="field">
          <label for="login-username">用户名</label>
          <input
            id="login-username"
            v-model="username"
            type="text"
            autocomplete="username"
            placeholder="用户名"
          >
        </div>
        <div class="field">
          <label for="login-password">密码</label>
          <input
            id="login-password"
            v-model="password"
            type="password"
            autocomplete="current-password"
            placeholder="密码"
          >
        </div>
        <button
          type="submit"
          class="btn"
          :class="{ 'is-loading': submitting }"
          :disabled="submitting"
        >
          <LoadingSpinner
            v-if="submitting"
            size="sm"
          />
          {{ submitting ? '登录中…' : '登录' }}
        </button>
      </form>
    </div>
  </section>
</template>

<style scoped>
.login-page {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-lg);
  background: var(--color-surface-muted);
}

.theme-switch {
  position: absolute;
  top: var(--spacing-md);
  right: var(--spacing-md);
}

.login-wrap {
  width: min(420px, 100%);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-lg);
}

.login-heading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--spacing-xs);
}

.brand-mark {
  display: inline-block;
  width: 36px;
  height: 36px;
  border-radius: var(--radius-sm);
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-size: 19px;
  font-weight: 700;
  line-height: 36px;
  text-align: center;
}

.login-title {
  margin: 0;
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.03em;
}

.login-sub {
  margin: 0;
  font-size: var(--font-size-sm);
}

.login-form {
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: 24px;
}

.login-card-title {
  margin: 0;
  font-size: var(--font-size-lg);
  font-weight: 600;
  letter-spacing: -0.02em;
}

.login-card-desc {
  margin: calc(-1 * var(--spacing-sm)) 0 0;
  font-size: var(--font-size-sm);
}
</style>
