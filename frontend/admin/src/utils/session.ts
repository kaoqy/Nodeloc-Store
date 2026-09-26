import type { User } from '../types'

// The back office shares the storefront session: both SPAs read the same
// localStorage key, so an admin who signed in at the store can open /admin
// without a second password. OAuth-only admins have no password to log in with.
export const TOKEN_KEY = 'token'
export const USER_KEY = 'admin_user'
const LEGACY_TOKEN_KEY = 'admin_token'
const ADMIN_ROLES = ['admin', 'super_admin']

export function isAdminUser(user?: Pick<User, 'role' | 'is_admin'> | null): boolean {
  if (!user) return false
  return user.is_admin === true || (user.role ? ADMIN_ROLES.includes(user.role) : false)
}

// migrateLegacyToken adopts the token key used before the sessions were shared.
export function migrateLegacyToken() {
  const legacy = localStorage.getItem(LEGACY_TOKEN_KEY)
  if (!legacy) return
  if (!localStorage.getItem(TOKEN_KEY)) localStorage.setItem(TOKEN_KEY, legacy)
  localStorage.removeItem(LEGACY_TOKEN_KEY)
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(USER_KEY)
  localStorage.removeItem(LEGACY_TOKEN_KEY)
}

migrateLegacyToken()
