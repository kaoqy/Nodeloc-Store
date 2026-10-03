<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  createTicket,
  getQuickQuestions,
  getSupportConfig,
  sendChat,
  sendFeedback,
  transferTicket,
  type ChatReply,
  type QuickQuestion,
  type SupportConfig,
} from '../api/support'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'

/**
 * 在线客服窗口。
 *
 * 结构上分成三段：头部（身份与状态）、消息流、输入区。
 * 消息流里除了对话气泡，还展示「AI 查了哪些数据」与「依据了哪篇文档」——
 * 这两块是买家判断答案可不可信的唯一线索，之前完全没露出来。
 */

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

type PendingConfirm = { toolKey: string; toolName: string; params: Record<string, unknown> }

type StreamItem = {
  id: number
  role: 'user' | 'assistant' | 'system'
  content: string
  rating?: number
  tools?: { name: string; key: string; status: string; error?: string }[]
  sources?: { id: number; title: string }[]
  suggestions?: string[]
  /** 需要用户点确认才能执行的高风险操作，例如退款。 */
  confirm?: PendingConfirm
  at: number
}

const open = ref(false)
const loading = ref(true)
const sending = ref(false)
const submitting = ref(false)
const error = ref('')
const config = ref<SupportConfig | null>(null)
const questions = ref<QuickQuestion[]>([])
const conversationId = ref(0)
const draft = ref('')
const stream = ref<StreamItem[]>([])
const messageList = ref<HTMLElement | null>(null)
const createdTicketNo = ref('')
// 当前对话关联的工单：转人工和追问都落到同一张单上。
const activeTicketId = ref(0)


const enabled = computed(() => config.value?.enabled === true)
const loginRequired = computed(() => Boolean(config.value && !config.value.guest_allowed && !auth.isAuthenticated))
const pageContext = computed(() => route.fullPath)
const agentName = computed(() => config.value?.agent_name || '智能客服')

// 建议追问取自最后一条 AI 回复：只在用户没有新输入时展示，避免刷屏。
const lastAssistantSuggestions = computed(() => {
  for (let i = stream.value.length - 1; i >= 0; i--) {
    const item = stream.value[i]
    if (item.role === 'assistant' && item.suggestions?.length) return item.suggestions
    if (item.role === 'user') return []
  }
  return []
})

// 会话存本地：刷新页面后还能接着问，不必重新描述问题。
const STORAGE_KEY = 'support.conversation'

function persist() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({
      conversationId: conversationId.value,
      ticketId: activeTicketId.value,
      stream: stream.value.slice(-40),
    }))
  } catch {
    // 本地存储写失败不影响对话本身。
  }
}

function restore() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return false
    const saved = JSON.parse(raw) as { conversationId?: number; ticketId?: number; stream?: StreamItem[] }
    if (!Array.isArray(saved.stream) || !saved.stream.length) return false
    conversationId.value = saved.conversationId ?? 0
    activeTicketId.value = saved.ticketId ?? 0
    stream.value = saved.stream
    return true
  } catch {
    return false
  }
}

async function load() {
  loading.value = true
  try {
    config.value = await getSupportConfig().catch(() => null)
    questions.value = await getQuickQuestions('widget').catch(() => [])
    if (!restore() && enabled.value && config.value?.greeting) {
      stream.value.push({ id: 0, role: 'assistant', content: config.value.greeting, at: Date.now() })
    }
  } finally {
    loading.value = false
  }
}

async function scrollToBottom() {
  await nextTick()
  const el = messageList.value
  if (el) el.scrollTop = el.scrollHeight
}

function push(item: Omit<StreamItem, 'at'> & { at?: number }) {
  stream.value.push({ ...item, at: item.at ?? Date.now() })
  persist()
}

async function send(text?: string) {
  const content = (text ?? draft.value).trim()
  if (!content || sending.value) return
  if (loginRequired.value) {
    error.value = '请先登录后再使用在线客服。'
    return
  }
  sending.value = true
  error.value = ''
  push({ id: -Date.now(), role: 'user', content })
  draft.value = ''
  await scrollToBottom()

  try {
    const reply: ChatReply = await sendChat({
      content,
      conversation_id: conversationId.value || undefined,
      ticket_id: activeTicketId.value || undefined,
      page_context: pageContext.value,
    })
    applyReply(reply)
  } catch (err) {
    error.value = errorMessage(err, '发送失败，请稍后重试。')
    push({ id: -Date.now(), role: 'system', content: '消息发送失败，可以再点一次发送。' })
  } finally {
    sending.value = false
  }
}

