import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import * as authApi from '../api/auth'
import type { User } from '../types'

export const useAuthStore = defineStore('auth', () => {
  const token = ref(localStorage.getItem('token') || '')
  const user = ref<User | null>(null)
  const isAuthenticated = computed(() => Boolean(token.value))

  function saveSession(accessToken: string, currentUser: User) {
    token.value = accessToken
    user.value = currentUser
    localStorage.setItem('token', accessToken)
  }

  async function login(identifier: string, password: string) {
    const response = await authApi.login({ identifier, password })
    saveSession(response.tokens.access_token, response.user)
    return response.user
  }

  async function register(username: string, email: string, password: string) {
    const response = await authApi.register({ username, email: email || undefined, password })
    saveSession(response.tokens.access_token, response.user)
    return response.user
  }

  function logout() {
    token.value = ''
    user.value = null
    localStorage.removeItem('token')
  }

  async function fetchUser() {
    if (!token.value) return null
    try {
      const response = await authApi.me()
      user.value = response.user
      return response.user
    } catch (error) {
      logout()
      throw error
    }
  }

  return { user, token, isAuthenticated, login, logout, register, fetchUser }
})
