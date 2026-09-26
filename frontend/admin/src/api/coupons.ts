import client from './client'
import type { Coupon } from '../types'

export const listCoupons = () =>
  client.get<{ data: Coupon[] }>('/admin/coupons').then((r) => r.data.data ?? [])

// The backend binds the coupon itself as the request body (no wrapper key).
export const createCoupon = (coupon: Partial<Coupon>) =>
  client.post<{ data: Coupon }>('/admin/coupons', coupon).then((r) => r.data.data)

export const updateCoupon = (id: number, coupon: Partial<Coupon>) =>
  client.put<{ data: Coupon }>(`/admin/coupons/${id}`, coupon).then((r) => r.data.data)

export const deleteCoupon = (id: number) => client.delete(`/admin/coupons/${id}`)
