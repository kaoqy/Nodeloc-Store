<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
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

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const open = ref(false)
const loading = ref(true)
const sending = ref(false)
const error = ref('')
const config = ref<SupportConfig | null>(null)
const questions = ref<QuickQuestion[]>([])
const conversationId = ref(0)
const draft = ref('')
const transferNotice = ref('')
const needConfirm = ref<{ ticketId: number; reason: string } | null>(null)
const stream = ref<{ role: 'user' | 'assistant' | 'system'; content: string; id: number; rating?: number }[]>([])
const messageList = ref<HTMLElement | null>(null)

const enabled = computed(() => config.value?.enabled === true)
const loginRequired = computed(() => Boolean(config.value && !config.value.guest_allowed && !auth.isAuthenticated))
const pageContext = computed(() => route.fullPath)

async function load() {
  loading.value = true
  try {
    config.value = await getSupportConfig().catch(() => null)
    questions.value = await getQuickQuestions('widget').catch(() => [])
    if (enabled.value && config.value?.greeting && !stream.value.length) {
      stream.value.push({ role: 'assistant', content: config.value.greeting, id: 0 })
    }
  } finally {
    loading.value = false
  }
}

async function scrollToBottom() {
  await nextTick()
  if (messageList.value) messageList.value.scrollTop = messageList.value.scrollHeight
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
  stream.value.push({ role: 'user', content, id: -Date.now() })
  draft.value = ''
  await scrollToBottom()
  try {
    const reply: ChatReply = await sendChat({
      content,
      conversation_id: conversationId.value || undefined,
      page_context: pageContext.value,
    })
    conversationId.value = reply.conversation_id
    stream.value.push({ role: 'assistant', content: reply.content, id: reply.message_id })
    if (reply.suggest_transfer) {
      transferNotice.value = '如果你希望人工处理，可以点下方「转人工客服」。'
    }
    if (reply.need_confirm) {
      needConfirm.value = { ticketId: reply.ticket_id ?? 0, reason: reply.content }
    }
    await scrollToBottom()
  } catch (err) {
    error.value = errorMessage(err, '发送失败，请稍后重试。')
    stream.value.push({ role: 'system', content: '消息发送失败，请点击重试。', id: -Date.now() })
  } finally {
    sending.value = false
  }
}

async function rate(messageId: number, rating: number) {
  if (!conversationId.value) return
  try {
    await sendFeedback({ conversation_id: conversationId.value, message_id: messageId || undefined, rating })
    const item = stream.value.find((entry) => entry.id === messageId)
    if (item) item.rating = rating
  } catch {
    // 评价失败不影响对话继续。
  }
}

