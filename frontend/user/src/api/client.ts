import axios from 'axios'
import type { AxiosError, InternalAxiosRequestConfig } from 'axios'
import { beginRequest, endRequest } from '../utils/progress'

const client = axios.create({ baseURL: '/api/v1' })

const TOKEN_KEY = 'token'
const REFRESH_KEY = 'refresh_token'

client.interceptors.request.use((config) => {
  beginRequest()
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

/**
 * A session lasts as long as the refresh token, not the two-hour access token:
 * a 401 quietly trades the refresh token for a new session and replays the
 * original request once. Only a failed refresh ends the session.
 *
 * The call goes through a bare axios instance so this interceptor cannot
 * re-enter itself, and one shared promise keeps a burst of parallel 401s from
 * spending the refresh token repeatedly.
 */
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

function endSession(allowRetry = true) {
  clearSession()
  if (!allowRetry) return
  if (window.location.pathname === '/login') return
  const current = window.location.pathname + window.location.search
  window.location.href = `/login?redirect=${encodeURIComponent(current)}`
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
      if (error.response?.status === 401 && localStorage.getItem(TOKEN_KEY)) endSession()
      return Promise.reject(error)
    }
    // An anonymous endpoint answering 401 means bad credentials, not a stale
    // session; refreshing would only hide the real message.
    if (!localStorage.getItem(REFRESH_KEY)) {
      endSession(false)
      return Promise.reject(error)
    }
    if (!(await refreshOnce())) {
      endSession()
      return Promise.reject(error)
    }
    config._retried = true
    config.headers = config.headers ?? {}
    config.headers.Authorization = `Bearer ${localStorage.getItem(TOKEN_KEY)}`
    return client.request(config)
  },
)

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY)
  localStorage.removeItem(REFRESH_KEY)
}

/** HTTP status of a failed request, or undefined when the server never answered. */
export function errorStatus(error: unknown): number | undefined {
  return axios.isAxiosError(error) ? error.response?.status : undefined
}

/**
 * Machine-readable reason the API attached to a failure. Status codes alone
 * could not tell "NodeLoc has not seen this payment yet" apart from "the shop
 * never got a transaction id", which are different next steps for a buyer.
 */
export function errorCode(error: unknown): string {
  const data = (axios.isAxiosError(error) ? error.response?.data : undefined) as { code?: string } | undefined
  return data?.code ?? ''
}

/** Whether retrying the same call can plausibly succeed (a provider outage will, bad credentials will not). */
export function errorRetryable(error: unknown): boolean {
  const data = (axios.isAxiosError(error) ? error.response?.data : undefined) as { retryable?: boolean } | undefined
  return data?.retryable === true
}

/** A human-readable reason for a failed request, preferring the API's own text. */
export function errorMessage(error: unknown, fallback = '操作失败，请稍后重试'): string {
  if (axios.isAxiosError(error)) {
    const data = error.response?.data as { error?: string; message?: string } | undefined
    if (data?.error) return data.error
    if (data?.message) return data.message
  }
  return fallback
}

export default client
