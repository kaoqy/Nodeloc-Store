import client from './client'
import type { AuditLog } from '../types'

export interface AuditLogPage {
  items: AuditLog[]
  total: number
  page: number
  limit: number
  total_pages: number
}

/**
 * Filters the log page understands. `actor` is either a user id or the word
 * `system`, and `since`/`until` are `YYYY-MM-DD` days with the end day still
 * included.
 */
export interface AuditLogQuery {
  page?: number
  limit?: number
  action?: string
  search?: string
  actor?: string
  since?: string
  until?: string
}

export async function listAuditLogs(params: AuditLogQuery = {}): Promise<AuditLogPage> {
  const { data } = await client.get<AuditLogPage>('/admin/audit-logs', { params })
  return { items: data.items ?? [], total: Number(data.total ?? 0), page: data.page, limit: data.limit, total_pages: data.total_pages }
}

/** The actions this shop has already recorded, for the filter's suggestions. */
export async function listAuditActions(): Promise<string[]> {
  const { data } = await client.get<{ items?: string[] }>('/admin/audit-logs/actions')
  return data.items ?? []
}
