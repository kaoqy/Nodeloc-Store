import client from './client'
import type { Category } from '../types'

export const listCategories = () =>
  client.get<{ data: Category[] }>('/admin/categories').then((r) => r.data.data ?? [])

// The backend binds the category itself as the request body (no wrapper key).
export const createCategory = (category: Partial<Category>) =>
  client.post<{ data: Category }>('/admin/categories', category).then((r) => r.data.data)

export const updateCategory = (id: number, category: Partial<Category>) =>
  client.put<{ data: Category }>(`/admin/categories/${id}`, category).then((r) => r.data.data)

export const deleteCategory = (id: number) => client.delete(`/admin/categories/${id}`)
