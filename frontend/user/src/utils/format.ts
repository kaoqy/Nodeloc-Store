// This file is kept in step with frontend/admin/src/utils/format.ts: buyer and
// shopkeeper must not disagree about what the same number or the same status is
// called. A price is a number of NodeLoc points (NL) — the checkout has NodeLoc
// deduct that many from the buyer's forum account, so a 「¥」 in front of it
// named a currency that never moves. The in-shop 积分 from checking in is a
// different balance and keeps its own word.
const amount = new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 2 })

export const PRICE_UNIT = 'NL'

const dateTime = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })
const dayFormat = new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' })

/** money renders one of the shop's amounts, which are quoted and charged in NL. */
export function money(value?: number | string | null): string {
  return `${amount.format(Number(value || 0))} ${PRICE_UNIT}`
}

export function when(value?: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  // A zero time is stored as year 1 and would read as 「1年1月1日」; unparseable
  // text is not a date either. Neither is worth showing a buyer.
  if (Number.isNaN(date.getTime()) || date.getFullYear() < 1970) return '—'
  return dateTime.format(date)
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
  unsettled: {
    label: 'NodeLoc 回报本单尚未到账。商店会自动继续核实，已扣款请稍候，不必重复付款。',
    badge: 'alert-warning',
  },
  unreachable: {
    label: '暂时联系不上 NodeLoc 的支付服务，商店会自动重试核实，请稍候，不必重复付款。',
    badge: 'alert-warning',
  },
  rejected: {
    label: 'NodeLoc 拒绝了商店的支付请求，本单暂未入账。这通常是店家的支付配置问题，请联系处理。',
    badge: 'alert-danger',
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

// normalizeStatus() on the server collapses NodeLoc's spellings onto a small
// set; whatever still slips through is shown as evidence inside a Chinese
// sentence rather than as a bare English badge on the order page.
const PROVIDER_STATUS: Record<string, string> = {
  pending: '处理中',
  succeeded: '已到账',
  failed: '失败',
  cancelled: '已取消',
  refunded: '已退款',
  expired: '已超时',
  closed: '已关闭',
}

/** providerStatus names what NodeLoc recorded for the payment, in Chinese. */
export function providerStatus(status?: string): string {
  if (!status) return '未知状态'
  const known = PROVIDER_STATUS[status]
  if (known) return known
  return `未知道账状态（${status}）`
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