// applyReply 把一次 AI 回答落成一条消息，并把「需要确认」的高风险操作挂上去。
function applyReply(reply: ChatReply) {
  if (reply.conversation_id) conversationId.value = reply.conversation_id
  // AI 可能自己建了工单：把编号与 id 记下来，之后的追问和转人工都落到这张单上。
  if (reply.ticket_no) createdTicketNo.value = reply.ticket_no
  if (reply.ticket_id) activeTicketId.value = reply.ticket_id
  const pending = reply.need_confirm
  push({
    id: reply.message_id,
    role: 'assistant',
    content: reply.content,
    tools: (reply.tool_calls ?? []).map((call) => ({
      name: call.tool_name || call.tool_key,
      key: call.tool_key,
      status: call.status,
      error: call.error,
    })),
    sources: (reply.knowledge_hits ?? []).map((hit) => ({ id: hit.id, title: hit.title })),
    suggestions: reply.suggested_replies ?? [],
    confirm: pending?.require_confirm
      ? {
          toolKey: pending.tool_key,
          toolName: pending.tool_name || pending.tool_key,
          params: (pending.params ?? {}) as Record<string, unknown>,
        }
      : undefined,
  })
}

// confirmTool 是用户点了「确认执行」：把工具与参数原样交回后端，
// 后端会重新做一次权限与参数校验，再真正执行。
async function confirmTool(item: StreamItem) {
  if (!item.confirm || sending.value) return
  sending.value = true
  error.value = ''
  const pending = item.confirm
  item.confirm = undefined
  try {
    const reply = await sendChat({
      content: '确认执行' + pending.toolName,
      conversation_id: conversationId.value || undefined,
      page_context: pageContext.value,
      confirmed: true,
      pending_tool: pending.toolKey,
      pending_params: pending.params,
    })
    applyReply(reply)
  } catch (err) {
    error.value = errorMessage(err, '执行失败，请稍后重试或转人工。')
    item.confirm = pending
  } finally {
    sending.value = false
    await scrollToBottom()
  }
}

// confirmSummary 用买家看得懂的话描述这次待确认操作，而不是丢一个工具标识。
function confirmSummary(pending: PendingConfirm): string {
  const params = pending.params ?? {}
  const orderNo = typeof params.order_no === 'string' ? params.order_no : ''
  switch (pending.toolKey) {
    case 'refund.order':
      return orderNo ? '对订单 ' + orderNo + ' 发起退款，金额原路退回你的 NodeLoc 账户。' : '发起订单退款，金额原路退回你的 NodeLoc 账户。'
    case 'ticket.add_message':
      return '把这条内容追加到你的工单里。'
    case 'notification.send':
      return '给你发送一条站内通知。'
    default:
      return '执行「' + pending.toolName + '」。'
  }
}

function cancelConfirm(item: StreamItem) {
  item.confirm = undefined
  push({ id: -Date.now(), role: 'system', content: '已取消这次操作，需要的话可以随时再提。' })
  persist()
}

async function rate(messageId: number, rating: number) {
  if (!conversationId.value) return
  try {
    await sendFeedback({ conversation_id: conversationId.value, message_id: messageId || undefined, rating })
    const item = stream.value.find((entry) => entry.id === messageId)
    if (item) item.rating = rating
    persist()
  } catch {
    // 评价失败不影响对话继续。
  }
}

