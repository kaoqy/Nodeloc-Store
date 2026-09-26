import client from './client'
import type { Card, Product } from '../types'

export const listProducts = () =>
  client.get<{ data: Product[] }>('/admin/products').then((r) => r.data.data ?? [])

export const getProduct = (id: number) =>
  client.get<{ data: Product }>(`/admin/products/${id}`).then((r) => r.data.data)

// The backend binds the product itself as the request body (no wrapper key).
export const createProduct = (product: Partial<Product>) =>
  client.post<{ data: Product }>('/admin/products', product).then((r) => r.data.data)

export const updateProduct = (id: number, product: Partial<Product>) =>
  client.put<{ data: Product }>(`/admin/products/${id}`, product).then((r) => r.data.data)

export const deleteProduct = (id: number) => client.delete(`/admin/products/${id}`)

export const listCards = (productId: number) =>
  client.get<{ data: Card[] }>(`/admin/products/${productId}/cards`).then((r) => r.data.data ?? [])

export const addCard = (productId: number, content: string, status = 'available') =>
  client.post<{ data: Card }>(`/admin/products/${productId}/cards`, { content, status }).then((r) => r.data.data)

export const batchAddCards = (productId: number, contents: string[]) =>
  client
    .post<{ data: Card[]; count: number }>(`/admin/products/${productId}/cards/batch-add`, { cards: contents })
    .then((r) => r.data)

export const updateCard = (productId: number, cardId: number, patch: Partial<Card>) =>
  client
    .put<{ data: Card }>(`/admin/products/${productId}/cards/${cardId}`, patch)
    .then((r) => r.data.data)

export const deleteCard = (productId: number, cardId: number) =>
  client.delete(`/admin/products/${productId}/cards/${cardId}`)
