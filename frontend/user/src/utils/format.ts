const currency = new Intl.NumberFormat('zh-CN', {
  style: 'currency',
  currency: 'CNY',
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
})

const dateTime = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })

/** Points are the store's unit of value, but they are priced and shown as money. */
export function money(value: number): string {
  return currency.format(value)
}

export function when(value?: string | null): string {
  return value ? dateTime.format(new Date(value)) : '—'
}

export interface StatusMeta {
  label: string
  badge: string
}

const ORDER_STATUS: Record<string, StatusMeta> = {
  pending: { label: '待支付', badge: 'badge-warning' },
  paid: { label: '已支付', badge: 'badge-info' },
  completed: { label: '已完成', badge: 'badge-success' },
  cancelled: { label: '已取消', badge: 'badge-neutral' },
  refunded: { label: '已退款', badge: 'badge-danger' },
  failed: { label: '交易失败', badge: 'badge-danger' },
}

const FULFILLMENT_STATUS: Record<string, StatusMeta> = {
  pending: { label: '待发货', badge: 'badge-neutral' },
  delivered: { label: '已发货', badge: 'badge-teal' },
  completed: { label: '已交付', badge: 'badge-success' },
  manual_pending: { label: '等待人工发货', badge: 'badge-warning' },
  waiting_stock: { label: '等待补货', badge: 'badge-warning' },
  cancelled: { label: '已取消发货', badge: 'badge-neutral' },
}

export function orderStatus(status: string): StatusMeta {
  return ORDER_STATUS[status] || { label: status || '未知', badge: 'badge-neutral' }
}

export function fulfillmentStatus(status: string): StatusMeta {
  return FULFILLMENT_STATUS[status] || { label: status || '未知', badge: 'badge-neutral' }
}
