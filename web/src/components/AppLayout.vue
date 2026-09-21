<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { computed, ref } from 'vue'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type ThemeMode } from '@/stores/theme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const appVersion = __APP_VERSION__

const collapsed = ref(localStorage.getItem('sidebar-collapsed') === '1')

function toggleCollapse() {
  collapsed.value = !collapsed.value
  localStorage.setItem('sidebar-collapsed', collapsed.value ? '1' : '0')
}

const collapseTitle = computed(() => (collapsed.value ? '展开侧边栏' : '收起侧边栏'))

const navItems = [
  {
    to: '/',
    label: '仪表盘',
    exact: true,
    icon: 'M3 3h7v9H3z M14 3h7v5h-7z M14 12h7v9h-7z M3 16h7v5H3z',
  },
  {
    to: '/users',
    label: '用户',
    exact: false,
    icon: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2 M9 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z M22 21v-2a4 4 0 0 0-3-3.87 M16 3.13a4 4 0 0 1 0 7.75',
  },
  {
    to: '/servers',
    label: '服务器',
    exact: false,
    icon: 'M2 2h20v8H2z M2 14h20v8H2z M6 6h.01 M6 18h.01',
  },
  {
    to: '/nodes',
    label: '节点',
    exact: false,
    icon: 'M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71 M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71',
  },
  {
    to: '/settings',
    label: '设置',
    exact: false,
    icon: 'M12 15a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z',
  },
]

const activePath = computed(() => route.path)

function isActive(item: { to: string; exact: boolean }): boolean {
  if (item.exact) return activePath.value === item.to
  return activePath.value === item.to || activePath.value.startsWith(`${item.to}/`)
}

const themeOrder: ThemeMode[] = ['light', 'dark', 'system']

const themeLabels: Record<ThemeMode, string> = {
  light: '浅色',
  dark: '深色',
  system: '跟随系统',
}

const themeTitle = computed(() => `主题：${themeLabels[theme.mode]}（点击切换）`)

function cycleTheme() {
  const next = themeOrder[(themeOrder.indexOf(theme.mode) + 1) % themeOrder.length]
  theme.setMode(next)
}

async function handleLogout() {
  await auth.logout()
  void router.push({ name: 'login' })
}
</script>

<template>
  <div class="layout">
    <aside
      class="sidebar"
      :class="{ collapsed }"
    >
      <div class="brand">
        <RouterLink to="/">
          <span class="brand-mark">V</span>
          <span class="brand-text">VPS Node</span>
        </RouterLink>
      </div>
      <nav class="nav">
        <RouterLink
          v-for="item in navItems"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ active: isActive(item) }"
          :title="collapsed ? item.label : undefined"
        >
          <svg
            class="nav-icon"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
          >
            <path :d="item.icon" />
          </svg>
          <span class="nav-label">{{ item.label }}</span>
        </RouterLink>
      </nav>
      <div class="sidebar-footer">
        <i class="version-dot" />
        <span class="version-text mono">{{ appVersion }}</span>
      </div>
      <button
        type="button"
        class="collapse-btn"
        :title="collapseTitle"
        :aria-label="collapseTitle"
        :aria-expanded="!collapsed"
        @click="toggleCollapse"
      >
        <svg
          class="collapse-icon"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="2"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
        >
          <path d="m15 18-6-6 6-6" />
        </svg>
      </button>
    </aside>
    <div class="main-area">
      <header class="topbar">
        <div class="topbar-title">
          {{ navItems.find((item) => isActive(item))?.label ?? '' }}
        </div>
        <div class="topbar-right">
          <button
            type="button"
            class="theme-toggle"
            :title="themeTitle"
            :aria-label="themeTitle"
            @click="cycleTheme"
          >
            <svg
              v-if="theme.mode === 'light'"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M12 17a5 5 0 1 0 0-10 5 5 0 0 0 0 10z M12 1v2 M12 21v2 M4.22 4.22l1.42 1.42 M18.36 18.36l1.42 1.42 M1 12h2 M21 12h2 M4.22 19.78l1.42-1.42 M18.36 5.64l1.42-1.42" />
            </svg>
            <svg
              v-else-if="theme.mode === 'dark'"
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
            </svg>
            <svg
              v-else
              viewBox="0 0 24 24"
              fill="none"
              stroke="currentColor"
              stroke-width="1.8"
              stroke-linecap="round"
              stroke-linejoin="round"
              aria-hidden="true"
            >
              <path d="M2 3h20v14H2z M8 21h8 M12 17v4" />
            </svg>
          </button>
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
  position: relative;
  width: var(--shell-width);
  flex-shrink: 0;
  background: var(--color-shell);
  border-right: 1px solid var(--color-border);
  display: flex;
  flex-direction: column;
  z-index: 2;
  transition: width 0.2s ease, background-color 0.2s ease, border-color 0.2s ease;
}

