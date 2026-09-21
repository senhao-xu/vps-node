<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { errorMessage } from '@/api/http'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const username = ref('')
const password = ref('')
const submitting = ref(false)
const error = ref('')

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
    <form
      class="card login-form"
      @submit.prevent="submit"
    >
      <div class="login-brand">
        <span>V</span> VPS NODE
      </div>
      <h1 class="login-title">
        VPS Node 管理面板
      </h1>
      <p class="text-secondary login-sub">
        请使用管理员账号登录
      </p>
      <div
        v-if="error"
        class="login-error"
      >
        {{ error }}
      </div>
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
        :disabled="submitting"
      >
        {{ submitting ? '登录中…' : '登录' }}
      </button>
    </form>
  </section>
</template>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--spacing-lg);
  background:
    radial-gradient(1200px 560px at 50% -10%, var(--color-primary-soft), transparent 62%),
    var(--color-bg);
}

.login-form {
  width: min(380px, 100%);
  display: flex;
  flex-direction: column;
  gap: var(--spacing-md);
  padding: var(--spacing-xl);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-dialog);
}

.login-brand {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: var(--spacing-sm);
  color: var(--color-primary);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.12em;
}

.login-brand span {
  display: inline-block;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  background: var(--color-primary);
  color: var(--color-on-primary);
  font-size: 17px;
  letter-spacing: 0;
  line-height: 30px;
  text-align: center;
}

.login-title {
  margin: 0;
  font-size: var(--font-size-xl);
  text-align: center;
  letter-spacing: -0.03em;
}

.login-sub {
  margin: 0;
  text-align: center;
  font-size: var(--font-size-sm);
}

.login-error {
  background: var(--color-danger-soft);
  border: 1px solid var(--color-danger-border);
  color: var(--color-danger);
  border-radius: var(--radius-sm);
  padding: var(--spacing-xs) var(--spacing-sm);
  font-size: var(--font-size-sm);
}
</style>
