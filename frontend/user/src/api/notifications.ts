import client from './client'
import type { AppNotification } from '../types'

export interface InboxKind {
  type: string
  total: number
  unread: number
}

export interface NotificationPage {
  items: AppNotification[]
  total: number
  page: number
  page_size: number
  kinds: InboxKind[]
}

export interface InboxQuery {
  kind?: string
  unreadOnly?: boolean
}

export async function listNotifications(page = 1, pageSize = 20, query: InboxQuery = {}): Promise<NotificationPage> {
  const { data } = await client.get<NotificationPage>('/notifications', {
    params: { page, page_size: pageSize, type: query.kind || undefined, unread: query.unreadOnly ? 1 : undefined },
  })
  return {
    items: data.items ?? [],
    total: data.total ?? 0,
    page: data.page ?? page,
    page_size: data.page_size ?? pageSize,
    // Tabs come from the server because it is the only one who knows whether a
    // kind exists: offering 订单 to a buyer with no order messages is a dead tab.
    kinds: data.kinds ?? [],
  }
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
