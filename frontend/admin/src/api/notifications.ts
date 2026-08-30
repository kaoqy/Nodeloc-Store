import client from './client'
import type { Notification } from '../types'

export interface NotificationPayload {
  type: string
  title: string
  content?: string
  user_id?: number
  link?: string
}

export const listNotifications = () =>
  client.get<{ items: Notification[]; total: number }>('/notifications').then(r => r.data)

export const markAsRead = (id: number) =>
  client.post(`/notifications/${id}/read`)

export const sendNotification = (payload: NotificationPayload) =>
  client.post('/notifications', payload).then(r => r.data)

export const broadcastNotification = (payload: Omit<NotificationPayload, 'user_id'>) =>
  client.post<{sent:number}>('/admin/notifications/broadcast', payload).then(r => r.data)
