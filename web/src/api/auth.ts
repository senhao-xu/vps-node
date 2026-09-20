import { request } from './http'
import type { Admin } from './types'

interface AdminEnvelope {
  admin: Admin
}

export async function loginAdmin(username: string, password: string): Promise<Admin> {
  const res = await request<AdminEnvelope>('/api/admin/login', {
    method: 'POST',
    body: { username, password },
    authRedirect: false,
  })
  return res.admin
}

export async function logoutAdmin(): Promise<void> {
  await request<unknown>('/api/admin/logout', { method: 'POST', authRedirect: false })
}

export async function getAdminMe(): Promise<Admin> {
  const res = await request<AdminEnvelope>('/api/admin/me', { authRedirect: false })
  return res.admin
}
