import axios from 'axios'
import type { AxiosError, InternalAxiosRequestConfig } from 'axios'
import { clearSession, REFRESH_KEY, TOKEN_KEY } from '../utils/session'
import { beginRequest, endRequest } from '../utils/progress'

const client = axios.create({
  baseURL: '/api/v1',
  headers: { 'Content-Type': 'application/json' },
})

client.interceptors.request.use((config) => {
  beginRequest()
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

// The access token is short-lived; the session is not. A 401 trades the stored
// refresh token for a new pair and replays the original request once, so an
// admin left on the dashboard overnight does not lose a form to an expired
// token. The refresh goes through bare axios so this interceptor cannot
// re-enter itself, and one shared promise keeps a burst of parallel 401s from
// burning the refresh token more than once.
let refreshing: Promise<boolean> | null = null

async function tryRefresh(): Promise<boolean> {
  const refreshToken = localStorage.getItem(REFRESH_KEY)
  if (!refreshToken) return false
  try {
    const { data } = await axios.post('/api/v1/auth/refresh', { refresh_token: refreshToken })
    const accessToken = data?.tokens?.access_token
    if (!accessToken) return false
    localStorage.setItem(TOKEN_KEY, accessToken)
    if (data.tokens.refresh_token) localStorage.setItem(REFRESH_KEY, data.tokens.refresh_token)
    return true
  } catch {
    return false
  }
}

function refreshOnce(): Promise<boolean> {
  if (!refreshing) {
    refreshing = tryRefresh().finally(() => {
      refreshing = null
    })
  }
  return refreshing
}

interface RetriableConfig extends InternalAxiosRequestConfig {
  _retried?: boolean
}

client.interceptors.response.use(
  (response) => {
    endRequest()
    return response
  },
  async (error: AxiosError) => {
    endRequest()
    const config = error.config as RetriableConfig | undefined
    if (error.response?.status !== 401 || !config || config._retried) {
      if (error.response?.status === 401) {
        clearSession()
        if (window.location.pathname !== '/admin/login') window.location.href = '/admin/login'
      }
      return Promise.reject(error)
    }
    // A 401 from the login form itself is a wrong password, not a stale
    // session; refreshing would replace the server's message with a redirect.
    if (!localStorage.getItem(REFRESH_KEY)) return Promise.reject(error)
    if (!(await refreshOnce())) {
      clearSession()
      if (window.location.pathname !== '/admin/login') window.location.href = '/admin/login'
      return Promise.reject(error)
    }
    config._retried = true
    config.headers = config.headers ?? {}
    config.headers.Authorization = `Bearer ${localStorage.getItem(TOKEN_KEY)}`
    return client.request(config)
  },
)

export default client
