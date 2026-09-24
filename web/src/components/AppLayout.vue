<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { RouterLink, RouterView, useRoute, useRouter } from 'vue-router'
import { LogOut, Menu, Monitor, Moon, Search, Sun, X, ChevronRight, type LucideIcon } from 'lucide-vue-next'
import MenuSearch from '@/components/MenuSearch.vue'
import {
  isTopDialog,
  pushDialog,
  removeDialog,
  useFocusTrap,
  useScrollLock,
} from '@/components/ui/composables'
import { navigationItems, type NavigationItem } from '@/router/navigation'
import { useAuthStore } from '@/stores/auth'
import { useOverviewStore } from '@/stores/overview'
import { useThemeStore, type ThemeMode } from '@/stores/theme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()
const overview = useOverviewStore()
const searchOpen = ref(false)
const drawerOpen = ref(false)
const sidebarRef = ref<HTMLElement | null>(null)
const mobileMedia = window.matchMedia('(max-width: 899px)')
const isMobileShell = ref(mobileMedia.matches)
const drawerActive = computed(() => isMobileShell.value && drawerOpen.value)
const drawerId = Symbol('mobile-navigation')
const appVersion = __APP_VERSION__
const toolbarTitle = computed(() => route.meta.title ?? "控制台")
const toolbarParent = computed(() => route.meta.parentTitle ?? "VPS Node")

const themeOptions: Array<{ mode: ThemeMode; label: string; icon: LucideIcon }> = [
  { mode: 'light', label: '浅色', icon: Sun },
  { mode: 'dark', label: '深色', icon: Moon },
  { mode: 'system', label: '跟随系统', icon: Monitor },
]

const panelState = computed(() => {
  if (overview.loading && !overview.stats) return { label: '检查中', tone: 'primary' }
  if (overview.unavailable) return { label: '不可用', tone: 'danger' }
  if (overview.stale) return { label: '数据陈旧', tone: 'warning' }
  if (overview.stats) return { label: '正常', tone: 'success' }
  return { label: '未检查', tone: 'muted' }
})

const serverState = computed(() => {
  const stats = overview.stats
  if (!stats) return { label: '暂无数据', tone: 'muted' }
  return {
    label: stats.servers_online + ' / ' + stats.servers_total + ' 在线',
    tone: stats.servers_online < stats.servers_total ? 'warning' : 'success',
  }
})

const lastUpdatedLabel = computed(() => {
  const date = overview.lastUpdatedAt
  if (!date) return '暂无'
  return date.toLocaleTimeString('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
})

useScrollLock(drawerActive)
useFocusTrap(sidebarRef, drawerActive)

function isActive(item: NavigationItem): boolean {
  if (item.exact) return route.path === item.to
  return route.path === item.to || route.path.startsWith(item.to + '/')
}

function openDrawer() {
  if (isMobileShell.value) drawerOpen.value = true
}

function closeDrawer() {
  drawerOpen.value = false
}

function onShellMediaChange() {
  isMobileShell.value = mobileMedia.matches
  if (!isMobileShell.value) closeDrawer()
}

function onGlobalKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && drawerActive.value && isTopDialog(drawerId)) {
    event.preventDefault()
    closeDrawer()
    return
  }
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    closeDrawer()
    searchOpen.value = !searchOpen.value
  }
}

async function handleLogout() {
  closeDrawer()
  await auth.logout()
  void router.push({ name: 'login' })
}

watch(drawerActive, (active) => {
  if (active) pushDialog(drawerId)
  else removeDialog(drawerId)
})

watch(
  () => route.fullPath,
  () => closeDrawer(),
)

onMounted(() => {
  window.addEventListener('keydown', onGlobalKeydown)
  mobileMedia.addEventListener('change', onShellMediaChange)
  overview.startPolling()
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onGlobalKeydown)
  mobileMedia.removeEventListener('change', onShellMediaChange)
  removeDialog(drawerId)
  overview.stopPolling()
})
</script>

