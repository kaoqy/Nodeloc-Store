import client from './client'
import type { Category, Product, ProductListResponse } from '../types'

export async function listProducts(params: { q?: string; category?: string; page?: number } = {}) {
  const { data } = await client.get<ProductListResponse>('/store/products', { params })
  return data
}

export async function getProduct(slug: string) {
  const { data } = await client.get<{ data: Product }>(`/store/products/${slug}`)
  return data
}

export async function listCategories() {
  const { data } = await client.get<{ data: Category[] }>('/store/categories')
  return data
}
