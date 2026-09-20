<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { computed } from 'vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const navItems = [
  { to: '/', label: '仪表盘', exact: true },
  { to: '/users', label: '用户', exact: false },
  { to: '/servers', label: '服务器', exact: false },
  { to: '/settings', label: '设置', exact: false },
]

const activePath = computed(() => route.path)

function isActive(item: { to: string; exact: boolean }): boolean {
  if (item.exact) return activePath.value === item.to
  return activePath.value === item.to || activePath.value.startsWith(`${item.to}/`)
}

async function handleLogout() {
  await auth.logout()
  void router.push({ name: 'login' })
}
</script>

<template>
  <div class="layout">
    <aside class="sidebar">
      <div class="brand">
        <RouterLink to="/">
          VPS Node
        </RouterLink>
      </div>
      <nav class="nav">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ active: isActive(item) }"
        >
          {{ item.label }}
        </RouterLink>
      </nav>
    </aside>
    <div class="main-area">
      <header class="topbar">
        <div class="topbar-title">
          {{ navItems.find((item) => isActive(item))?.label ?? '' }}
        </div>
        <div class="topbar-right">
          <span class="admin text-secondary">{{ auth.username || '管理员' }}</span>
          <button
            type="button"
            class="btn secondary small"
            @click="handleLogout"
          >
            退出登录
          </button>
        </div>
      </header>
      <main class="content">
        <RouterView />
      </main>
    </div>
  </div>
</template>

<style scoped>
.layout {
  display: flex;
  min-height: 100vh;
}

.sidebar {
  width: 200px;
  flex-shrink: 0;
  background: var(--color-surface);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
}

.brand {
  padding: var(--spacing-md) var(--spacing-lg);
  font-size: var(--font-size-lg);
  font-weight: 700;
  border-bottom: 1px solid var(--color-border);
}

.brand a {
  color: var(--color-text);
}

.nav {
  display: flex;
  flex-direction: column;
  padding: var(--spacing-sm);
  gap: 2px;
}

.nav-item {
  padding: var(--spacing-sm) var(--spacing-md);
  border-radius: var(--radius-sm);
  color: var(--color-text-secondary);
}

.nav-item:hover {
  color: var(--color-text);
  background: var(--color-bg);
}

.nav-item.active {
  color: var(--color-primary);
  background: rgba(37, 99, 235, 0.08);
  font-weight: 600;
}

.main-area {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  height: 52px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-lg);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
}

.topbar-title {
  font-weight: 600;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.content {
  flex: 1;
  min-width: 0;
}

@media (max-width: 900px) {
  .layout {
    flex-direction: column;
  }

  .sidebar {
    width: 100%;
    flex-direction: row;
    align-items: center;
    border-right: none;
    border-bottom: 1px solid var(--color-border);
  }

  .brand {
    border-bottom: none;
    padding: var(--spacing-md);
  }

  .nav {
    flex-direction: row;
  }
}
</style>
