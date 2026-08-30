import client from './client'
import type { Card } from '../types'

export async function listCards(params: { page?: number; per_page?: number; q?: string } = {}) {
  const { data } = await client.get<{ data: Card[] }>('/admin/cards', { params })
  return data
}

export async function deleteCard(id: number) {
  await client.delete(`/admin/cards/${id}`)
}
