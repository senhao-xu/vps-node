import { ref } from 'vue'
import { defineStore } from 'pinia'
import { getAdminMe, loginAdmin, logoutAdmin } from '@/api/auth'
import type { Admin } from '@/api/types'

export const useAuthStore = defineStore('auth', () => {
  const adminId = ref<number | null>(null)
  const username = ref('')
  const authenticated = ref(false)
  let hydration: Promise<boolean> | null = null

  function setSession(admin: Admin) {
    adminId.value = admin.id
    username.value = admin.username
    authenticated.value = true
  }

  function clearSession() {
    adminId.value = null
    username.value = ''
    authenticated.value = false
  }

  function hydrate(): Promise<boolean> {
    if (hydration) return hydration
    hydration = getAdminMe()
      .then((admin) => {
        setSession(admin)
        return true
      })
      .catch(() => {
        clearSession()
        return false
      })
    return hydration
  }

  async function login(name: string, password: string): Promise<void> {
    const admin = await loginAdmin(name, password)
    setSession(admin)
  }

  async function logout(): Promise<void> {
    try {
      await logoutAdmin()
    } finally {
      clearSession()
      hydration = Promise.resolve(false)
    }
  }

  return { adminId, username, authenticated, hydrate, login, logout }
})