// 提交工单：把这段对话的诉求整理成一张工单，之后由 AI 在工单里继续处理。
async function createTicketFromChat() {
  if (!auth.isAuthenticated) {
    void router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  const text = draft.value.trim() || lastUserMessage()
  if (!text) {
    error.value = '先说一句你的问题，再提交工单。'
    return
  }
  submitting.value = true
  error.value = ''
  try {
    const ticket = await createTicket({ type: 'other', subject: text.slice(0, 40), content: text })
    createdTicketNo.value = ticket.ticket_no
    activeTicketId.value = ticket.id
    push({
      id: 0,
      role: 'system',
      content: '工单 ' + ticket.ticket_no + ' 已创建，智能客服正在处理。你也可以点「查看工单」跟进。',
    })
    await scrollToBottom()
  } catch (err) {
    error.value = errorMessage(err, '创建工单失败，请稍后重试。')
  } finally {
    submitting.value = false
  }
}

function lastUserMessage(): string {
  for (let i = stream.value.length - 1; i >= 0; i--) {
    if (stream.value[i].role === 'user') return stream.value[i].content
  }
  return ''
}

// 转人工：保留完整上下文，进人工队列。
async function transferToHuman() {
  if (!auth.isAuthenticated) {
    void router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  if (!activeTicketId.value) {
    // 还没有工单：先建一张，避免转人工落到一个不存在的工单号上。
    await createTicketFromChat()
    if (!activeTicketId.value) return
  }
  submitting.value = true
  error.value = ''
  try {
    const result = await transferTicket(activeTicketId.value, '用户在客服窗口申请转人工')
    const no = result.ticket_no || createdTicketNo.value
    createdTicketNo.value = no
    push({
      id: -Date.now(),
      role: 'system',
      content: no
        ? '已转人工客服，工单号 ' + no + '，客服会尽快跟进。'
        : '已转人工客服，客服会尽快跟进。',
    })
    await scrollToBottom()
  } catch (err) {
    error.value = errorMessage(err, '转人工失败，请稍后重试。')
  } finally {
    submitting.value = false
  }
}

function reset() {
  stream.value = []
  conversationId.value = 0
  activeTicketId.value = 0
  createdTicketNo.value = ''
  localStorage.removeItem(STORAGE_KEY)
  if (config.value?.greeting) {
    push({ id: 0, role: 'assistant', content: config.value.greeting })
  }
}

function openWidget() {
  open.value = true
  void scrollToBottom()
}

watch(open, (value) => {
  if (value) void scrollToBottom()
})

onMounted(() => {
  void load()
  window.addEventListener('nodeloc:open-support', openWidget)
})

onBeforeUnmount(() => {
  window.removeEventListener('nodeloc:open-support', openWidget)
})
</script>

<template>
  <div v-if="enabled" class="support-root print:hidden">
    <!-- 收起状态：一个圆形悬浮按钮 -->
    <button
      v-if="!open"
      class="support-fab"
      type="button"
      :aria-label="'打开' + agentName"
      @click="openWidget"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" class="size-6" aria-hidden="true">
        <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v7a2.5 2.5 0 0 1-2.5 2.5H12l-4.5 3.4V16H6.5A2.5 2.5 0 0 1 4 13.5z" />
        <path d="M8.5 9.5h7M8.5 12.5h4.5" />
      </svg>
      <span class="support-fab-label">{{ agentName }}</span>
    </button>

    <!-- 展开状态：对话面板 -->
    <section
      v-else
      class="support-panel"
      role="dialog"
      aria-modal="false"
      :aria-label="agentName"
    >
      <header class="flex items-center gap-3 border-b border-[var(--stroke)] px-4 py-3">
        <span class="relative shrink-0">
          <img
            v-if="config?.avatar"
            :src="config.avatar"
            :alt="agentName"
            class="size-9 rounded-full object-cover"
          />
          <span v-else class="brand-mark">{{ agentName.slice(0, 1) }}</span>
          <span class="support-online" aria-hidden="true" />
        </span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[13.5px] font-semibold">{{ agentName }}</span>
          <span class="quiet block truncate text-[11.5px]">
            {{ config?.working_hours || 'AI 先接待 · 可随时转人工' }}
          </span>
        </span>
        <button class="support-close" type="button" aria-label="收起对话" @click="open = false">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="size-4" aria-hidden="true">
            <path d="M6 12h12" />
          </svg>
        </button>
      </header>

      <div ref="messageList" class="flex-1 space-y-3 overflow-y-auto px-4 py-4">
        <div v-if="loading" class="space-y-3">
          <div class="skeleton h-9 w-3/4" />
          <div class="skeleton h-9 w-1/2" />
        </div>

        <template v-else>
          <div
            v-for="item in stream"
            :key="item.id + '-' + item.at"
            class="flex"
            :class="item.role === 'user' ? 'justify-end' : 'justify-start'"
          >
            <div
              class="support-bubble"
              :class="item.role === 'user' ? 'support-bubble-user' : item.role === 'system' ? 'support-bubble-system' : 'support-bubble-ai'"
            >
              <p class="whitespace-pre-line text-[13px] leading-relaxed">{{ item.content }}</p>

              <!-- 依据：引用了哪几篇文档 -->
              <div v-if="item.sources?.length" class="support-meta">
                <span class="support-meta-label">依据</span>
                <span v-for="source in item.sources" :key="source.id" class="support-chip">{{ source.title }}</span>
              </div>

              <!-- 工具：这一轮查了什么 -->
              <div v-if="item.tools?.length" class="support-meta">
                <span class="support-meta-label">已查询</span>
                <span
                  v-for="tool in item.tools"
                  :key="tool.key"
                  class="support-chip"
                  :class="tool.status === 'ok' ? '' : 'support-chip-warn'"
                >
                  {{ tool.name }}
                </span>
              </div>

              <!-- 高风险操作：退款这类写操作必须由买家本人确认，AI 不能自己执行 -->
              <div v-if="item.confirm" class="support-confirm">
                <p class="support-confirm-title">需要你确认</p>
                <p class="support-confirm-text">{{ confirmSummary(item.confirm) }}</p>
                <div class="mt-2 flex gap-2">
                  <button class="btn btn-primary btn-sm" :disabled="sending" @click="confirmTool(item)">
                    确认执行
                  </button>
                  <button class="btn btn-quiet btn-sm" :disabled="sending" @click="cancelConfirm(item)">取消</button>
                </div>
              </div>

              <div v-if="item.role === 'assistant' && item.id > 0" class="mt-2 flex items-center gap-2 text-[11px]">
                <button class="support-rate" :class="{ 'support-rate-on': item.rating === 1 }" @click="rate(item.id, 1)">
                  👍 有用
                </button>
                <button class="support-rate" :class="{ 'support-rate-on': item.rating === -1 }" @click="rate(item.id, -1)">
                  👎 没用
                </button>
              </div>
            </div>
          </div>

          <p v-if="sending" class="quiet flex items-center gap-2 text-[12px]">
            <span class="support-typing" aria-hidden="true"><i /><i /><i /></span>
            正在查询并整理…
          </p>
        </template>
      </div>

      <p v-if="error" class="alert alert-danger mx-4 mb-2 text-xs" role="alert">{{ error }}</p>

      <div v-if="createdTicketNo" class="mx-4 mb-2 flex flex-wrap items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--accent-line)] bg-[var(--accent-soft)] px-3 py-2 text-xs">
        <span>工单 {{ createdTicketNo }} 已创建</span>
        <RouterLink to="/tickets" class="hint ml-auto">查看工单 →</RouterLink>
      </div>

      <div class="border-t border-[var(--stroke)] px-4 py-3">
        <!-- 快捷问题与建议追问 -->
        <div v-if="!stream.length && questions.length" class="mb-2 flex flex-wrap gap-1.5">
          <button
            v-for="question in questions.slice(0, 4)"
            :key="question.id"
            class="chip text-xs"
            @click="send(question.content || question.title)"
          >
            {{ question.title }}
          </button>
        </div>
        <div
          v-else-if="lastAssistantSuggestions.length"
          class="mb-2 flex flex-wrap gap-1.5"
        >
          <button
            v-for="item in lastAssistantSuggestions"
            :key="item"
            class="chip text-xs"
            @click="send(item)"
          >
            {{ item }}
          </button>
        </div>

        <div class="support-input">
          <textarea
            v-model="draft"
            class="support-textarea"
            rows="1"
            maxlength="2000"
            :placeholder="loginRequired ? '请先登录后再提问' : '描述你的问题，AI 会先查订单与规则…'"
            @keydown.enter.exact.prevent="send()"
          />
          <button
            class="support-send"
            type="button"
            :disabled="sending || !draft.trim()"
            aria-label="发送"
            @click="send()"
          >
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="size-4" aria-hidden="true">
              <path d="M4 12l16-8-6 8 6 8z" />
            </svg>
          </button>
        </div>

        <div class="mt-2 flex flex-wrap items-center gap-2 text-[11.5px]">
          <button class="support-action" :disabled="submitting" @click="createTicketFromChat">提交工单</button>
          <button class="support-action" :disabled="submitting" @click="transferToHuman">转人工</button>
          <button class="support-action support-action-quiet" @click="reset">清空</button>
          <RouterLink v-if="auth.isAuthenticated" to="/tickets" class="support-action support-action-quiet ml-auto">
            我的工单
          </RouterLink>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* ── 悬浮按钮 ─────────────────────────────────────────────────── */
.support-root {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 40;
}
.support-fab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 18px 12px 14px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--accent-line);
  background: linear-gradient(140deg, var(--accent-hi), var(--accent));
  color: var(--on-accent);
  font-size: 13.5px;
  font-weight: 600;
  box-shadow: var(--shadow-accent), var(--shadow-md);
  transition: transform 220ms var(--spring), box-shadow var(--normal);
}
.support-fab:hover { transform: translateY(-2px); box-shadow: var(--shadow-accent), var(--shadow-lg); }
.support-fab-label { white-space: nowrap; }

