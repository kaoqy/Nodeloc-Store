import client from './client'
import type { Order, Page, PageParams } from '../types'

export async function listOrders(params: PageParams = {}): Promise<Page<Order>> {
  const { data } = await client.get('/admin/orders', { params })
  return { data: data.data ?? [], total: Number(data.total ?? 0) }
}

export const getOrder = (orderNo: string) =>
  client.get<{ data: Order }>(`/admin/orders/${orderNo}`).then((r) => r.data.data)

export const cancelOrder = (orderNo: string) =>
  client.post<{ data: Order }>(`/admin/orders/${orderNo}/cancel`).then((r) => r.data.data)

export const refundOrder = (orderNo: string) =>
  client.post<{ data: Order }>(`/admin/orders/${orderNo}/refund`).then((r) => r.data.data)

export const deliverOrder = (orderNo: string, deliveryContent: string) =>
  client
    .post<{ data: Order }>(`/admin/orders/${orderNo}/deliver`, { delivery_content: deliveryContent })
    .then((r) => r.data.data)

// Retries automatic delivery, e.g. after card stock was restocked for an order
// parked in waiting_stock.
export const fulfillOrder = (orderNo: string) =>
  client.post<{ data: Order }>(`/admin/orders/${orderNo}/fulfill`).then((r) => r.data.data)
