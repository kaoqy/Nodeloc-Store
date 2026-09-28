import client from './client'
import type { Card, Product } from '../types'

export interface CardQuery {
  productId?: number
  status?: string
  q?: string
  limit?: number
  offset?: number
}

export interface GeneratedCards {
  created: Card[]
  skipped: number
  blank: number
}

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
  client
    .get<{ data: Card[] }>(`/admin/products/${productId}/cards`)
    .then((r) => r.data.data ?? [])

// listAllCards answers the inventory screen without a product chosen: one
// server-side page of the shop's whole key stock, filtered and searched there
// rather than in the browser.
export const listAllCards = (query: CardQuery = {}) =>
  client
    .get<{ data: Card[]; total: number }>('/admin/cards', {
      params: {
        product_id: query.productId || undefined,
        status: query.status && query.status !== 'all' ? query.status : undefined,
        q: query.q?.trim() || undefined,
        limit: query.limit,
        offset: query.offset || undefined,
      },
    })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const addCard = (productId: number, content: string, status = 'available') =>
  client.post<{ data: Card }>(`/admin/products/${productId}/cards`, { content, status }).then((r) => r.data.data)

export const batchAddCards = (productId: number, contents: string[]) =>
  client
    .post<GeneratedCards>(`/admin/products/${productId}/cards/batch-add`, { cards: contents })
    .then((r) => r.data)

// generateCards lets the shop mint keys instead of pasting them; the server
// returns what it created so the screen can show the new rows.
export const generateCards = (productId: number, count: number, prefix = '') =>
  client
    .post<GeneratedCards>(`/admin/products/${productId}/cards/generate`, { count, prefix })
    .then((r) => r.data)

export const setCardStatusBatch = (productId: number, cardIds: number[], status: string) =>
  client
    .post<{ updated: number }>(`/admin/products/${productId}/cards/batch-status`, { card_ids: cardIds, status })
    .then((r) => r.data)

export const deleteCardsBatch = (productId: number, cardIds: number[]) =>
  client
    .post<{ deleted: number }>(`/admin/products/${productId}/cards/batch-delete`, { card_ids: cardIds })
    .then((r) => r.data)

// exportCards pulls the CSV as a blob because the endpoint sits behind the
// Authorization header — a plain download link would be rejected.
export const exportCards = async (query: CardQuery = {}) => {
  const { data } = await client.get<Blob>('/admin/cards/export', {
    responseType: 'blob',
    params: {
      product_id: query.productId || undefined,
      status: query.status && query.status !== 'all' ? query.status : undefined,
      q: query.q?.trim() || undefined,
    },
  })
  const url = URL.createObjectURL(data)
  const link = document.createElement('a')
  link.href = url
  link.download = `cards-${query.productId || 'all'}-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export const updateCard = (productId: number, cardId: number, patch: Partial<Card>) =>
  client
    .put<{ data: Card }>(`/admin/products/${productId}/cards/${cardId}`, patch)
    .then((r) => r.data.data)

export const deleteCard = (productId: number, cardId: number) =>
  client.delete(`/admin/products/${productId}/cards/${cardId}`)

export const listLowStock = () =>
  client
    .get<{ data: Product[]; threshold: number }>('/admin/low-stock')
    .then((r) => ({ data: r.data.data ?? [], threshold: r.data.threshold ?? 0 }))
