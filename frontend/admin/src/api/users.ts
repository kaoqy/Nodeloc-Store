import client from './client'
import type { Page, PageParams, Transfer, User } from '../types'

export interface AdminUserQuery {
  limit?: number
  offset?: number
  q?: string
  role?: string
}

export async function listUsers(params: AdminUserQuery = {}): Promise<Page<User>> {
  const { data } = await client.get('/admin/users', {
    params: {
      limit: params.limit,
      offset: params.offset || undefined,
      q: params.q?.trim() || undefined,
      role: params.role || undefined,
    },
  })
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

export const adjustPoints = (id: number, delta: number, reason?: string) =>
  client
    .post<{ user: User }>(`/admin/users/${id}/points`, { delta, reason })
    .then((r) => r.data.user)

// 转账 goes through NodeLoc, not the shop's own 积分 counter: the buyer's balance
// lives at the provider, so the only honest way to pay them is the provider's
// transfer API. The response carries the ledger row either way.
export const grantTransfer = (id: number, payload: { amount: number; note?: string }) =>
  client.post<{ data: Transfer }>(`/admin/users/${id}/transfer`, payload).then((r) => r.data.data)

export const listUserTransfers = (id: number, params: PageParams = {}) =>
  client
    .get<{ data: Transfer[]; total: number }>(`/admin/users/${id}/transfers`, { params })
    .then((r) => ({ data: r.data.data ?? [], total: Number(r.data.total ?? 0) }))

export const listTransfers = (params: PageParams = {}) =>
  client
    .get<{ data: Transfer[]; total: number }>('/admin/transfers', { params })
    .then((r) => ({ data: r.data.data ?? [], total: Number(r.data.total ?? 0) }))
