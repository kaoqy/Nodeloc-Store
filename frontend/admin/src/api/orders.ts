import client from './client'
import type { Order, PaginationParams } from '../types'

export const listOrders = (params?: PaginationParams) =>
  client.get<{data: Order[]}>('/admin/orders', { params }).then(r => r.data.data)

export const getOrder = (no: string) =>
  client.get<{data: Order}>(`/admin/orders/${no}`).then(r => r.data.data)

export const cancelOrder = (no: string) =>
  client.post<{data: Order}>(`/admin/orders/${no}/cancel`).then(r => r.data.data)

export const deliverOrder = (no: string, delivery_content: string) =>
  client.post<{data: Order}>(`/admin/orders/${no}/deliver`, { delivery_content }).then(r => r.data.data)

export const refundOrder = (no: string) =>
  client.post<{data: Order}>(`/admin/orders/${no}/refund`).then(r => r.data.data)
