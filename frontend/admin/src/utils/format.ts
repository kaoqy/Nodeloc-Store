// Two balances live in this shop and they must not borrow each other's word.
// A price is a number of NodeLoc points (NL) — the checkout asks NodeLoc to
// deduct exactly this many from the buyer's forum account, so printing 「¥99」
// showed a currency that never moves. 商店积分 (User.Points, the check-in
// ledger) is a separate in-shop balance and stays 积分.
const amount = new Intl.NumberFormat('zh-CN', { minimumFractionDigits: 0, maximumFractionDigits: 2 })

export const PRICE_UNIT = 'NL'

const dateTime = new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' })
const dayFormat = new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' })

export function money(value?: number | string | null): string {
  return `${amount.format(Number(value || 0))} ${PRICE_UNIT}`
}

export function when(value?: string | null): string {
  if (!value) return '—'
  const date = new Date(value)
  // A zero time is stored as year 1 and would otherwise read as 「1年1月1日」,
  // which is a date nobody asked for. Unparseable text is not a date either.
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
  // 插件交付: the goods are delivered by an installed plugin, not by the shop's
  // own card/manual queue, so the buyer is told which one is running.
  plugin_pending: { label: '插件交付中', badge: 'badge-info' },
  plugin_review: { label: '待人工确认', badge: 'badge-warning' },
  cancelled: { label: '已取消发货', badge: 'badge-neutral' },
}

const CARD_STATUS: Record<string, StatusMeta> = {
  available: { label: '可用', badge: 'badge-success' },
  sold: { label: '已售出', badge: 'badge-info' },
  disabled: { label: '已停用', badge: 'badge-neutral' },
}

const TRANSFER_STATUS: Record<string, StatusMeta> = {
  succeeded: { label: '已到账', badge: 'badge-success' },
  pending: { label: '处理中', badge: 'badge-warning' },
  failed: { label: '未成功', badge: 'badge-danger' },
}

/**
 * transferStatus labels one 转账 row. A refusal is kept in the ledger, so it needs
 * a reading of its own: 「未成功」 is not the same statement as 「没有发生过」.
 */
export function transferStatus(status?: string | null): StatusMeta {
  if (!status) return { label: '—', badge: 'badge-neutral' }
  return TRANSFER_STATUS[status] || { label: status, badge: 'badge-neutral' }
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

const ROLE: Record<string, StatusMeta> = {
  user: { label: '普通用户', badge: 'badge-neutral' },
  support: { label: '客服', badge: 'badge-info' },
  operator: { label: '运营', badge: 'badge-teal' },
  admin: { label: '管理员', badge: 'badge-warning' },
  super_admin: { label: '超级管理员', badge: 'badge-danger' },
}

/**
 * roleMeta names an account role. The labels match the roles the permission
 * editor writes, so a user list and a policy matrix never disagree about what
 * 运营 means.
 */
export function roleMeta(role?: string | null): StatusMeta {
  return ROLE[role || 'user'] || { label: role || '普通用户', badge: 'badge-neutral' }
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
  payment_id_unknown:
    'NodeLoc 在这个地址上没有后台填的 Payment ID：请到 设置 复核「Payment ID」（pay_ 开头，不是 OAuth 的 Client ID）与「支付 API 地址」是否指向挂着该支付应用的域名。',
  provider_guarded:
    'NodeLoc 的查单接口只接受论坛后台的浏览器会话，商店的服务器调不动它。这一单改用下单回执与支付回调核实，收款与发货本身不受影响。',
  provider_clock:
    '商店服务器的时间与 NodeLoc 相差过大，下单/查单都会被拒。请在宿主机同步时钟（NTP），支付凭据没有问题。',
  not_found: '订单或支付记录已不存在，无法查询。',
}

// What the gateway records for a payment is a small closed set on purpose:
// normalizeStatus() collapses NodeLoc's own spellings onto these, and an
// unnormalised word can still arrive (the gateway passes an unknown status
// through). Both cases need Chinese on screen — a badge reading 「succeeded」 in
// an otherwise Chinese order list is the bug, so an unrecognised word is kept
// as the evidence and framed in Chinese rather than shown bare.
const PROVIDER_STATUS: Record<string, string> = {
  pending: '处理中',
  succeeded: '已到账',
  failed: '失败',
  cancelled: '已取消',
  refunded: '已退款',
  expired: '已超时',
  closed: '已关闭',
}

/** providerStatus names what NodeLoc recorded for a payment, in Chinese. */
export function providerStatus(status?: string): string {
  if (!status) return '未知状态'
  const known = PROVIDER_STATUS[status]
  if (known) return known
  return `未知道账状态（${status}）`
}

// The inbox stores the kind as the sender named it. These are the kinds this
// shop writes itself; anything else (a 自定义 type from the broadcast form) stays
// as written, because inventing a label for it would hide what was sent.
const NOTIFICATION_KIND: Record<string, string> = {
  order: '订单',
  system: '系统',
  promo: '促销',
  announcement: '公告',
  stock: '库存预警',
  transfer: '店家转账',
}

/** notificationKind labels one kind of inbox message for 通知中心. */
export function notificationKind(type: string): string {
  return NOTIFICATION_KIND[type] || type
}

// 设置 页的「最近 NodeLoc 登录记录」：买家只会说「登录不了」，而商店自己知道停在了
// 哪一步。词表要跟着 handler 写下的 code 走，认不出的原样保留，那才是证据。
const OAUTH_STEP: Record<string, string> = {
  initiate: '发起授权',
  callback: 'NodeLoc 回调',
}

const OAUTH_OUTCOME: Record<string, StatusMeta> = {
  started: { label: '已跳转到 NodeLoc', badge: 'badge-info' },
  success: { label: '登录成功', badge: 'badge-success' },
  failed: { label: '未成功', badge: 'badge-danger' },
}

const OAUTH_REASON: Record<string, string> = {
  disabled: 'OAuth 登录在设置里是关闭的',
  not_configured: 'OAuth 参数没填全（站点地址 / Client ID / Client Secret）',
  rejected: 'NodeLoc 不认这组 Client ID 与 Client Secret',
  unreachable: '联系不上 NodeLoc，可能是服务商或出口网络问题',
  provider: 'NodeLoc 的回答无法解析',
  denied: 'NodeLoc 拒绝了这次授权（买家点了拒绝，或应用未获批）',
  expired: '登录链接过期，或浏览器没能带回 state',
  state: '回调带回的 state 与本店发出的不是同一个',
  bind: '这一趟是把 NodeLoc 账号绑到当前登录的账号',
}

/** oauthStep names where a 登录 round trip was when the shop wrote the row. */
export function oauthStep(step?: string | null): string {
  if (!step) return '—'
  return OAUTH_STEP[step] || step
}

/** oauthOutcome labels what the shop did with this attempt. */
export function oauthOutcome(outcome?: string | null): StatusMeta {
  if (!outcome) return { label: '—', badge: 'badge-neutral' }
  return OAUTH_OUTCOME[outcome] || { label: outcome, badge: 'badge-neutral' }
}

/**
 * oauthReason explains a code in Chinese. The code travels to the storefront's
 * login page too, so it stays visible next to the sentence.
 */
export function oauthReason(reason?: string | null): string {
  if (!reason) return ''
  const known = OAUTH_REASON[reason]
  return known ? `${known}（${reason}）` : reason
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
