import client from './client'

// ── 通知模板 ─────────────────────────────────────────────────────────

export interface NotificationTemplate {
  id?: number
  key: string
  name: string
  event: string
  category: string
  is_enabled: boolean
  in_app: boolean
  mail: boolean
  title_template: string
  content_template: string
  variables?: string
  recipients?: string
  retry_limit: number
  sort_order: number
}

// ── 系统配置 ─────────────────────────────────────────────────────────

export interface SystemConfig {
  id?: number
  group: string
  key: string
  value: string
  value_type: string
  label?: string
  description?: string
  is_secret: boolean
  sort_order: number
}

export const listTemplates = (category = '') =>
  client
    .get<{ data: NotificationTemplate[] }>('/admin/config-center/templates', { params: { category: category || undefined } })
    .then((r) => r.data.data ?? [])

export const saveTemplate = (template: Partial<NotificationTemplate>) =>
  template.id
    ? client.put('/admin/config-center/templates/' + template.id, template).then((r) => r.data.data)
    : client.post('/admin/config-center/templates', template).then((r) => r.data.data)

export const deleteTemplate = (id: number) => client.delete('/admin/config-center/templates/' + id)

export interface NotificationLogRow {
  id: number
  template_key: string
  channel: string
  user_id?: number
  ticket_id?: number
  order_id?: number
  title?: string
  content?: string
  status: string
  error?: string
  attempts: number
  created_at?: string
}

export const listNotificationLogs = (params: { status?: string; limit?: number; offset?: number } = {}) =>
  client
    .get<{ data: NotificationLogRow[]; total: number }>('/admin/config-center/logs', { params })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const listSystemConfigs = (group = '') =>
  client
    .get<{ data: SystemConfig[] }>('/admin/config-center/system', { params: { group: group || undefined } })
    .then((r) => r.data.data ?? [])

export const saveSystemConfig = (config: Partial<SystemConfig>) =>
  client.put('/admin/config-center/system', config).then((r) => r.data.data)

// ── 总览卡片偏好（本地保存，不依赖后端）────────────────────────────

export interface DashboardCardPref {
  key: string
  label: string
  visible: boolean
}

const CARD_KEY = 'admin.dashboard.cards'

export const defaultDashboardCards: DashboardCardPref[] = [
  { key: 'revenue', label: '今日销售额', visible: true },
  { key: 'orders', label: '今日订单数', visible: true },
  { key: 'tickets_pending', label: '待处理工单', visible: true },
  { key: 'tickets_ai', label: 'AI 处理中工单', visible: true },
  { key: 'activities', label: '活动进行中的数量', visible: true },
  { key: 'activity_users', label: '活动参与人数', visible: true },
  { key: 'stock', label: '商品库存预警', visible: true },
  { key: 'delivery_failed', label: '自动发货异常', visible: true },
  { key: 'ai_calls', label: 'AI 调用次数', visible: true },
  { key: 'ai_transfers', label: 'AI 转人工数量', visible: true },
  { key: 'resolve_rate', label: '工单解决率', visible: true },
  { key: 'satisfaction', label: '用户满意度', visible: true },
  { key: 'avg_minutes', label: '平均处理时长', visible: true },
]

export function readDashboardCards(): DashboardCardPref[] {
  try {
    const raw = localStorage.getItem(CARD_KEY)
    if (!raw) return defaultDashboardCards.map((item) => ({ ...item }))
    const saved = JSON.parse(raw) as DashboardCardPref[]
    // 保留新版本新增的卡片：以默认清单为准，按已保存的可见性与顺序合并。
    const byKey = new Map(saved.map((item) => [item.key, item]))
    const merged = defaultDashboardCards.map((item) => {
      const found = byKey.get(item.key)
      return found ? { ...item, visible: found.visible } : { ...item }
    })
    return merged
  } catch {
    return defaultDashboardCards.map((item) => ({ ...item }))
  }
}

export function writeDashboardCards(cards: DashboardCardPref[]) {
  localStorage.setItem(CARD_KEY, JSON.stringify(cards))
}
