import client from './client'
import type { AuthResponse, User } from '../types'

export async function register(payload: { username: string; email?: string; password: string }) {
  const { data } = await client.post<AuthResponse>('/auth/register', payload)
  return data
}

export async function login(payload: { identifier: string; password: string }) {
  const { data } = await client.post<AuthResponse>('/auth/login', payload)
  return data
}

export function oauthInitiate(bind = false) {
  window.location.href = `/api/v1/auth/oauth/initiate?redirect=true${bind ? '&bind=true' : ''}`
}

export async function oauthCallback(code: string, state: string) {
  const { data } = await client.get<AuthResponse>('/auth/oauth/callback', { params: { code, state } })
  return data
}

export async function me() {
  const { data } = await client.get<{ user: User }>('/auth/me')
  return data
}

/** Redeem a NodeLoc authorization code for the signed-in local account. */
export async function bindOAuth(code: string, state: string) {
  const { data } = await client.post<{ user: User }>('/auth/bind-oauth', {
    code,
    params: { code, state },
  })
  return data.user
}

export async function unbindOAuth() {
  const { data } = await client.delete<{ user: User }>('/auth/unbind-oauth')
  return data
}
