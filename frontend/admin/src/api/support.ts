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
  ai_enabled: boolean
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

export interface TicketToolCall {
  id: number
  tool_key: string
  tool_name?: string
  params?: string
  result?: string
  status: string
  error?: string
  duration_ms: number
  risk_level: string
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
  tool_calls: TicketToolCall[]
  assignments?: TicketAssignment[]
  quick_replies?: QuickReply[]
  related?: Ticket[]
  username?: string
  agent_name?: string
}

export interface TicketStats {
  total: number
  ai_processing: number
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

export const ticketStatusLabels: Record<string, string> = {
  ai_processing: 'AI 处理中',
  waiting_user: '等待用户回复',
  ai_solved: 'AI 已解决',
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

export const autoAssignTicket = (id: number) =>
  client.post<{ data: Ticket }>('/admin/tickets/' + id + '/auto-assign').then((r) => r.data.data)

export const markTicketRead = (id: number) => client.post('/admin/tickets/' + id + '/read')

export const getTicketSummary = (id: number) =>
  client
    .get<{ summary: string; suggested_plan: string }>('/admin/tickets/' + id + '/summary')
    .then((r) => r.data)

// ── AI 客服配置 ──────────────────────────────────────────────────────

export interface AIConfig {
  id?: number
  provider: string
  base_url: string
  model: string
  timeout_ms: number
  max_context: number
  max_reply_len: number
  temperature: number
  top_p: number
  is_enabled: boolean
  guest_allowed: boolean
  guest_daily_limit: number
  user_daily_limit: number
  ip_rate_limit: number
  max_message_len: number
  system_prompt: string
  greeting: string
  fallback_reply: string
  transfer_tip: string
  ticket_tip: string
  sensitive_tip: string
  avatar: string
  agent_name: string
  rating_enabled: boolean
  log_enabled: boolean
  mail_enabled: boolean
}

export interface AIWorkflow {
  id?: number
  default_handle_minutes: number
  max_failures: number
  transfer_after_failures: number
  transfer_after_downvotes: number
  transfer_on_explicit: boolean
  transfer_high_amount: boolean
  high_amount_threshold: number
  transfer_refund: boolean
  transfer_card_dispute: boolean
  transfer_payment_issue: boolean
  transfer_abuse: boolean
  can_create_ticket: boolean
  can_update_ticket: boolean
  can_notify: boolean
  can_query_order: boolean
  can_query_shipping: boolean
  can_recommend_activity: boolean
  can_grant_coupon: boolean
  can_refund: boolean
  require_human_refund: boolean
  transfer_notice: string
  working_hours: string
  estimate_reply_minutes: number
}

export interface AIToolPermission {
  id?: number
  tool_id: number
  role: string
  allowed: boolean
}

export interface AITool {
  id: number
  key: string
  name: string
  description?: string
  category: string
  method: string
  kind: string
  require_login: boolean
  own_data_only: boolean
  require_approval: boolean
  allow_auto: boolean
  require_confirm: boolean
  is_enabled: boolean
  rate_limit: number
  timeout_ms: number
  failure_mode: string
  risk_level: string
  builtin: boolean
  sort_order: number
}

export interface AIToolRow {
  tool: AITool
  permissions: AIToolPermission[]
}

export interface AIToolCall {
  id: number
  conversation_id?: number
  ticket_id?: number
  user_id?: number
  tool_key: string
  tool_name?: string
  params?: string
  result?: string
  status: string
  error?: string
  duration_ms: number
  risk_level: string
  created_at?: string
}

export interface AIQuickQuestion {
  id?: number
  title: string
  content?: string
  position: string
  pages?: string
  sort_order: number
  is_enabled: boolean
  require_login: boolean
  knowledge_id?: number
  tool_key?: string
}

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

export interface KnowledgeArticle {
  id?: number
  category_id: number
  title: string
  slug: string
  summary?: string
  content: string
  keywords?: string
  tags?: string
  priority: number
  status: string
  version?: number
  source?: string
  builtin?: boolean
  created_at?: string
}

export interface KnowledgeCategory {
  id?: number
  name: string
  slug: string
  description?: string
  sort_order: number
  is_enabled: boolean
}

export const getAIConfig = () =>
  client
    .get<{ config: AIConfig; workflow: AIWorkflow; has_key: boolean }>('/admin/ai/config')
    .then((r) => r.data)

export const saveAIConfig = (config: Partial<AIConfig> & { api_key?: string }) =>
  client.put('/admin/ai/config', config).then((r) => r.data)

export const saveAIWorkflow = (workflow: Partial<AIWorkflow>) =>
  client.put('/admin/ai/workflow', workflow).then((r) => r.data.workflow as AIWorkflow)

export const listAITools = () =>
  client.get<{ data: AIToolRow[] }>('/admin/ai-tool-index').then((r) => r.data.data ?? [])

export const saveAITool = (id: number, tool: Partial<AITool>) =>
  client.put<{ data: AITool }>('/admin/ai-tools/' + id, tool).then((r) => r.data.data)

export const setAIToolPermission = (toolId: number, role: string, allowed: boolean) =>
  client.post('/admin/ai-tools/' + toolId + '/permissions', { role, allowed })

export const listAIToolCalls = (params: { tool?: string; status?: string; limit?: number; offset?: number } = {}) =>
  client
    .get<{ data: AIToolCall[]; total: number }>('/admin/ai-tool-index/calls', { params })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const listAIRoles = () =>
  client.get<{ data: string[] }>('/admin/ai-tool-index/roles').then((r) => r.data.data ?? [])

export const listQuickQuestions = () =>
  client.get<{ data: AIQuickQuestion[] }>('/admin/ai/quick-questions').then((r) => r.data.data ?? [])

export const saveQuickQuestion = (question: Partial<AIQuickQuestion>) =>
  question.id
    ? client.put('/admin/ai/quick-questions/' + question.id, question).then((r) => r.data.data)
    : client.post('/admin/ai/quick-questions', question).then((r) => r.data.data)

export const deleteQuickQuestion = (id: number) => client.delete('/admin/ai/quick-questions/' + id)

export const listKnowledge = (params: { q?: string; status?: string; category_id?: number; limit?: number; offset?: number } = {}) =>
  client
    .get<{ data: KnowledgeArticle[]; total: number }>('/admin/knowledge-index', { params })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const getKnowledge = (id: number) =>
  client.get<{ data: KnowledgeArticle }>('/admin/knowledge/' + id).then((r) => r.data.data)

export const saveKnowledge = (article: Partial<KnowledgeArticle>) =>
  article.id
    ? client.put('/admin/knowledge/' + article.id, article).then((r) => r.data.data)
    : client.post('/admin/knowledge-index', article).then((r) => r.data.data)

export const deleteKnowledge = (id: number) => client.delete('/admin/knowledge/' + id)

export const listKnowledgeCategories = () =>
  client.get<{ data: KnowledgeCategory[] }>('/admin/knowledge-index/categories').then((r) => r.data.data ?? [])

export const saveKnowledgeCategory = (category: Partial<KnowledgeCategory>) =>
  client.post('/admin/knowledge-index/categories', category).then((r) => r.data.data)

export const deleteKnowledgeCategory = (id: number) =>
  client.delete('/admin/knowledge-index/categories/' + id)

export const importKnowledge = (text: string, categoryId = 0) =>
  client.post('/admin/knowledge-index/import', { text, category_id: categoryId }).then((r) => r.data)

export const testKnowledge = (id: number, question = '') =>
  client.post('/admin/knowledge/' + id + '/test', { question }).then((r) => r.data)

export const listAgents = () =>
  client.get<{ data: { agent: CustomerServiceAgent; current_load: number }[] }>('/admin/agents').then((r) => r.data.data ?? [])

export const saveAgent = (agent: Partial<CustomerServiceAgent>) =>
  agent.id
    ? client.put('/admin/agents/' + agent.id, agent).then((r) => r.data.data)
    : client.post('/admin/agents', agent).then((r) => r.data.data)

export const deleteAgent = (id: number) => client.delete('/admin/agents/' + id)

export const listQuickReplies = () =>
  client.get<{ data: QuickReply[] }>('/admin/quick-replies').then((r) => r.data.data ?? [])

export const saveQuickReply = (reply: Partial<QuickReply>) =>
  reply.id
    ? client.put('/admin/quick-replies/' + reply.id, reply).then((r) => r.data.data)
    : client.post('/admin/quick-replies', reply).then((r) => r.data.data)

export const deleteQuickReply = (id: number) => client.delete('/admin/quick-replies/' + id)

export const renderQuickReply = (id: number, variables: Record<string, string>) =>
  client.post<{ content: string }>('/admin/quick-replies/' + id + '/render', { variables }).then((r) => r.data.content)
