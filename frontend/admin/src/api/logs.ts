import client from './client'
import type { AuditLog } from '../types'

export interface AuditLogPage {
  items: AuditLog[]
  total: number
  page: number
  limit: number
  total_pages: number
}

export async function listAuditLogs(params: { page?: number; limit?: number; action?: string } = {}): Promise<AuditLogPage> {
  const { data } = await client.get<AuditLogPage>('/admin/audit-logs', { params })
  return { items: data.items ?? [], total: Number(data.total ?? 0), page: data.page, limit: data.limit, total_pages: data.total_pages }
}
