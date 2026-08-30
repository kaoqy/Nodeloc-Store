import client from './client'
import type { Category } from '../types'

export async function listCategories() {
  const { data } = await client.get<{ data: Category[] }>('/admin/categories')
  return data
}

export async function createCategory(category: Partial<Category>) {
  const { data } = await client.post('/admin/categories', { category })
  return data
}

export async function updateCategory(id: number, category: Partial<Category>) {
  const { data } = await client.put(`/admin/categories/${id}`, { category })
  return data
}

export async function deleteCategory(id: number) {
  await client.delete(`/admin/categories/${id}`)
}
