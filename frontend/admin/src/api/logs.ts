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

// 导出走 blob：这个接口在 Authorization 头后面，普通的下载链接会被当成未登录。
// 筛选参数与列表同一套，所以下下来的是屏幕上这一批日志，而不是全部历史。
export type AuditLogExportQuery = Omit<AuditLogQuery, 'page' | 'limit'>

export async function exportAuditLogs(query: AuditLogExportQuery = {}): Promise<{ rows: number; truncated: boolean }> {
  const response = await client.get<Blob>('/admin/audit-logs/export', {
    responseType: 'blob',
    params: {
      action: query.action?.trim() || undefined,
      search: query.search?.trim() || undefined,
      actor: query.actor || undefined,
      since: query.since || undefined,
      until: query.until || undefined,
    },
  })
  const url = URL.createObjectURL(response.data)
  const link = document.createElement('a')
  link.href = url
  link.download = `audit-logs-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
  // 行数与「有没有被截断」由服务端在响应头里报数，不靠前端数行 —— 详情里换一个
  // 回车就会把行数数错。
  return {
    rows: Number(response.headers['x-export-rows'] ?? 0),
    truncated: response.headers['x-export-truncated'] === '1',
  }
}
