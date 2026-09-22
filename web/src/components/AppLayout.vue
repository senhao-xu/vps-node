<script setup lang="ts">
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import {
  ChevronsLeft,
  Globe,
  LayoutDashboard,
  Monitor,
  Moon,
  Search,
  Server,
  Settings,
  Sun,
  Users,
  Waypoints,
  type LucideIcon,
} from 'lucide-vue-next'
import { useAuthStore } from '@/stores/auth'
import { useThemeStore, type ThemeMode } from '@/stores/theme'
import MenuSearch from '@/components/MenuSearch.vue'

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

interface NavItem {
  to: string
  label: string
  icon: LucideIcon
  exact?: boolean
}

const navItems: NavItem[] = [
  { to: '/', label: '仪表盘', icon: LayoutDashboard, exact: true },
  { to: '/users', label: '用户', icon: Users },
  { to: '/servers', label: '服务器', icon: Server },
  { to: '/nodes', label: '节点', icon: Waypoints },
  { to: '/visits', label: '访问记录', icon: Globe },
  { to: '/settings', label: '设置', icon: Settings },
]

const activePath = computed(() => route.path)

function isActive(item: NavItem): boolean {
  if (item.exact) return activePath.value === item.to
  return activePath.value === item.to || activePath.value.startsWith(`${item.to}/`)
}

const searchOpen = ref(false)

function onGlobalKeydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    searchOpen.value = !searchOpen.value
  }
}

onMounted(() => window.addEventListener('keydown', onGlobalKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', onGlobalKeydown))

const themeOptions: Array<{ mode: ThemeMode; label: string; icon: LucideIcon }> = [
  { mode: 'light', label: '浅色', icon: Sun },
  { mode: 'dark', label: '深色', icon: Moon },
  { mode: 'system', label: '跟随系统', icon: Monitor },
]

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
          <component
            :is="item.icon"
            :size="18"
            class="nav-icon"
          />
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
        <ChevronsLeft
          :size="15"
          class="collapse-icon"
        />
      </button>
    </aside>
    <div class="main-area">
      <header class="topbar">
        <div class="topbar-row">
          <RouterLink
            class="mobile-brand"
            to="/"
            aria-label="返回仪表盘"
          >
            <span class="brand-mark">V</span>
          </RouterLink>
          <div class="topbar-right">
            <button
              type="button"
              class="search-trigger"
              @click="searchOpen = true"
            >
              <Search :size="14" />
              <span class="search-text">搜索菜单</span>
              <span class="kbd">⌘K</span>
            </button>
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
            <span class="admin text-secondary">{{ auth.username || '管理员' }}</span>
            <button
              type="button"
              class="btn ghost small"
              @click="handleLogout"
            >
              退出登录
            </button>
          </div>
        </div>
        <nav
          class="mobile-nav"
          aria-label="主导航"
        >
          <RouterLink
            v-for="item in navItems"
            :key="item.to"
            :to="item.to"
            class="mobile-nav-item"
            :class="{ active: isActive(item) }"
          >
            <component
              :is="item.icon"
              :size="15"
            />
            <span>{{ item.label }}</span>
          </RouterLink>
        </nav>
      </header>
      <main class="content">
        <RouterView />
      </main>
    </div>
    <MenuSearch
      :open="searchOpen"
      @close="searchOpen = false"
    />
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
  padding: 14px var(--spacing-lg);
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
  padding: var(--spacing-sm) 0;
  gap: 2px;
}

.nav-item {
  display: flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-height: 44px;
  padding: 0 var(--spacing-lg);
  border: none;
  border-radius: 0;
  background: none;
  color: var(--color-shell-muted);
  font-size: var(--font-size-md);
  font-weight: 500;
  text-align: left;
  width: 100%;
  cursor: pointer;
  transition: color 0.15s ease, background 0.15s ease;
}

.nav-icon {
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
  z-index: 30;
  transition: color 0.15s ease, border-color 0.15s ease, background 0.15s ease;
}

.collapse-btn:hover {
  color: var(--color-text);
  border-color: var(--color-border-strong);
  background: var(--color-muted-soft);
}

.collapse-icon {
  transition: transform 0.2s ease;
}

@media (min-width: 901px) {
  .sidebar.collapsed {
    width: var(--shell-width-collapsed);
  }

  .sidebar.collapsed .brand {
    padding: 14px 0;
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
  background: var(--color-surface);
  border-bottom: 1px solid var(--color-border);
  position: sticky;
  top: 0;
  z-index: 1;
}

.topbar-row {
  min-height: 64px;
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--spacing-md);
  padding: 0 var(--spacing-xl);
}

.mobile-brand {
  display: none;
}

.topbar-right {
  display: flex;
  align-items: center;
  gap: var(--spacing-md);
  flex: none;
}

.search-trigger {
  display: inline-flex;
  align-items: center;
  gap: var(--spacing-sm);
  min-height: var(--control-height);
  padding: 0 12px;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease;
}

.search-trigger:hover {
  border-color: var(--color-border-strong);
  color: var(--color-text);
}

.mobile-nav {
  display: none;
}

.content {
  flex: 1;
  min-width: 0;
}

@media (max-width: 900px) {
  .sidebar {
    display: none;
  }

  .topbar-row {
    min-height: 56px;
    padding: 0 var(--spacing-md);
    justify-content: space-between;
  }

  .mobile-brand {
    display: inline-flex;
    flex: none;
  }

  .mobile-nav {
    display: flex;
    gap: var(--spacing-xs);
    overflow-x: auto;
    padding: 0 var(--spacing-md) var(--spacing-sm);
    scrollbar-width: none;
  }

  .mobile-nav::-webkit-scrollbar {
    display: none;
  }

  .mobile-nav-item {
    display: inline-flex;
    align-items: center;
    gap: var(--spacing-xs);
    flex: none;
    min-height: 34px;
    padding: 0 12px;
    border-radius: var(--radius-full);
    color: var(--color-shell-muted);
    font-size: var(--font-size-sm);
    font-weight: 500;
    white-space: nowrap;
  }

  .mobile-nav-item:hover {
    color: var(--color-text);
    background: var(--color-muted-soft);
  }

  .mobile-nav-item.active {
    color: var(--color-text);
    background: var(--color-shell-active);
    font-weight: 600;
  }
}

@media (max-width: 600px) {
  .search-text,
  .search-trigger .kbd,
  .admin {
    display: none;
  }
}
</style>
