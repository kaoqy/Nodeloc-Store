import client from './client'
import type { Notification } from '../types'

export interface NotificationPage {
  items: Notification[]
  total: number
  page: number
  page_size: number
}

export interface NotificationPayload {
  type: string
  title: string
  content?: string
  link?: string
}

export async function listNotifications(page = 1, pageSize = 20): Promise<NotificationPage> {
  const { data } = await client.get<NotificationPage>('/notifications', { params: { page, page_size: pageSize } })
  return { items: data.items ?? [], total: Number(data.total ?? 0), page: data.page, page_size: data.page_size }
}

export const markAsRead = (id: number) => client.post(`/notifications/${id}/read`)

// Send to one user; the backend requires an explicit user_id.
export const sendNotification = (payload: NotificationPayload & { user_id: number }) =>
  client.post('/notifications', payload).then((r) => r.data)

export const broadcastNotification = (payload: NotificationPayload) =>
  client.post<{ sent: number }>('/admin/notifications/broadcast', payload).then((r) => r.data)
