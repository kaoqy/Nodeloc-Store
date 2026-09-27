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

/**
 * 查单失败时，服务端除了给管理员看的原因（NodeLoc 的原话）还带一个 code，
 * 这里把两者合起来：先说要做什么，再附上服务商自己的说法。
 */
const RECONCILE_HINT: Record<string, string> = {
  no_transaction: '商店没有这单的 NodeLoc 交易号，无法代为查询：让买家重新发起支付，或核对下单是否真的到达 NodeLoc。',
  unsettled: 'NodeLoc 还没有这笔付款的到账记录，买家可能并未付款；确认已付后再查一次。',
  amount_mismatch: 'NodeLoc 记录的金额与本单不一致，商店已拒绝自动入账，请人工核对后处理。',
  foreign_transaction: 'NodeLoc 把这笔交易归属到了别的订单，商店已拒绝自动入账，请人工核对。',
  provider_rejected: 'NodeLoc 拒绝了这次请求，通常是 Payment ID / Token / Secret Key 与后台填写的不一致，请到 设置 用「测试支付网关」复核。',
  provider_unreachable: '暂时联系不上 NodeLoc，可能是服务商或出口网络问题，稍后重查。',
  not_configured: '商店的支付还没配置完整（Payment ID / Token / Secret Key 三项），请先到 设置 补齐。',
  not_found: '订单或支付记录已不存在，无法查询。',
}

const PROVIDER_STATUS: Record<string, string> = {
  pending: '处理中',
  failed: '失败',
  cancelled: '已取消',
  refunded: '已退款',
  expired: '已超时',
  closed: '已关闭',
}

/** providerStatus names what NodeLoc recorded for a payment, in Chinese. */
export function providerStatus(status?: string): string {
  if (!status) return '未知状态'
  return PROVIDER_STATUS[status] || status
}

export function errorCode(error: unknown): string {
  const data = (error as { response?: { data?: { code?: string } } })?.response?.data
  return data?.code ?? ''
}

export function reconcileMessage(error: unknown): string {
  const detail = errorMessage(error, '')
  const hint = RECONCILE_HINT[errorCode(error)]
  if (!hint) return detail || '无法向 NodeLoc 查询这单，请稍后重试，或到 设置 核对 Payment ID / Secret Key。'
  return detail ? `${hint}（NodeLoc：${detail}）` : hint
}
