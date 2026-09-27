import axios from 'axios'
import { beginRequest, endRequest } from '../utils/progress'

const client = axios.create({ baseURL: '/api/v1' })

client.interceptors.request.use((config) => {
  beginRequest()
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

client.interceptors.response.use(
  (response) => {
    endRequest()
    return response
  },
  (error) => {
    endRequest()
    if (error.response?.status === 401 && localStorage.getItem('token')) {
      localStorage.removeItem('token')
      if (window.location.pathname !== '/login') {
        const redirect = window.location.pathname + window.location.search
        window.location.href = `/login?redirect=${encodeURIComponent(redirect)}`
      }
    }
    return Promise.reject(error)
  },
)

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
    const message = (error.response?.data as { error?: string } | undefined)?.error
    if (message) return message
  }
  return fallback
}

export default client
