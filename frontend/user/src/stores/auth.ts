import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as authApi from '../api/auth'
import { clearSession } from '../api/client'
import type { MyPermissions, User } from '../types'

const TOKEN_KEY = 'token'
const REFRESH_KEY = 'refresh_token'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem(TOKEN_KEY) || '')
  const user = ref<User | null>(null)
  const permissions = ref<string[]>([])
  const accountRole = ref('')
  const isStaff = ref(false)
  const isAuthenticated = computed(() => Boolean(token.value))

  /** The back office is a role question, not an is_admin flag: 运营 and 客服
   *  belong there too, and the server already answers who does. */
  const canEnterAdmin = computed(() => isStaff.value || Boolean(user.value?.is_admin))

  function allows(resource: string, action: string): boolean {
    if (permissions.value.includes('*:*')) return true
    return permissions.value.includes(`${resource}:${action}`)
  }

  function saveSession(accessToken: string, currentUser?: User | null, refreshToken?: string) {
    token.value = accessToken
    user.value = currentUser ?? null
    permissions.value = []
    accountRole.value = ''
    isStaff.value = false
    localStorage.setItem(TOKEN_KEY, accessToken)
    localStorage.removeItem(REFRESH_KEY)
    if (refreshToken) localStorage.setItem(REFRESH_KEY, refreshToken)
  }

  async function login(identifier: string, password: string) {
    const response = await authApi.login({ identifier, password })
    saveSession(response.tokens.access_token, response.user, response.tokens.refresh_token)
    void loadPermissions()
    return response.user
  }

  async function register(username: string, email: string, password: string) {
    const response = await authApi.register({ username, email: email || undefined, password })
    saveSession(response.tokens.access_token, response.user, response.tokens.refresh_token)
    void loadPermissions()
    return response.user
  }

  function logout() {
    token.value = ''
    user.value = null
    permissions.value = []
    accountRole.value = ''
    isStaff.value = false
    clearSession()
  }

  async function fetchUser() {
    if (!token.value) return null
    try {
      const response = await authApi.me()
      user.value = response.user
      void loadPermissions()
      return response.user
    } catch (error) {
      logout()
      throw error
    }
  }

  /** The server is the only copy of the permission rules; the client just
   *  mirrors its answer so menus and a 403 can never disagree. */
  async function loadPermissions() {
    if (!token.value) return
    try {
      const response = await authApi.myPermissions()
      permissions.value = response.permissions ?? []
      accountRole.value = response.role
      isStaff.value = response.is_staff
    } catch {
      permissions.value = []
      isStaff.value = false
    }
  }

  return {
    user,
    token,
    permissions,
    accountRole,
    isStaff,
    isAuthenticated,
    canEnterAdmin,
    allows,
    saveSession,
    login,
    logout,
    register,
    fetchUser,
    loadPermissions,
  }
})
