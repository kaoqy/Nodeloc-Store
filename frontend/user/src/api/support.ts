import client from './client'

// ── AI 客服 ──────────────────────────────────────────────────────────

export interface SupportConfig {
  enabled: boolean
  agent_name: string
  avatar?: string
  greeting?: string
  guest_allowed: boolean
  rating_enabled: boolean
  working_hours?: string
  estimate_minutes: number
}

export interface QuickQuestion {
  id: number
  title: string
  content?: string
  position: string
  require_login: boolean
}

export interface ToolCallResult {
  tool_key: string
  tool_name?: string
  status: string
  data?: unknown
  summary?: string
  error?: string
  duration_ms: number
  require_confirm: boolean
  risk_level: string
}

export interface ChatReply {
  conversation_id: number
  message_id: number
  content: string
  tool_calls?: ToolCallResult[]
  suggest_transfer: boolean
  suggest_ticket: boolean
  need_confirm?: ToolCallResult
  ticket_id?: number
  ticket_no?: string
  knowledge_hits?: { id: number; title: string; summary?: string; content: string }[]
  fallback: boolean
}

export const getSupportConfig = () =>
  client.get<SupportConfig>('/support/config').then((r) => r.data)

export const getQuickQuestions = (position = 'widget') =>
  client
    .get<{ data: QuickQuestion[] }>('/support/quick-questions', { params: { position } })
    .then((r) => r.data.data ?? [])

export const sendChat = (payload: {
  content: string
  conversation_id?: number
  ticket_id?: number
  page_context?: string
}) => client.post<ChatReply>('/support/chat', payload).then((r) => r.data)

export const sendFeedback = (payload: {
  conversation_id: number
  message_id?: number
  ticket_id?: number
  rating: number
  reason?: string
  comment?: string
}) => client.post('/support/feedback', payload)

// ── 我的工单 ─────────────────────────────────────────────────────────

export interface Ticket {
  id: number
  ticket_no: string
  subject: string
  content?: string
  type: string
  status: string
  priority: string
  handler: string
  ai_enabled: boolean
  summary?: string
  suggested_plan?: string
  transfer_reason?: string
  order_no?: string
  product_name?: string
  satisfaction: number
  satisfaction_note?: string
  message_count: number
  unread_for_user: boolean
  created_at?: string
  updated_at?: string
  resolved_at?: string | null
}

export interface TicketMessage {
  id: number
  ticket_id: number
  sender_type: string
  sender_name?: string
  content: string
  content_type: string
  is_internal: boolean
  created_at?: string
}

export interface TicketToolCall {
  id: number
  tool_key: string
  tool_name?: string
  status: string
  duration_ms: number
  created_at?: string
}

export interface TicketDetail {
  ticket: Ticket
  messages: TicketMessage[]
  tool_calls: TicketToolCall[]
}

export const ticketStatusLabels: Record<string, string> = {
  ai_processing: 'AI 处理中',
  waiting_user: '等待你回复',
  ai_solved: 'AI 已解决',
  user_requested_human: '已申请人工',
  pending_human: '等待人工处理',
  human_handling: '人工处理中',
  waiting_confirm: '等待你确认',
  resolved: '已解决',
  closed: '已关闭',
  rejected: '已拒绝',
  cancelled: '已撤销',
}

export const ticketTypeOptions = [
  { value: 'order', label: '订单问题' },
  { value: 'payment', label: '支付问题' },
  { value: 'card', label: '卡密问题' },
  { value: 'refund', label: '退款售后' },
  { value: 'account', label: '账号问题' },
  { value: 'activity', label: '活动咨询' },
  { value: 'other', label: '其他' },
]

export const listMyTickets = (params: { status?: string; q?: string; limit?: number; offset?: number } = {}) =>
  client
    .get<{ data: Ticket[]; total: number }>('/me/tickets', { params })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const createTicket = (payload: {
  type: string
  subject: string
  content?: string
  order_no?: string
  priority?: string
  contact?: string
}) => client.post<{ data: Ticket }>('/me/tickets', payload).then((r) => r.data.data)

export const getTicket = (id: number) =>
  client.get<{ data: TicketDetail }>('/me/tickets/' + id).then((r) => r.data.data)

export const replyTicket = (id: number, content: string) =>
  client
    .post<{ data: TicketMessage; suggest_transfer: boolean }>('/me/tickets/' + id + '/messages', { content })
    .then((r) => r.data)

export const transferTicket = (id: number, reason = '') =>
  client.post('/me/tickets/' + id + '/transfer', { reason }).then((r) => r.data.data)

export const rateTicket = (id: number, satisfaction: number, comment = '') =>
  client.post('/me/tickets/' + id + '/rate', { satisfaction, comment })

export const reopenTicket = (id: number) => client.post('/me/tickets/' + id + '/reopen')

export const cancelTicket = (id: number) => client.post('/me/tickets/' + id + '/cancel')

// ── 活动中心 ─────────────────────────────────────────────────────────

export interface Activity {
  id: number
  name: string
  subtitle?: string
  description?: string
  cover_image?: string
  banner_image?: string
  type: string
  rules?: string
  start_at?: string | null
  end_at?: string | null
  status: string
  rule_list?: { rule_type: string; config: string }[]
}

export const listActivities = (type = '') =>
  client
    .get<{ data: Activity[] }>('/store/activities', { params: { type: type || undefined } })
    .then((r) => r.data.data ?? [])

export const getActivity = (id: number) =>
  client.get<{ data: Activity }>('/store/activities/' + id).then((r) => r.data.data)

export const listMyActivityRecords = (limit = 20, offset = 0) =>
  client
    .get<{ data: unknown[]; total: number }>('/me/activity-records', { params: { limit, offset } })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const claimCoupon = (activityID: number) =>
  client.post('/activities/' + activityID + '/claim').then((r) => r.data.data)