async function transferToHuman() {
  if (!auth.isAuthenticated) {
    void router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  try {
    const ticket = await transferTicket(0, '用户在客服窗口申请转人工')
    const ticketID = (ticket as { ticket?: { id?: number } })?.ticket?.id
    transferNotice.value = '已转人工客服，工单号：' + ((ticket as { ticket_no?: string })?.ticket_no ?? '')
    if (ticketID) {
      stream.value.push({ role: 'system', content: '已为你创建人工工单，客服会尽快跟进。', id: 0 })
      transferNotice.value = '工单已创建，可在「我的工单」里查看进度。'
    }
  } catch (err) {
    error.value = errorMessage(err, '转人工失败，请稍后重试。')
  }
}

function reset() {
  stream.value = []
  conversationId.value = 0
  transferNotice.value = ''
  if (config.value?.greeting) {
    stream.value.push({ role: 'assistant', content: config.value.greeting, id: 0 })
  }
}

watch(open, (value) => {
  if (value) void scrollToBottom()
})

// 客服中心页面的按钮通过事件打开这个窗口：买家用一个入口就能找到对话框，
// 不用在页面上再放一个位置不同、样式不同的聊天面板。
function onOpenRequest() {
  open.value = true
  void scrollToBottom()
}

onMounted(() => {
  window.addEventListener('nodeloc:open-support', onOpenRequest)
})

onBeforeUnmount(() => {
  window.removeEventListener('nodeloc:open-support', onOpenRequest)
})

onMounted(load)
</script>

<template>
  <div v-if="enabled" class="fixed bottom-5 right-5 z-40 print:hidden">
    <button
      v-if="!open"
      class="btn btn-primary shadow-lg"
      aria-label="打开在线客服"
      @click="open = true"
    >
      <span aria-hidden="true">💬</span>
      <span class="hidden sm:inline">{{ config?.agent_name || '在线客服' }}</span>
    </button>

    <div
      v-else
      class="card flex h-[520px] w-[min(92vw,380px)] flex-col !p-0 shadow-2xl"
      role="dialog"
      aria-label="在线客服"
    >
      <header class="flex items-center gap-3 border-b border-[var(--stroke)] px-4 py-3">
        <span class="brand-mark">{{ (config?.agent_name || 'AI').slice(0, 1) }}</span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-sm font-semibold">{{ config?.agent_name || '智能客服' }}</span>
          <span class="quiet block truncate text-xs">
            {{ config?.working_hours || 'AI 先接待，可随时转人工' }}
          </span>
        </span>
        <button class="btn btn-quiet btn-sm" aria-label="关闭客服" @click="open = false">✕</button>
      </header>

      <div ref="messageList" class="flex-1 space-y-3 overflow-y-auto px-4 py-4">
        <div v-if="loading" class="space-y-2">
          <div class="skeleton h-8 w-3/4" />
          <div class="skeleton h-8 w-1/2" />
        </div>
        <template v-else>
          <div
            v-for="message in stream"
            :key="message.id"
            class="flex"
            :class="message.role === 'user' ? 'justify-end' : 'justify-start'"
          >
            <div
              class="max-w-[85%] rounded-[var(--radius-sm)] px-3 py-2 text-sm leading-relaxed whitespace-pre-line"
              :class="message.role === 'user' ? 'bg-[var(--accent)] text-[var(--on-accent)]' : 'bg-[var(--surface-hi)]'"
            >
              {{ message.content }}
              <div v-if="message.role === 'assistant' && message.id > 0" class="mt-1.5 flex gap-2 text-xs opacity-70">
                <button @click="rate(message.id, 1)">{{ message.rating === 1 ? '👍 已赞' : '👍' }}</button>
                <button @click="rate(message.id, -1)">{{ message.rating === -1 ? '👎 已反馈' : '👎' }}</button>
              </div>
            </div>
          </div>
          <p v-if="sending" class="quiet text-xs">正在输入…</p>
        </template>
      </div>

      <p v-if="error" class="alert alert-danger mx-4 mb-2 text-xs" role="alert">{{ error }}</p>
      <p v-if="transferNotice" class="quiet mx-4 mb-2 text-xs">{{ transferNotice }}</p>

      <div class="border-t border-[var(--stroke)] px-4 py-3">
        <div v-if="questions.length" class="mb-2 flex flex-wrap gap-1.5">
          <button
            v-for="question in questions.slice(0, 4)"
            :key="question.id"
            class="chip text-xs"
            @click="send(question.content || question.title)"
          >
            {{ question.title }}
          </button>
        </div>
        <div class="flex items-end gap-2">
          <textarea
            v-model="draft"
            class="input min-h-[44px] flex-1 resize-none text-sm"
            rows="1"
            maxlength="2000"
            placeholder="描述你的问题…"
            @keydown.enter.exact.prevent="send()"
          />
          <button class="btn btn-primary btn-sm" :disabled="sending || !draft.trim()" @click="send()">发送</button>
        </div>
        <div class="mt-2 flex items-center gap-2 text-xs">
          <button class="hint" @click="transferToHuman">转人工客服</button>
          <button class="hint" @click="reset">清空对话</button>
          <RouterLink v-if="auth.isAuthenticated" to="/tickets" class="hint ml-auto">我的工单</RouterLink>
        </div>
      </div>
    </div>
  </div>
</template>
