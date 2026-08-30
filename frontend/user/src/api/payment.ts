import client from './client'
import type { Order } from '../types'

export async function createPayment(payload: { order_no: string; description?: string }) {
  const { data } = await client.post<{ payment_order: { payment_url: string } }>('/payment/create', payload)
  return data
}

export async function getOrder(orderNo: string) {
  const { data } = await client.get<{ order: Order }>(`/payment/orders/${orderNo}`)
  return data
}

export async function listOrders() {
  const { data } = await client.get<{ orders: Order[]; total: number }>('/payment/orders')
  return data
}
