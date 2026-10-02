import client from './client'
import type {
  AuthResponse,
  CheckinResult,
  CheckinRecord,
  CheckinStatus,
  MyPermissions,
  PointEntry,
  User,
} from '../types'

export async function register(payload: { username: string; email?: string; password: string }) {
  const { data } = await client.post<AuthResponse>('/auth/register', payload)
  return data
}

export async function login(payload: { identifier: string; password: string }) {
  const { data } = await client.post<AuthResponse>('/auth/login', payload)
  return data
}

/** Trades a stored refresh token for a live session; the server re-reads the
 *  account, so a revoked admin cannot keep admin claims on the old token. */
export async function refresh(refreshToken: string) {
  const { data } = await client.post<AuthResponse>('/auth/refresh', { refresh_token: refreshToken })
  return data
}

export function oauthInitiate(bind = false) {
  const path = bind ? '/api/v1/auth/oauth/bind-initiate' : '/api/v1/auth/oauth/initiate'
  const returnURL = bind ? '/profile' : '/oauth/callback'
  window.location.href = `${path}?redirect=true&return_url=${encodeURIComponent(returnURL)}`
}

export async function oauthCallback(code: string, state: string) {
  const { data } = await client.get<AuthResponse>('/auth/oauth/callback', { params: { code, state } })
  return data
}

export async function me() {
  const { data } = await client.get<{ user: User }>('/auth/me')
  return data
}

/** The fields a buyer edits themselves. Only present keys are sent. */
export async function updateProfile(payload: {
  nickname?: string
  avatar_url?: string
  bio?: string
  email?: string
}) {
  const { data } = await client.patch<{ user: User }>('/auth/me', payload)
  return data.user
}

/**
 * A buyer's own picture, kept by the shop. The server names the file, points the
 * account at it and drops the avatar this one replaces, so the response carries
 * the whole updated account rather than just an address to paste somewhere.
 */
export async function uploadAvatar(file: File) {
  const form = new FormData()
  form.append('image', file)
  const { data } = await client.post<{ user: User; url: string }>('/auth/me/avatar', form)
  return data
}

export async function myPermissions() {
  const { data } = await client.get<MyPermissions>('/auth/me/permissions')
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

/** Pulls the latest 头像/昵称/邮箱 from NodeLoc for an already-bound account. */
export async function syncOAuthProfile() {
  const { data } = await client.post<{ user: User }>('/auth/me/sync-oauth')
  return data.user
}

export async function checkinStatus() {
  const { data } = await client.get<CheckinStatus>('/auth/checkin/status')
  return data
}

export async function checkinHistory(limit = 30) {
  const { data } = await client.get<{ data: CheckinRecord[] }>('/auth/checkin/history', {
    params: { limit },
  })
  return data.data ?? []
}

export async function checkIn() {
  const { data } = await client.post<CheckinResult>('/auth/checkin')
  return data
}

export interface PointsResult {
  data: PointEntry[]
  total: number
  limit: number
  offset: number
}

export async function myPoints(limit = 20, offset = 0) {
  const { data } = await client.get<PointsResult>('/auth/me/points', { params: { limit, offset } })
  return data
}
