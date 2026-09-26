import client from './client'
import type { Page, PageParams, User } from '../types'

export async function listUsers(params: PageParams = {}): Promise<Page<User>> {
  const { data } = await client.get('/admin/users', { params })
  return { data: data.data ?? [], total: Number(data.total ?? 0) }
}

export const getUser = (id: number) =>
  client.get<{ user: User }>(`/admin/users/${id}`).then((r) => r.data.user)

export const setRole = (id: number, role: string) =>
  client.post<{ user: User }>(`/admin/users/${id}/role`, { role }).then((r) => r.data.user)

export const toggleAdmin = (id: number) =>
  client.post<{ user: User }>(`/admin/users/${id}/toggle-admin`).then((r) => r.data.user)

export const toggleActive = (id: number) =>
  client.post<{ user: User }>(`/admin/users/${id}/toggle-active`).then((r) => r.data.user)

export const adjustPoints = (id: number, delta: number) =>
  client.post<{ user: User }>(`/admin/users/${id}/points`, { delta }).then((r) => r.data.user)