/* ── 对话面板 ─────────────────────────────────────────────────── */
.support-panel {
  display: flex;
  flex-direction: column;
  width: min(94vw, 400px);
  height: min(78vh, 620px);
  border: 1px solid var(--stroke);
  border-radius: var(--radius-lg);
  background: var(--surface);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  animation: support-in 240ms var(--spring);
}
@keyframes support-in {
  from { opacity: 0; transform: translateY(12px) scale(0.98); }
  to { opacity: 1; transform: none; }
}

.support-close {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  border: 1px solid transparent;
  color: var(--text-quiet);
  transition: background var(--fast), color var(--fast), border-color var(--fast);
}
.support-close:hover { background: var(--surface-hi); border-color: var(--stroke); color: var(--text); }

.support-online {
  position: absolute;
  right: -1px;
  bottom: -1px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid var(--surface);
  background: var(--success);
}

/* ── 气泡 ─────────────────────────────────────────────────────── */
.support-bubble {
  max-width: 84%;
  border-radius: var(--radius-md);
  padding: 9px 12px;
}
.support-bubble-ai {
  background: var(--surface-hi);
  border: 1px solid var(--stroke-quiet);
  border-bottom-left-radius: 4px;
}
.support-bubble-user {
  background: linear-gradient(140deg, var(--accent-hi), var(--accent));
  color: var(--on-accent);
  border-bottom-right-radius: 4px;
}
.support-bubble-system {
  max-width: 100%;
  background: var(--accent-soft);
  border: 1px solid var(--accent-line);
  color: var(--text-dim);
  font-size: 12.5px;
}

