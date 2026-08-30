import client from './client'
import type { Coupon } from '../types'

export async function listCoupons() {
  const { data } = await client.get<{ data: Coupon[] }>('/admin/coupons')
  return data
}

export async function createCoupon(coupon: Partial<Coupon>) {
  const { data } = await client.post('/admin/coupons', { coupon })
  return data
}

export async function updateCoupon(id: number, coupon: Partial<Coupon>) {
  const { data } = await client.put(`/admin/coupons/${id}`, { coupon })
  return data
}

export async function deleteCoupon(id: number) {
  await client.delete(`/admin/coupons/${id}`)
}
