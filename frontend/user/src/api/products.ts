import client from './client'
import type { Category, Product } from '../types'

export async function listProducts(): Promise<Product[]> {
  const { data } = await client.get<{ data: Product[] }>('/store/products')
  return data.data
}

export async function getProduct(slug: string): Promise<Product> {
  const { data } = await client.get<{ data: Product }>(`/store/products/${slug}`)
  return data.data
}

export async function listCategories(): Promise<Category[]> {
  const { data } = await client.get<{ data: Category[] }>('/store/categories')
  return data.data
}
