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

export function fulfillmentStatus(status: string, paymentStatus?: string): StatusMeta {
  // Cancelling, payment failure and refunding leave fulfillment_status at a
  // waiting state, which would read as delivery still coming. Orders that
  // already shipped keep showing what happened to them.
  const shipped = status === 'delivered' || status === 'completed'
  if (paymentStatus === 'cancelled' || paymentStatus === 'failed' || (paymentStatus === 'refunded' && !shipped)) {
    return { label: '无需发货', badge: 'badge-neutral' }
  }
  return FULFILLMENT_STATUS[status] || { label: status || '未知', badge: 'badge-neutral' }
}

const PAYMENT_NOTICE: Record<string, StatusMeta> = {
  ok: { label: '支付已确认，正在为你交付。', badge: 'alert-success' },
  signature: {
    label: '支付结果校验失败：回调签名与商户密钥不一致，本单暂未入账。请稍后重新支付，或联系店家核对后台设置。',
    badge: 'alert-danger',
  },
  amount: {
    label: '支付金额与订单不一致，本单暂未入账。请重新支付，或联系店家核实。',
    badge: 'alert-danger',
  },
  pending: {
    label: '支付渠道回报本单尚未完成。若已扣款请稍候刷新，仍未到账请联系店家处理。',
    badge: 'alert-warning',
  },
  unknown_order: {
    label: '没有找到对应的订单，本单暂未入账。请重新下单支付，或联系店家核实。',
    badge: 'alert-warning',
  },
  error: {
    label: '支付结果处理失败，本单暂未入账。请稍后重试或联系店家。',
    badge: 'alert-danger',
  },
}

/** paymentNotice explains why the provider redirect came back unpaid. */
export function paymentNotice(code?: string | null): StatusMeta | null {
  if (!code) return null
  return PAYMENT_NOTICE[code] || PAYMENT_NOTICE.error
}
