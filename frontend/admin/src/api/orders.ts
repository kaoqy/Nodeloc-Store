import client from './client'
import type { Order, Page, PageParams } from '../types'

export interface AdminOrderQuery {
  limit?: number
  offset?: number
  status?: string
  q?: string
  user_id?: number
  attention?: string
}

export async function listOrders(params: AdminOrderQuery = {}): Promise<Page<Order>> {
  const { data } = await client.get('/admin/orders', {
    params: {
      limit: params.limit,
      offset: params.offset || undefined,
      status: params.status || undefined,
      q: params.q?.trim() || undefined,
      user_id: params.user_id || undefined,
      attention: params.attention || undefined,
    },
  })
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
  // 「query」是 NodeLoc 的查单接口，「reprocess」是商店改用下单核实这一单。
  provider_via?: string
  // 查单没能用上时 NodeLoc 的原话，只发给后台。
  provider_note?: string
}

export const reconcileOrder = (orderNo: string) =>
  client.post<ReconcileResult>(`/admin/orders/${orderNo}/reconcile`).then((r) => r.data)

// 批量查单对账：一次问完所有仍显示待支付、但已经拿到 NodeLoc 交易号的订单，
// 买家关页面、回调丢失的单子不用再一笔一笔手点。
export interface ReconcileItem {
  order_no: string
  settled: boolean
  provider_status?: string
  provider_via?: string
  provider_note?: string
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

// 导出走 blob：这个接口在 Authorization 头后面，普通的下载链接会被当成未登录。
// 参数与列表同一套，所以下下来的就是屏幕上那一批单子。
export interface OrderExportQuery {
  status?: string
  q?: string
  user_id?: number
  attention?: string
}

export const exportOrders = async (query: OrderExportQuery = {}) => {
  const response = await client.get<Blob>('/admin/orders/export', {
    responseType: 'blob',
    params: {
      status: query.status || undefined,
      q: query.q?.trim() || undefined,
      user_id: query.user_id || undefined,
      attention: query.attention || undefined,
    },
  })
  const data = response.data
  const url = URL.createObjectURL(data)
  const link = document.createElement('a')
  link.href = url
  link.download = `orders-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
  // 行数与「有没有被截断」由服务端在响应头里报数，不靠前端数行 —— 商品名里
  // 换一个回车就会把行数数错。
  return {
    rows: Number(response.headers['x-export-rows'] ?? 0),
    truncated: response.headers['x-export-truncated'] === '1',
  }
}