.support-meta {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--stroke);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}
.support-meta-label { color: var(--text-quiet); }
.support-chip {
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke);
  padding: 1px 8px;
  color: var(--text-dim);
}
.support-chip-warn { border-color: var(--warning); color: var(--warning); }

.support-confirm {
  margin-top: 8px;
  border: 1px solid var(--warning);
  border-radius: var(--radius-sm);
  background: var(--warning-soft, var(--accent-soft));
  padding: 8px 10px;
}
.support-confirm-title { font-size: 11.5px; font-weight: 700; color: var(--warning); }
.support-confirm-text { margin-top: 3px; font-size: 12px; line-height: 1.6; color: var(--text-dim); }

.support-rate {
  border-radius: var(--radius-pill);
  padding: 2px 8px;
  color: var(--text-quiet);
  transition: background var(--fast), color var(--fast);
}
.support-rate:hover { background: var(--surface-hi); color: var(--text-dim); }
.support-rate-on { background: var(--accent-soft); color: var(--accent); }

/* 输入中的三点动画 */
.support-typing { display: inline-flex; gap: 3px; }
.support-typing i {
  width: 4px; height: 4px; border-radius: 50%;
  background: var(--text-quiet);
  animation: support-blink 1.2s infinite ease-in-out;
}
.support-typing i:nth-child(2) { animation-delay: 0.15s; }
.support-typing i:nth-child(3) { animation-delay: 0.3s; }
@keyframes support-blink { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }

/* ── 输入区 ───────────────────────────────────────────────────── */
.support-input {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
  padding: 6px 6px 6px 12px;
  transition: border-color var(--fast);
}
.support-input:focus-within { border-color: var(--accent-line); }
.support-textarea {
  flex: 1;
  min-height: 34px;
  max-height: 120px;
  border: 0;
  background: transparent;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
  line-height: 1.6;
  resize: none;
  outline: none;
}
.support-send {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: var(--on-accent);
  transition: opacity var(--fast);
}
.support-send:disabled { opacity: 0.45; }

.support-action {
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke);
  padding: 3px 10px;
  color: var(--text-dim);
  transition: background var(--fast), border-color var(--fast), color var(--fast);
}
.support-action:hover { border-color: var(--accent-line); color: var(--accent); background: var(--accent-soft); }
.support-action-quiet { border-color: transparent; color: var(--text-quiet); }
.support-action:disabled { opacity: 0.5; }

/* 小屏：面板铺满可用宽度，按钮文字隐藏 */
@media (max-width: 480px) {
  .support-root { right: 12px; bottom: 12px; left: 12px; }
  .support-fab { width: 100%; justify-content: center; }
  .support-panel { width: 100%; height: min(80vh, 560px); }
  .support-fab-label { display: none; }
}

@media (prefers-reduced-motion: reduce) {
  .support-panel { animation: none; }
  .support-fab { transition: none; }
}
</style>
