import client, { errorStatus } from './client'
import type { Order, PaymentOrder } from '../types'

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
 */
export async function reconcileOrder(orderNo: string): Promise<Order> {
  const { data } = await client.post<{ order: Order }>(`/payment/orders/${orderNo}/reconcile`)
  return data.order
}

/**
 * 查单的失败原因在服务端是英文技术文案，这里换成买家能照着做的提示。
 */
export function reconcileMessage(error: unknown): string {
  switch (errorStatus(error)) {
    case 404:
    case 409:
      return 'NodeLoc 暂时没有这单的到账记录。请确认付款已完成，稍后再查一次，或联系店家。'
    case 400:
      return 'NodeLoc 记录的金额与本单不一致，商店已暂停自动入账，请联系店家核对。'
    default:
      // 例如 NodeLoc 查询接口本身报错：原文是给运维看的，买家只需要知道可以重试。
      return '暂时无法向 NodeLoc 确认支付结果，请稍后再查一次。'
  }
}

export async function listOrders(limit = 50, offset = 0, status = '', q = '') {
  const { data } = await client.get<{ orders: Order[]; total: number }>('/payment/orders', {
    params: { limit, offset, status: status || undefined, q: q.trim() || undefined },
  })
  return data
}
