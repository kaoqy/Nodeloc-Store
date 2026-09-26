import axios from 'axios'

const client = axios.create({ baseURL: '/api/v1' })

client.interceptors.request.use((config) => {
  const token = localStorage.getItem('token')
  if (token) config.headers.Authorization = `Bearer ${token}`
  return config
})

client.interceptors.response.use(
  (response) => response,
  (error) => {
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

/** A human-readable reason for a failed request, preferring the API's own text. */
export function errorMessage(error: unknown, fallback = '操作失败，请稍后重试'): string {
  if (axios.isAxiosError(error)) {
    const message = (error.response?.data as { error?: string } | undefined)?.error
    if (message) return message
  }
  return fallback
}

export default client
