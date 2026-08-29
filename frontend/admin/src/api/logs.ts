import client from './client'
import type { AuditLog, PaginationParams } from '../types'
export const listAuditLogs=(params?:PaginationParams)=>client.get<{items:AuditLog[],total:number}>('/admin/audit-logs',{params}).then(r=>r.data)
