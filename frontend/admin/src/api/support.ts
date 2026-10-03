import client from './client'

// ── 工单 ─────────────────────────────────────────────────────────────

export interface Ticket {
  id: number
  ticket_no: string
  user_id: number
  order_id?: number
  order_no?: string
  product_id?: number
  product_name?: string
  type: string
  type_name?: string
  subject: string
  content?: string
  status: string
  priority: string
  handler: string
  assigned_agent_id?: number
  transfer_reason?: string
  summary?: string
  suggested_plan?: string
  tags?: string
  source?: string
  customer_contact?: string
  satisfaction: number
  satisfaction_note?: string
  last_message_at?: string | null
  first_response_at?: string | null
  resolved_at?: string | null
  closed_at?: string | null
  due_at?: string | null
  message_count: number
  attachment_count: number
  unread_for_staff: boolean
  unread_for_user: boolean
  created_at?: string
  updated_at?: string
}

export interface TicketView extends Ticket {
  username?: string
  agent_name?: string
  unread?: boolean
  overdue?: boolean
}

export interface TicketMessage {
  id: number
  ticket_id: number
  sender_type: string
  sender_id?: number
  sender_name?: string
  content: string
  content_type: string
  is_internal: boolean
  meta?: string
  created_at?: string
}

export interface TicketLog {
  id: number
  ticket_id: number
  actor_id?: number
  actor_type: string
  action: string
  before?: string
  after?: string
  detail?: string
  result: string
  created_at?: string
}

export interface TicketAssignment {
  id: number
  ticket_id: number
  agent_id: number
  strategy: string
  status: string
  assigned_at: string
}

export interface TicketDetail {
  ticket: Ticket
  messages: TicketMessage[]
  logs: TicketLog[]
  assignments?: TicketAssignment[]
  quick_replies?: QuickReply[]
  related?: Ticket[]
  username?: string
  agent_name?: string
}

export interface TicketStats {
  total: number
  pending_human: number
  human_handling: number
  resolved: number
  unread: number
  overdue: number
  urgent: number
  today_created: number
  satisfaction_avg: number
  resolve_rate: number
  avg_handle_minutes: number
}

export interface TicketListQuery {
  status?: string
  handler?: string
  priority?: string
  type?: string
  q?: string
  attention?: string
  mine?: boolean
  limit?: number
  offset?: number
}

// ai_processing / ai_solved 是早期版本遗留的工单状态码。两个键名不能改：
// 它们要和库里历史工单的 status 字段对齐，这里只是把它们翻译成现在的中文文案。
export const ticketStatusLabels: Record<string, string> = {
  ai_processing: '处理中',
  waiting_user: '等待用户回复',
  ai_solved: '已结束',
  user_requested_human: '用户申请人工',
  pending_human: '待人工处理',
  human_handling: '人工处理中',
  waiting_confirm: '等待用户确认',
  resolved: '已解决',
  closed: '已关闭',
  rejected: '已拒绝',
  cancelled: '已撤销',
}

export const ticketPriorityLabels: Record<string, string> = {
  low: '低',
  normal: '普通',
  high: '高',
  urgent: '紧急',
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

export const listTickets = (query: TicketListQuery = {}) =>
  client
    .get<{ data: TicketView[]; total: number }>('/admin/tickets', {
      params: {
        status: query.status && query.status !== 'all' ? query.status : undefined,
        handler: query.handler && query.handler !== 'all' ? query.handler : undefined,
        priority: query.priority && query.priority !== 'all' ? query.priority : undefined,
        type: query.type && query.type !== 'all' ? query.type : undefined,
        q: query.q?.trim() || undefined,
        attention: query.attention || undefined,
        mine: query.mine ? '1' : undefined,
        limit: query.limit,
        offset: query.offset || undefined,
      },
    })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const getTicketStats = (mine = false) =>
  client
    .get<{ data: TicketStats }>('/admin/ticket-stats', { params: { mine: mine ? '1' : undefined } })
    .then((r) => r.data.data)

export const getTicket = (id: number) =>
  client.get<{ data: TicketDetail }>('/admin/tickets/' + id).then((r) => r.data.data)

export const replyTicket = (id: number, content: string, internal = false) =>
  client
    .post<{ data: TicketMessage }>('/admin/tickets/' + id + '/messages', { content, internal })
    .then((r) => r.data.data)

export const setTicketStatus = (id: number, status: string, detail = '') =>
  client
    .post<{ data: Ticket }>('/admin/tickets/' + id + '/status', { status, detail })
    .then((r) => r.data.data)

export const assignTicket = (id: number, agentId: number) =>
  client.post<{ data: Ticket }>('/admin/tickets/' + id + '/assign', { agent_id: agentId }).then((r) => r.data.data)

export const claimTicket = (id: number) =>
  client.post<{ data: Ticket }>('/admin/tickets/' + id + '/claim').then((r) => r.data.data)

export const autoAssignTicket = (id: number) =>
  client.post<{ data: Ticket }>('/admin/tickets/' + id + '/auto-assign').then((r) => r.data.data)

export const markTicketRead = (id: number) => client.post('/admin/tickets/' + id + '/read')

export const getTicketSummary = (id: number) =>
  client
    .get<{ summary: string; suggested_plan: string }>('/admin/tickets/' + id + '/summary')
    .then((r) => r.data)

// ── 客服坐席与快捷回复 ────────────────────────────────────────────────

export interface CustomerServiceAgent {
  id?: number
  user_id: number
  nickname?: string
  avatar?: string
  status: string
  accept_manual: boolean
  ticket_types?: string
  max_concurrent: number
  work_start?: string
  work_end?: string
  assign_strategy: string
  role: string
  score: number
  avg_response_sec: number
  handled_count: number
  is_active: boolean
}

export interface QuickReply {
  id?: number
  title: string
  content: string
  ticket_types?: string
  scene?: string
  roles?: string
  sort_order: number
  is_enabled: boolean
  variables?: string
  use_count?: number
}

export const listAgents = () =>
  client
    .get<{ data: { agent: CustomerServiceAgent; current_load: number }[] }>('/admin/agents')
    .then((r) => r.data.data ?? [])

export const saveAgent = (agent: Partial<CustomerServiceAgent>) =>
  agent.id
    ? client.put('/admin/agents/' + agent.id, agent).then((r) => r.data.agent as CustomerServiceAgent)
    : client.post('/admin/agents', agent).then((r) => r.data.agent as CustomerServiceAgent)

export const deleteAgent = (id: number) => client.delete('/admin/agents/' + id)

export const listQuickReplies = () =>
  client.get<{ data: QuickReply[] }>('/admin/quick-replies').then((r) => r.data.data ?? [])

export const saveQuickReply = (reply: Partial<QuickReply>) =>
  reply.id
    ? client.put('/admin/quick-replies/' + reply.id, reply).then((r) => r.data.data as QuickReply)
    : client.post('/admin/quick-replies', reply).then((r) => r.data.data as QuickReply)

export const deleteQuickReply = (id: number) => client.delete('/admin/quick-replies/' + id)

export const renderQuickReply = (id: number, variables: Record<string, string>) =>
  client.post<{ content: string }>('/admin/quick-replies/' + id + '/render', { variables }).then((r) => r.data.content)