<template>
  <div class="layout">
    <aside
      id="mobile-primary-navigation"
      ref="sidebarRef"
      aria-label="主导航"
      class="app-sidebar"
      :class="{ open: drawerActive }"
      :aria-hidden="isMobileShell && !drawerOpen ? 'true' : undefined"
      :aria-modal="drawerActive ? 'true' : undefined"
      :inert="isMobileShell && !drawerOpen"
      :role="drawerActive ? 'dialog' : undefined"
      tabindex="-1"
    >
      <div class="sidebar-head">
        <RouterLink
          class="brand"
          to="/"
          aria-label="VPS Node 仪表盘"
          @click="closeDrawer"
        >
          <span class="brand-mark">VN</span>
          <span class="brand-text">VPS Node</span>
        </RouterLink>
        <button
          type="button"
          class="sidebar-close"
          aria-label="关闭导航"
          @click="closeDrawer"
        >
          <X :size="18" />
        </button>
      </div>

      <nav
        class="primary-nav"
        aria-label="主导航"
      >
        <RouterLink
          v-for="item in navigationItems"
          :key="item.to"
          :to="item.to"
          class="nav-item"
          :class="{ active: isActive(item) }"
          @click="closeDrawer"
        >
          <component
            :is="item.icon"
            :size="17"
            aria-hidden="true"
          />
          <span>{{ item.label }}</span>
        </RouterLink>
      </nav>

      <div
        class="sidebar-status"
        aria-label="系统状态"
        aria-live="polite"
      >
        <div class="status-row">
          <span class="status-label">
            <i
              class="status-dot"
              :class="panelState.tone"
            />
            Panel
          </span>
          <strong>{{ panelState.label }}</strong>
        </div>
        <div class="status-row">
          <span class="status-label">
            <i
              class="status-dot"
              :class="serverState.tone"
            />
            服务器
          </span>
          <strong>{{ serverState.label }}</strong>
        </div>
        <div class="status-row status-row-stacked">
          <span>状态刷新</span>
          <strong>{{ lastUpdatedLabel }}</strong>
        </div>
        <div class="status-row status-row-stacked">
          <span>构建修订</span>
          <strong class="mono">{{ appVersion }}</strong>
        </div>
      </div>
    </aside>

    <button
      v-if="drawerActive"
      type="button"
      class="drawer-overlay"
      aria-label="关闭导航"
      @click="closeDrawer"
    />

    <div class="main-column">
      <header class="content-toolbar">
        <div
          class="desktop-route-context"
          aria-label="当前位置"
        >
          <span>{{ toolbarParent }}</span>
          <ChevronRight
            :size="13"
            aria-hidden="true"
          />
          <strong>{{ toolbarTitle }}</strong>
        </div>
        <div class="mobile-shell-start">
          <button
            type="button"
            class="toolbar-icon-button"
            aria-label="打开导航"
            aria-controls="mobile-primary-navigation"
            :aria-expanded="drawerActive"
            @click="openDrawer"
          >
            <Menu :size="18" />
          </button>
          <RouterLink
            class="mobile-brand"
            to="/"
            aria-label="VPS Node 仪表盘"
          >
            <span class="brand-mark">VN</span>
            <span class="mobile-brand-text">VPS Node</span>
          </RouterLink>
        </div>

        <div class="toolbar-actions">
          <button
            type="button"
            class="search-trigger"
            aria-haspopup="dialog"
            :aria-expanded="searchOpen"
            @click="searchOpen = true"
          >
            <Search :size="14" />
            <span class="search-label">搜索菜单</span>
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
                :size="14"
              />
            </button>
          </div>
          <span
            class="admin-avatar"
            :title="auth.username || '管理员'"
          >{{ (auth.username || '管').slice(0, 1).toUpperCase() }}</span>
          <span class="admin-name">{{ auth.username || '管理员' }}</span>
          <button
            type="button"
            class="toolbar-icon-button"
            title="退出登录"
            aria-label="退出登录"
            @click="handleLogout"
          >
            <LogOut :size="15" />
          </button>
        </div>
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
  display: grid;
  grid-template-columns: var(--shell-sidebar-width) minmax(0, 1fr);
  min-height: 100vh;
  min-width: 0;
  background: var(--color-bg);
}

.app-sidebar {
  position: sticky;
  top: 0;
  z-index: 40;
  display: flex;
  width: var(--shell-sidebar-width);
  height: 100vh;
  height: 100dvh;
  min-width: 0;
  flex-direction: column;
  padding: 14px 12px 12px;
  border-right: 1px solid var(--color-shell-border);
  background: var(--color-shell);
}

.sidebar-head {
  display: flex;
  min-height: 42px;
  align-items: center;
  justify-content: space-between;
  padding: 0 6px;
}

.brand,
.mobile-brand {
  display: inline-flex;
  align-items: center;
  gap: 9px;
  min-width: 0;
  color: var(--color-shell-text);
  font-size: var(--font-size-md);
  font-weight: 800;
  white-space: nowrap;
}

.mobile-brand {
  color: var(--color-text);
}

.brand-mark {
  display: grid;
  width: 30px;
  height: 30px;
  flex: none;
  place-items: center;
  border-radius: var(--radius-sm);
  background: #2dd4bf;
  color: #062521;
  font-size: 11px;
  line-height: 1;
}

.sidebar-close {
  display: none;
}

.primary-nav {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
  margin-top: 22px;
}

.nav-item {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 38px;
  align-items: center;
  gap: 10px;
  padding: 0 11px;
  border-radius: var(--radius-sm);
  color: var(--color-shell-muted);
  font-size: var(--font-size-md);
  transition: color 0.15s ease, background 0.15s ease;
}

