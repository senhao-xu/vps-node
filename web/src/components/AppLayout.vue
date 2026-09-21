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
  { to: '/nodes', label: '节点', exact: false },
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
          <span class="brand-mark">V</span>
          <span>VPS Node</span>
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
  background: var(--color-bg);
}

.sidebar {
  width: var(--shell-width);
  flex-shrink: 0;
  background: var(--color-shell);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  z-index: 2;
}

.brand {
  padding: 21px var(--spacing-lg);
  font-size: var(--font-size-lg);
  font-weight: 700;
  border-bottom: 1px solid var(--color-border);
}

.brand a {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  color: var(--color-text);
  letter-spacing: -0.02em;
}

.brand-mark {
  display: inline-block;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: var(--color-primary);
  color: #fff;
  font-size: 17px;
  line-height: 30px;
  text-align: center;
}

.nav {
  display: flex;
  flex-direction: column;
  padding: var(--spacing-md) var(--spacing-sm);
  gap: var(--spacing-xs);
}

.secondary-label {
  margin-top: var(--spacing-md);
}

.nav-item {
  position: relative;
  padding: 10px var(--spacing-md);
  border-radius: var(--radius-sm);
  color: var(--color-shell-muted);
  font-weight: 500;
  transition: color 0.15s ease, background 0.15s ease;
}

.nav-item:hover {
  color: var(--color-text);
  background: var(--color-muted-soft);
}

.nav-item.active {
  color: var(--color-primary);
  background: var(--color-shell-active);
  font-weight: 600;
}

.nav-item.active::before {
  content: '';
  position: absolute;
  left: 6px;
  top: 50%;
  transform: translateY(-50%);
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--color-primary);
}

.main-area {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.topbar {
  min-height: 64px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 var(--spacing-lg);
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  position: sticky;
  top: 0;
  z-index: 1;
}

.topbar-title {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
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
    overflow: hidden;
  }

  .brand {
    border-bottom: none;
    padding: var(--spacing-md);
    flex: none;
  }

  .nav {
    flex-direction: row;
    overflow-x: auto;
    padding: var(--spacing-sm);
    scrollbar-width: none;
  }

  .nav::-webkit-scrollbar {
    display: none;
  }

  .nav-item {
    flex: none;
    white-space: nowrap;
  }

  .topbar {
    min-height: 56px;
  }
}

@media (max-width: 600px) {
  .sidebar {
    align-items: stretch;
    flex-direction: column;
  }

  .brand {
    padding: 14px var(--spacing-md) var(--spacing-sm);
  }

  .nav {
    padding: 0 var(--spacing-sm) var(--spacing-sm);
  }

  .topbar {
    padding: 0 var(--spacing-md);
  }

  .admin {
    display: none;
  }
}
</style>
