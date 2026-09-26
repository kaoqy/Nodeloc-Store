import client from './client'
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

export async function listOrders(limit = 50, offset = 0) {
  const { data } = await client.get<{ orders: Order[]; total: number }>('/payment/orders', {
    params: { limit, offset },
  })
  return data
}
