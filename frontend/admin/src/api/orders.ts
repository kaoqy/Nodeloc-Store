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

// 查单：让服务端拿这单去问 NodeLoc，已付就直接置为已支付并走发货流程。
// 「尚未到账」不是错误，而是 result.settled === false，所以两者要分开看。
export interface ReconcileResult {
  order: Order
  settled: boolean
  provider_status?: string
  retryable: boolean
  checked_at: string
}

export const reconcileOrder = (orderNo: string) =>
  client.post<ReconcileResult>(`/admin/orders/${orderNo}/reconcile`).then((r) => r.data)

// 批量查单对账：一次问完所有仍显示待支付、但已经拿到 NodeLoc 交易号的订单，
// 买家关页面、回调丢失的单子不用再一笔一笔手点。
export interface ReconcileItem {
  order_no: string
  settled: boolean
  provider_status?: string
  code?: string
  message?: string
  detail?: string
}

export interface ReconcileReport {
  checked_at: string
  checked: number
  settled: number
  items: ReconcileItem[]
}

export const reconcilePendingOrders = () =>
  client.post<ReconcileReport>('/admin/reconcile/pending').then((r) => r.data)
