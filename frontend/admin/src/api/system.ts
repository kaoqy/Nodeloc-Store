import client from './client'
import { applyBrand } from '../utils/brand'
import { applyShopIdentity } from '../utils/identity'
import type { DashboardStats, RuntimeSettings } from '../types'

export interface SystemStatus {
  initialized: boolean
  version: string
  app?: { name: string; logo?: string }
  theme?: { primary?: string }
}

// Raw axios-free fetch: setup endpoints must not be intercepted (no token
// exists yet and 401/redirect logic would loop).
export async function fetchStatus(): Promise<SystemStatus> {
  const res = await fetch('/api/v1/system/status', { headers: { Accept: 'application/json' } })
  if (!res.ok) throw new Error(`status ${res.status}`)
  const status = (await res.json()) as SystemStatus
  // The router guard reads this before the first view renders, which makes it
  // the earliest place the back office can learn the shop's own colour and name.
  applyBrand(status.theme?.primary)
  applyShopIdentity(status.app?.name, status.app?.logo)
  return status
}

let cachedUninitialized: boolean | null = null

// isUninitialized caches the bootstrap check for the router guard.
export async function isUninitialized(): Promise<boolean> {
  if (cachedUninitialized === null) {
    try {
      const status = await fetchStatus()
      cachedUninitialized = !status.initialized
    } catch {
      cachedUninitialized = false
    }
  }
  return cachedUninitialized
}

export function markInitialized() {
  cachedUninitialized = false
}

export interface InstallPayload {
  app: {
    site_name: string
    site_slogan?: string
    site_description?: string
    site_logo?: string
    scheme: string
    domain: string
  }
  database: { driver: string; dsn?: string }
  oauth: {
    enabled: boolean
    base_url: string
    client_id: string
    client_secret: string
    redirect_uri?: string
    scopes?: string
  }
  payment: { enabled: boolean; payment_id: string; token: string; secret_key: string }
  admin: { username: string; email?: string; password: string }
  features: { enabled_registration: boolean }
  theme: { theme_primary: string; default_locale: string }
}

export async function install(payload: InstallPayload): Promise<void> {
  const res = await fetch('/api/v1/system/install', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!res.ok) {
    let message = `初始化失败（HTTP ${res.status}）`
    try {
      const data = await res.json()
      if (data?.error) message = data.error
    } catch {
      /* keep default */
    }
    throw new Error(message)
  }
  markInitialized()
}

// SettingsDocument is what GET /admin/settings answers. Alongside the editable
// config it carries the server's own reading of whether payments can actually
// run — the owner's switch says what they want, this says what the shop can do.
export interface SettingsDocument {
  settings: RuntimeSettings
  payment_ready?: boolean
  payment_missing?: string[]
  // Credentials that are filled but cannot work: Token and Secret Key swapped, an
  // OAuth Client ID in the Payment ID box. Wording is the server's, in Chinese.
  payment_warnings?: string[]
  // The same reading for 登录: which OAuth box is still empty, and what the store
  // can see is wrong about the ones that are filled — a 重定向 URI on another host
  // than 站点域名 is refused by NodeLoc before the shop ever sees a code.
  oauth_ready?: boolean
  oauth_missing?: string[]
  oauth_warnings?: string[]
}

export const getRuntimeSettings = () =>
  client.get<SettingsDocument>('/admin/settings').then((r) => r.data)

// `restart_pending` tells the page that the row is on disk but the runtime
// rebuild failed, so it must not claim the change is live already.
export const saveRuntimeSettings = (settings: RuntimeSettings) =>
  client
    .put<{ ok: boolean; restart_pending?: boolean; message?: string }>('/admin/settings', { settings })
    .then((r) => r.data)

export const testOAuth = () =>
  client.post<{ ok: boolean; authorize_url?: string; msg?: string }>('/admin/settings/oauth-test').then((r) => r.data)

// OAuthAttempt is one NodeLoc 登录 round trip as the shop recorded it. The
// detail is the shop's own sentence or NodeLoc's, with tokens and codes already
// removed by the server — there is nothing secret left to leak from this list.
export interface OAuthAttempt {
  id: number
  created_at: string
  step: string
  outcome: string
  reason?: string
  detail?: string
  redirect_uri?: string
  username?: string
  binding: boolean
}

export const getOAuthAttempts = (limit = 20) =>
  client
    .get<{ data: OAuthAttempt[] }>('/admin/oauth-attempts', { params: { limit } })
    .then((r) => r.data.data ?? [])

export const testMail = (to: string) =>
  client.post<{ ok: boolean; message?: string }>('/admin/settings/mail-test', { to }).then((r) => r.data)

export const testPayment = () =>
  client.post<{ ok: boolean; msg: string }>('/admin/settings/payment-test').then((r) => r.data)

export const getStats = (days = 30) =>
  client.get<DashboardStats>('/admin/stats', { params: { days } }).then((r) => r.data)
