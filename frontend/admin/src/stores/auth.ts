import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import * as api from '../api/auth'
import { clearSession, isAdminUser, TOKEN_KEY, USER_KEY } from '../utils/session'
import type { User } from '../types'

export { isAdminUser } from '../utils/session'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem(TOKEN_KEY))
  const raw = localStorage.getItem(USER_KEY)
  const user = ref<User | null>(raw ? JSON.parse(raw) : null)
  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => isAdminUser(user.value))

  // validated caches the /auth/me check performed once per page load, so the
  // router guard does not re-validate on every navigation.
  let validated = false

  function persist() {
    if (token.value) localStorage.setItem(TOKEN_KEY, token.value)
    else clearSession()
    if (token.value && user.value) localStorage.setItem(USER_KEY, JSON.stringify(user.value))
  }

  async function login(identifier: string, password: string) {
    const res = await api.login(identifier, password)
    if (!isAdminUser(res.user)) {
      throw new Error('该账号不是管理员，无法进入后台')
    }
    token.value = res.tokens.access_token
    user.value = res.user
    validated = true
    persist()
  }

  function logout() {
    token.value = null
    user.value = null
    validated = false
    clearSession()
  }

  async function fetchUser() {
    const res = await api.me()
    user.value = res.user
    localStorage.setItem(USER_KEY, JSON.stringify(res.user))
  }

  // bootstrap confirms the shared token still belongs to an admin account. A
  // non-admin keeps their storefront session untouched; they simply cannot
  // enter the back office.
  async function bootstrap(): Promise<boolean> {
    if (!token.value) return false
    if (validated) return isAdmin.value
    try {
      await fetchUser()
    } catch {
      return false
    }
    validated = true
    return isAdmin.value
  }

  return { token, user, isAuthenticated, isAdmin, login, logout, fetchUser, bootstrap }
})
