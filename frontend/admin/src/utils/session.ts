import type { User } from '../types'

// The back office shares the storefront session: both SPAs read the same
// localStorage key, so an admin who signed in at the store can open /admin
// without a second password. OAuth-only admins have no password to log in with.
export const TOKEN_KEY = 'token'
export const USER_KEY = 'admin_user'
export const REFRESH_KEY = 'refresh_token'
const LEGACY_TOKEN_KEY = 'admin_token'

// Staff are the roles the back office recognises. 运营 and 客服 see fewer
// screens, not a locked door: what they may open comes from the server.
const STAFF_ROLES = ['super_admin', 'admin', 'operator', 'support']

export function isStaffUser(user?: Pick<User, 'role' | 'is_admin'> | null): boolean {
  if (!user) return false
  return user.is_admin === true || (user.role ? STAFF_ROLES.includes(user.role) : false)
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
  localStorage.removeItem(REFRESH_KEY)
  localStorage.removeItem(USER_KEY)
  localStorage.removeItem(LEGACY_TOKEN_KEY)
}

migrateLegacyToken()
