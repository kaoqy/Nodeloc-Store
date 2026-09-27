import client, { errorCode, errorMessage, errorRetryable } from './client'
import type { Order, PaymentOrder } from '../types'

/** What one 查单 established: the order, and whether NodeLoc confirmed it paid. */
export interface ReconcileResult {
  order: Order
  settled: boolean
  provider_status?: string
  retryable: boolean
  checked_at: string
}

export interface CreateOrderPayload {
  slug: string
  quantity: number
  contact?: string
  note?: string
}

export async function createOrder(payload: CreateOrderPayload): Promise<Order> {
  const { data } = await client.post<{ order: Order }>('/payment/orders', payload)
  return data.order
}

export async function createPayment(orderNo: string, description?: string): Promise<PaymentOrder> {
  const { data } = await client.post<{ payment_order: PaymentOrder }>('/payment/create', {
    order_no: orderNo,
    description,
  })
  return data.payment_order
}

export async function getOrder(orderNo: string): Promise<Order> {
  const { data } = await client.get<{ order: Order }>(`/payment/orders/${orderNo}`)
  return data.order
}

/**
 * 查单：让服务端向 NodeLoc 确认这单的支付结果，已付则立即入账并交付。
 * 回调没能到达商店时（签名不一致、浏览器中途关闭）靠它兜底。
 * 「尚未到账」是正常的回答而不是错误，所以看 settled 而不是只看有没有抛异常。
 */
export async function reconcileOrder(orderNo: string): Promise<ReconcileResult> {
  const { data } = await client.post<ReconcileResult>(`/payment/orders/${orderNo}/reconcile`)
  return data
}

/**
 * 查单失败的买家文案。服务端已经按 code 给出了能照着做的中文原因，这里只兜住
 * 服务端没说话的情况（例如反向代理直接回了 HTML），绝不再把状态码猜成文案。
 */
export function reconcileMessage(error: unknown): string {
  return errorMessage(error, '暂时无法向 NodeLoc 确认支付结果，商店会自动重试，你也可以稍后再查一次。')
}

/** Whether the buyer can usefully press 再查一次, or should wait for the shop. */
export function reconcileRetryable(error: unknown): boolean {
  if (errorCode(error) === 'no_transaction') return false
  return errorRetryable(error)
}

export async function listOrders(limit = 50, offset = 0, status = '', q = '') {
  const { data } = await client.get<{ orders: Order[]; total: number }>('/payment/orders', {
    params: { limit, offset, status: status || undefined, q: q.trim() || undefined },
  })
  return data
}
