const currency = new Intl.NumberFormat('zh-CN', {
  style: 'currency',
  currency: 'CNY',
  minimumFractionDigits: 0,
  maximumFractionDigits: 2,
})

const dateTime = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })
const dayFormat = new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit' })

export function money(value?: number | string | null): string {
  return currency.format(Number(value || 0))
}

export function when(value?: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : dateTime.format(date)
}

export function dayLabel(value: string): string {
  const date = new Date(`${value}T00:00:00`)
  return Number.isNaN(date.getTime()) ? value : dayFormat.format(date)
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

const CARD_STATUS: Record<string, StatusMeta> = {
  available: { label: '可用', badge: 'badge-success' },
  sold: { label: '已售出', badge: 'badge-info' },
  disabled: { label: '已停用', badge: 'badge-neutral' },
}

export function orderStatus(status: string): StatusMeta {
  return ORDER_STATUS[status] || { label: status || '未知', badge: 'badge-neutral' }
}

export function fulfillmentStatus(status?: string | null, paymentStatus?: string): StatusMeta {
  // Cancelling, payment failure and refunding leave fulfillment_status at a
  // waiting state, which would read as delivery work still outstanding. Orders
  // that already shipped keep showing what happened to them.
  const shipped = status === 'delivered' || status === 'completed'
  if (paymentStatus === 'cancelled' || paymentStatus === 'failed' || (paymentStatus === 'refunded' && !shipped)) {
    return { label: '无需发货', badge: 'badge-neutral' }
  }
  if (!status) return { label: '—', badge: 'badge-neutral' }
  return FULFILLMENT_STATUS[status] || { label: status, badge: 'badge-neutral' }
}

export function cardStatus(status?: string | null): StatusMeta {
  if (!status) return { label: '—', badge: 'badge-neutral' }
  return CARD_STATUS[status] || { label: status, badge: 'badge-neutral' }
}

export function errorMessage(error: unknown, fallback = '请求失败，请稍后重试'): string {
  const response = (error as { response?: { data?: { error?: string } } })?.response
  if (response?.data?.error) return response.data.error
  // Locally raised errors (form and permission checks) carry their own text.
  if (error instanceof Error && !response) return error.message
  return fallback
}
