import client from './client'
import type { AppNotification } from '../types'

export interface NotificationPage {
  items: AppNotification[]
  total: number
  page: number
  page_size: number
}

export async function listNotifications(page = 1, pageSize = 20): Promise<NotificationPage> {
  const { data } = await client.get<NotificationPage>('/notifications', {
    params: { page, page_size: pageSize },
  })
  return { items: data.items ?? [], total: data.total ?? 0, page: data.page ?? page, page_size: data.page_size ?? pageSize }
}

export async function markNotificationRead(id: number): Promise<void> {
  await client.post(`/notifications/${id}/read`)
}

/** How many messages are still unread, counted over the whole inbox. */
export async function unreadCount(): Promise<number> {
  const { data } = await client.get<{ unread: number }>('/notifications/unread')
  return Number(data.unread ?? 0)
}

/** Mark everything read at once; returns how many rows actually flipped. */
export async function markAllRead(): Promise<number> {
  const { data } = await client.post<{ marked: number }>('/notifications/read-all')
  return Number(data.marked ?? 0)
}
