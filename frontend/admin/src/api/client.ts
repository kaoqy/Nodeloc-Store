import axios from 'axios'
import { clearSession, TOKEN_KEY } from '../utils/session'
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

client.interceptors.response.use(
  (response) => {
    endRequest()
    return response
  },
  (error) => {
    endRequest()
    if (error.response?.status === 401) {
      clearSession()
      if (window.location.pathname !== '/admin/login') window.location.href = '/admin/login'
    }
    return Promise.reject(error)
  },
)

export default client
