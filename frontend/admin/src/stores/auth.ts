import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api/auth'
import { clearSession, isStaffUser, REFRESH_KEY, TOKEN_KEY, USER_KEY } from '../utils/session'
import type { User } from '../types'

export { isStaffUser } from '../utils/session'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem(TOKEN_KEY))
  const raw = localStorage.getItem(USER_KEY)
  const user = ref<User | null>(raw ? JSON.parse(raw) : null)
  const accountRole = ref('')
  const permissions = ref<string[]>([])
  const permissionsLoaded = ref(false)

  const isAuthenticated = computed(() => !!token.value)
  // Staff is the door; the permission list decides which rooms behind it are
  // open, so 运营 and 客服 get a shorter navigation instead of a 403 page.
  const isStaff = computed(() => isStaffUser(user.value) || accountRole.value === 'super_admin')
  const isSuperAdmin = computed(() => (user.value?.role || accountRole.value) === 'super_admin')

  // validated caches the /auth/me check performed once per page load, so the
  // router guard does not re-validate on every navigation.
  let validated = false
  let loadingPermissions: Promise<void> | null = null

  function persist() {
    if (token.value) localStorage.setItem(TOKEN_KEY, token.value)
    else clearSession()
    if (token.value && user.value) localStorage.setItem(USER_KEY, JSON.stringify(user.value))
  }

  function adoptTokens(tokens: { access_token: string; refresh_token?: string }) {
    token.value = tokens.access_token
    if (tokens.refresh_token) localStorage.setItem(REFRESH_KEY, tokens.refresh_token)
  }

  // allows answers one question the server already decided: may this account
  // open this screen. While the list is still in flight everything is allowed,
  // because a failed fetch should not hide a working back office — every route
  // is guarded by the API too.
  function allows(resource: string, action = 'view'): boolean {
    if (!permissionsLoaded.value) return true
    if (permissions.value.includes('*:*')) return true
    return permissions.value.includes(`${resource}:${action}`)
  }

  async function loadPermissions() {
    if (loadingPermissions) return loadingPermissions
    loadingPermissions = api
      .myPermissions()
      .then((result) => {
        accountRole.value = result.role
        permissions.value = result.permissions ?? []
        permissionsLoaded.value = true
      })
      .catch(() => {
        permissionsLoaded.value = false
      })
      .finally(() => {
        loadingPermissions = null
      })
    return loadingPermissions
  }

  async function login(identifier: string, password: string) {
    const res = await api.login(identifier, password)
    if (!isStaffUser(res.user)) {
      throw new Error('该账号没有后台管理权限')
    }
    adoptTokens(res.tokens)
    user.value = res.user
    validated = true
    persist()
    await loadPermissions()
  }

  function logout() {
    token.value = null
    user.value = null
    accountRole.value = ''
    permissions.value = []
    permissionsLoaded.value = false
    validated = false
    clearSession()
  }

  async function fetchUser() {
    const res = await api.me()
    user.value = res.user
    localStorage.setItem(USER_KEY, JSON.stringify(res.user))
  }

  // bootstrap confirms the shared token still belongs to a staff account. A
  // storefront buyer keeps their session untouched; they simply cannot enter
  // the back office.
  async function bootstrap(): Promise<boolean> {
    if (!token.value) return false
    if (validated) {
      if (!permissionsLoaded.value) await loadPermissions()
      return isStaff.value
    }
    try {
      await fetchUser()
    } catch {
      return false
    }
    validated = true
    if (!isStaff.value) return false
    await loadPermissions()
    return isStaff.value
  }

  return {
    token,
    user,
    accountRole,
    permissions,
    permissionsLoaded,
    isAuthenticated,
    isStaff,
    isSuperAdmin,
    allows,
    loadPermissions,
    login,
    logout,
    fetchUser,
    bootstrap,
  }
})
