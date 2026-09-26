import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

declare module 'vue-router' {
  interface RouteMeta {
    public?: boolean
    title?: string
    parentTitle?: string
    parentPath?: string
  }
}

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/pages/LoginPage.vue'),
      meta: { public: true, title: '登录' },
    },
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@/pages/DashboardPage.vue'),
      meta: { title: '仪表盘' },
    },
    {
      path: '/users',
      name: 'users',
      component: () => import('@/pages/UsersPage.vue'),
      meta: { title: '用户' },
    },
    {
      path: '/users/:id',
      name: 'user-detail',
      component: () => import('@/pages/UserDetailPage.vue'),
      meta: { title: '用户详情', parentTitle: '用户', parentPath: '/users' },
    },
    {
      path: '/servers',
      name: 'servers',
      component: () => import('@/pages/ServersPage.vue'),
      meta: { title: '服务器' },
    },
    {
      path: '/nodes',
      name: 'nodes',
      component: () => import('@/pages/NodesPage.vue'),
      meta: { title: '节点' },
    },
    {
      path: '/custom-nodes',
      name: 'custom-nodes',
      component: () => import('@/pages/CustomNodesPage.vue'),
      meta: { title: '自定义节点' },
    },
    {
      path: '/servers/:id',
      name: 'server-detail',
      component: () => import('@/pages/ServerDetailPage.vue'),
      meta: { title: '服务器详情', parentTitle: '服务器', parentPath: '/servers' },
    },
    {
      path: '/visits',
      name: 'visits',
      component: () => import('@/pages/VisitsPage.vue'),
      meta: { title: '访问记录' },
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/pages/SettingsPage.vue'),
      meta: { title: '设置' },
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/pages/NotFoundPage.vue'),
      meta: { title: '页面不存在' },
    },
  ],
})

router.beforeEach(async (to) => {
  if (to.meta.public) return true
  const auth = useAuthStore()
  if (auth.authenticated) return true
  const ok = await auth.hydrate()
  if (ok) return true
  return { name: 'login', query: { redirect: to.fullPath } }
})

router.afterEach((to) => {
  document.title = to.meta.title ? `${to.meta.title} · VPS Node` : 'VPS Node'
})