.nav-item:hover {
  color: var(--color-shell-text);
  background: rgba(255, 255, 255, 0.06);
}

.nav-item.active {
  color: #5eead4;
  background: var(--color-shell-active);
  font-weight: 700;
}

.nav-item.active::before {
  position: absolute;
  top: 9px;
  bottom: 9px;
  left: 0;
  width: 3px;
  border-radius: 0 var(--radius-xs) var(--radius-xs) 0;
  background: #2dd4bf;
  content: '';
}

.sidebar-status {
  display: grid;
  gap: 9px;
  min-width: 0;
  margin-top: auto;
  padding: 13px 10px 4px;
  border-top: 1px solid var(--color-shell-border);
  color: var(--color-shell-muted);
  font-size: var(--font-size-xs);
}

.status-row {
  display: flex;
  min-width: 0;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.status-row-stacked {
  align-items: flex-start;
  flex-direction: column;
  gap: 1px;
}

.status-label {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 6px;
}

.status-row strong {
  min-width: 0;
  overflow: hidden;
  color: var(--color-shell-text);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.main-column {
  min-width: 0;
}

.content-toolbar {
  position: sticky;
  top: 0;
  z-index: 35;
  display: flex;
  min-height: var(--shell-toolbar-height);
  align-items: center;
  padding: 0 var(--spacing-lg);
  border-bottom: 1px solid var(--color-border);
  background: var(--color-toolbar-backdrop);
  backdrop-filter: blur(12px);
}

.desktop-route-context {
  display: inline-flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
}

.desktop-route-context svg {
  flex: none;
}

.desktop-route-context strong {
  overflow: hidden;
  color: var(--color-text);
  font-weight: 700;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.mobile-shell-start {
  display: none;
  min-width: 0;
  align-items: center;
  gap: 9px;
}

.toolbar-actions {
  display: flex;
  min-width: 0;
  align-items: center;
  gap: 7px;
  margin-left: auto;
}

.search-trigger,
.toolbar-icon-button,
.admin-avatar,
.sidebar-close {
  height: 32px;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--color-border);
  border-radius: var(--radius-sm);
  background: var(--color-surface);
  color: var(--color-text-secondary);
}

.search-trigger,
.toolbar-icon-button,
.admin-avatar {
  display: inline-flex;
}

.search-trigger {
  min-width: 148px;
  justify-content: flex-start;
  gap: 7px;
  padding: 0 9px;
  font-size: var(--font-size-xs);
  cursor: pointer;
}

.search-trigger .kbd {
  margin-left: auto;
}

.search-trigger:hover,
.toolbar-icon-button:hover,
.sidebar-close:hover {
  border-color: var(--color-border-strong);
  color: var(--color-text);
  background: var(--color-surface-muted);
}

.toolbar-icon-button,
.admin-avatar,
.sidebar-close {
  width: 32px;
  flex: none;
  padding: 0;
}

.toolbar-icon-button,
.sidebar-close {
  cursor: pointer;
}

.admin-avatar {
  border-color: transparent;
  background: var(--color-surface-muted);
  color: var(--color-text);
  font-size: var(--font-size-xs);
  font-weight: 700;
}

.admin-name {
  max-width: 120px;
  min-width: 0;
  overflow: hidden;
  color: var(--color-text-secondary);
  font-size: var(--font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.content {
  min-width: 0;
}

.drawer-overlay {
  display: none;
}

@media (max-width: 1100px) {
  .admin-name {
    display: none;
  }
}

@media (max-width: 899px) {
  .layout {
    display: block;
  }

  .app-sidebar {
    position: fixed;
    right: auto;
    left: 0;
    z-index: 90;
    width: min(var(--shell-sidebar-width), calc(100vw - 48px));
    max-width: 100%;
    box-shadow: var(--shadow-dialog);
    transform: translateX(-105%);
    transition: transform 0.2s ease;
  }

  .app-sidebar.open {
    transform: translateX(0);
  }

  .sidebar-close {
    display: inline-flex;
  }

  .drawer-overlay {
    position: fixed;
    inset: 0;
    z-index: 80;
    display: block;
    width: 100%;
    height: 100%;
    padding: 0;
    border: 0;
    border-radius: 0;
    background: var(--color-overlay);
    cursor: pointer;
  }

  .content-toolbar {
    padding-inline: var(--spacing-md);
  }

  .desktop-route-context {
    display: none;
  }

  .mobile-shell-start {
    display: flex;
  }
}

@media (max-width: 600px) {
  .content-toolbar {
    padding-inline: 12px;
  }

  .mobile-brand-text,
  .search-label,
  .search-trigger .kbd,
  .admin-avatar {
    display: none;
  }

  .search-trigger {
    min-width: 32px;
    width: 32px;
    padding: 0;
    justify-content: center;
  }

  .theme-switch {
    gap: 0;
  }

  .theme-option {
    width: 26px;
  }
}
</style>