.brand {
  padding: 20px var(--spacing-lg);
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
  flex: none;
  width: 30px;
  height: 30px;
  border-radius: var(--radius-sm);
  background: var(--color-primary);
  color: var(--color-on-primary);
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

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-height: 48px;
  padding: 0 var(--spacing-md);
  border-radius: var(--radius-sm);
  color: var(--color-shell-muted);
  font-weight: 500;
  transition: color 0.15s ease, background 0.15s ease;
}

.nav-icon {
  width: 18px;
  height: 18px;
  flex: none;
}

.nav-item:hover {
  color: var(--color-text);
  background: var(--color-muted-soft);
}

.nav-item.active {
  color: var(--color-text);
  background: var(--color-shell-active);
  font-weight: 600;
}

.sidebar-footer {
  margin-top: auto;
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  padding: var(--spacing-md) var(--spacing-lg);
  border-top: 1px solid var(--color-border);
}

.version-dot {
  width: 7px;
  height: 7px;
  flex: none;
  border-radius: 50%;
  background: var(--color-success);
}

.version-text {
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.collapse-btn {
  position: absolute;
  right: -14px;
  top: 50%;
  transform: translateY(-50%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 28px;
  height: 28px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: 50%;
  background: var(--color-surface);
  color: var(--color-text-secondary);
  cursor: pointer;
  z-index: 3;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}

.collapse-btn:hover {
  color: var(--color-text);
  border-color: var(--color-border-strong);
  background: var(--color-muted-soft);
}

.collapse-icon {
  width: 15px;
  height: 15px;
  transition: transform 0.2s ease;
}

@media (min-width: 901px) {
  .sidebar.collapsed {
    width: var(--shell-width-collapsed);
  }

  .sidebar.collapsed .brand {
    padding: 20px 0;
    text-align: center;
  }

  .sidebar.collapsed .brand a {
    justify-content: center;
  }

  .sidebar.collapsed .brand-text,
  .sidebar.collapsed .nav-label,
  .sidebar.collapsed .version-text {
    display: none;
  }

  .sidebar.collapsed .nav-item {
    justify-content: center;
    padding: 0;
  }

  .sidebar.collapsed .sidebar-footer {
    justify-content: center;
    padding: var(--spacing-md) 0;
  }

  .sidebar.collapsed .collapse-icon {
    transform: rotate(180deg);
  }
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
  color: var(--color-text);
  font-size: var(--font-size-xl);
  font-weight: 700;
  letter-spacing: -0.02em;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
}

.theme-toggle {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  padding: 0;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  cursor: pointer;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}

.theme-toggle svg {
  width: 17px;
  height: 17px;
}

.theme-toggle:hover {
  color: var(--color-text);
  border-color: var(--color-border-strong);
  background: var(--color-muted-soft);
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
    min-height: 40px;
    white-space: nowrap;
  }

  .collapse-btn,
  .sidebar-footer {
    display: none;
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

  .topbar-title {
    font-size: var(--font-size-lg);
  }

  .admin {
    display: none;
  }
}
</style>
