import { createRouter, createWebHistory } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/pages/LoginPage.vue'),
      meta: { public: true },
    },
    {
      path: '/',
      name: 'dashboard',
      component: () => import('@/pages/DashboardPage.vue'),
    },
    {
      path: '/users',
      name: 'users',
      component: () => import('@/pages/UsersPage.vue'),
    },
    {
      path: '/users/:id',
      name: 'user-detail',
      component: () => import('@/pages/UserDetailPage.vue'),
    },
    {
      path: '/servers',
      name: 'servers',
      component: () => import('@/pages/ServersPage.vue'),
    },
    {
      path: '/servers/:id',
      name: 'server-detail',
      component: () => import('@/pages/ServerDetailPage.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/pages/SettingsPage.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      redirect: '/',
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
