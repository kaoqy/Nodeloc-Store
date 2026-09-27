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
    label: '回调校验没通过，商店正在向 NodeLoc 核实这单的支付结果；确认已付会自动入账并发卡，无需重复付款。',
    badge: 'alert-warning',
  },
  amount: {
    label: '支付金额与订单不一致，本单暂未入账。请重新支付，或联系店家核实。',
    badge: 'alert-danger',
  },
  pending: {
    label: '支付渠道回报本单尚未完成。若已扣款请稍候，商店会自动查单确认，仍未到账请联系店家处理。',
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

const OAUTH_ERROR: Record<string, string> = {
  denied: 'NodeLoc 说这次授权被拒绝了：可能是你点了取消，或应用没有获得申请的权限。',
  expired: '登录链接已失效：发起登录后超过 10 分钟没完成，或浏览器没有把校验凭证带回来。',
  state: '回调与你发起的登录不是同一次，已拒绝写入登录态。',
  provider: 'NodeLoc 没有受理这次授权（code 换取 token 失败），通常是 Client ID/Secret 或重定向白名单不匹配。',
}

/**
 * oauthErrorText turns the reason the server put on ?oauth_error= into copy.
 * The server names the cause it actually saw, so this never guesses.
 */
export function oauthErrorText(reason?: string | null, action = '登录'): string {
  const detail = reason ? OAUTH_ERROR[reason] : ''
  if (!detail) return `NodeLoc ${action}未完成，可能是链接过期或授权被拒绝。你可以重试，或改用账号密码${action}。`
  return `${detail}你可以重新${action}，或改用账号密码${action}。`
}
