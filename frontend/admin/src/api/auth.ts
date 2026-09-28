import client from './client'
import type { LoginResponse, User } from '../types'

export interface MyPermissions {
  role: string
  is_staff: boolean
  permissions: string[]
}

export const login = (identifier: string, password: string) =>
  client.post<LoginResponse>('/auth/login', { identifier, password }).then((r) => r.data)

export const me = () => client.get<{ user: User }>('/auth/me').then((r) => r.data)

// The server owns the permission model, so the back office renders from this
// list rather than a copy of the role rules that drifts on every edit.
export const myPermissions = () =>
  client.get<MyPermissions>('/auth/me/permissions').then((r) => r.data)
