<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  assignTicket,
  getTicket,
  markTicketRead,
  renderQuickReply,
  replyTicket,
  setTicketStatus,
  ticketPriorityLabels,
  ticketStatusLabels,
  type TicketDetail,
  type TicketMessage,
} from '../api/support'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const auth = useAuthStore()
const canManage = computed(() => auth.allows('tickets', 'manage'))
const canAssign = computed(() => auth.allows('tickets', 'assign'))

const id = computed(() => Number(route.params.id || 0))
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const detail = ref<TicketDetail | null>(null)
const reply = ref('')
const internal = ref(false)
const tab = ref<'timeline' | 'tools' | 'logs'>('timeline')

const messages = computed<TicketMessage[]>(() => detail.value?.messages ?? [])
const ticket = computed(() => detail.value?.ticket ?? null)

const senderLabel = (type: string) => {
  switch (type) {
    case 'ai':
      return 'AI 客服'
    case 'agent':
      return '人工客服'
    case 'system':
      return '系统'
    default:
      return '用户'
  }
}

const senderTone = (type: string) => {
  switch (type) {
    case 'ai':
      return 'badge-info'
    case 'agent':
      return 'badge-success'
    case 'system':
      return 'badge'
    default:
      return 'badge-warning'
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    detail.value = await getTicket(id.value)
    await markTicketRead(id.value).catch(() => undefined)
  } catch (err) {
    error.value = errorMessage(err, '加载工单失败')
  } finally {
    loading.value = false
  }
}

async function send() {
  if (!reply.value.trim()) return
  busy.value = true
  error.value = ''
  try {
    await replyTicket(id.value, reply.value.trim(), internal.value)
    reply.value = ''
    notice.value = internal.value ? '内部备注已保存。' : '回复已发送。'
    internal.value = false
    await load()
  } catch (err) {
    error.value = errorMessage(err, '发送失败')
  } finally {
    busy.value = false
  }
}

async function changeStatus(next: string) {
  busy.value = true
  try {
    await setTicketStatus(id.value, next, '客服在工单工作台更新状态')
    notice.value = '工单状态已更新。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '更新状态失败')
  } finally {
    busy.value = false
  }
}

async function useQuickReply(replyId: number) {
  try {
    const content = await renderQuickReply(replyId, {
      ticket_no: ticket.value?.ticket_no ?? '',
      user_name: detail.value?.username ?? '',
      order_no: ticket.value?.order_no ?? '',
    })
    reply.value = content
  } catch (err) {
    error.value = errorMessage(err, '读取快捷回复失败')
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-bold">{{ ticket?.subject || '工单详情' }}</h2>
        <p class="mono quiet mt-1 text-xs">
          {{ ticket?.ticket_no }}
          <span v-if="ticket?.order_no"> · 订单 {{ ticket.order_no }}</span>
          <span v-if="detail?.username"> · {{ detail.username }}</span>
        </p>
      </div>
      <div class="flex flex-wrap gap-2">
        <RouterLink to="/tickets" class="btn btn-secondary btn-sm">返回列表</RouterLink>
        <template v-if="canManage && ticket">
          <button class="btn btn-secondary btn-sm" :disabled="busy" @click="changeStatus('human_handling')">接管处理</button>
          <button class="btn btn-secondary btn-sm" :disabled="busy" @click="changeStatus('resolved')">标记已解决</button>
          <button class="btn btn-secondary btn-sm" :disabled="busy" @click="changeStatus('closed')">关闭工单</button>
          <button class="btn btn-quiet btn-sm" :disabled="busy" @click="changeStatus('ai_processing')">重新交给 AI</button>
        </template>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
    </div>

    <div v-else-if="ticket" class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_320px]">
      <div class="space-y-4">
        <div class="card space-y-3">
          <div class="flex flex-wrap items-center gap-2">
            <span class="badge">{{ ticketStatusLabels[ticket.status] || ticket.status }}</span>
            <span class="badge-warning">{{ ticketPriorityLabels[ticket.priority] || ticket.priority }}</span>
            <span class="badge-info">{{ ticket.handler === 'ai' ? 'AI 处理' : '人工处理' }}</span>
            <span v-if="ticket.source" class="badge">{{ ticket.source }}</span>
          </div>
          <div v-if="ticket.summary" class="card-quiet">
            <p class="eyebrow">问题摘要</p>
            <p class="mt-1.5 text-sm leading-relaxed">{{ ticket.summary }}</p>
            <p v-if="ticket.suggested_plan" class="quiet mt-2 text-xs">推荐处理：{{ ticket.suggested_plan }}</p>
          </div>
          <p v-if="detail?.messages.length" class="quiet text-xs">
            共 {{ detail.messages.length }} 条消息
            <span v-if="ticket.transfer_reason"> · 转人工原因：{{ ticket.transfer_reason }}</span>
          </p>
        </div>

        <div class="flex gap-1.5">
          <button class="chip" :class="tab === 'timeline' ? 'chip-active' : ''" @click="tab = 'timeline'">对话</button>
          <button class="chip" :class="tab === 'tools' ? 'chip-active' : ''" @click="tab = 'tools'">
            AI 工具调用 <span class="nums">{{ detail?.tool_calls.length ?? 0 }}</span>
          </button>
          <button class="chip" :class="tab === 'logs' ? 'chip-active' : ''" @click="tab = 'logs'">操作时间线</button>
        </div>

        <div v-if="tab === 'timeline'" class="card space-y-3">
          <div v-if="!messages.length" class="py-8 text-center text-sm text-[var(--text-quiet)]">还没有消息</div>
          <article
            v-for="message in messages"
            :key="message.id"
            class="card-quiet"
            :class="message.is_internal ? 'border-[var(--warning)]' : ''"
          >
            <div class="flex flex-wrap items-center gap-2">
              <span :class="senderTone(message.sender_type)">{{ senderLabel(message.sender_type) }}</span>
              <span v-if="message.is_internal" class="badge-warning">内部备注</span>
              <span class="quiet ml-auto text-xs">{{ when(message.created_at) }}</span>
            </div>
            <p class="mt-2 whitespace-pre-line text-sm leading-relaxed">{{ message.content }}</p>
          </article>
        </div>

        <div v-else-if="tab === 'tools'" class="table-container">
          <table class="table">
            <thead>
              <tr>
                <th>工具</th>
                <th>参数</th>
                <th>结果</th>
                <th>状态</th>
                <th class="nums">耗时</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="!detail?.tool_calls.length">
                <td colspan="6" class="py-10 text-center text-[var(--text-quiet)]">这次工单没有调用工具</td>
              </tr>
              <tr v-for="call in detail?.tool_calls" :key="call.id">
                <td>
                  <p class="text-xs font-semibold">{{ call.tool_name || call.tool_key }}</p>
                  <p class="mono quiet text-[11px]">{{ call.tool_key }}</p>
                </td>
                <td class="mono max-w-[220px] truncate text-[11px]">{{ call.params || '—' }}</td>
                <td class="mono max-w-[260px] truncate text-[11px]">{{ call.result || call.error || '—' }}</td>
                <td><span :class="call.status === 'ok' ? 'badge-success' : 'badge-danger'">{{ call.status }}</span></td>
                <td class="nums text-xs">{{ call.duration_ms }} ms</td>
                <td class="quiet text-xs">{{ when(call.created_at) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-else class="card space-y-2">
          <div v-for="log in detail?.logs" :key="log.id" class="flex flex-wrap items-center gap-2 text-xs">
            <span class="mono">{{ log.action }}</span>
            <span class="quiet">{{ log.detail || log.after || '—' }}</span>
            <span class="quiet ml-auto">{{ when(log.created_at) }}</span>
          </div>
          <p v-if="!detail?.logs.length" class="py-8 text-center text-sm text-[var(--text-quiet)]">还没有操作记录</p>
        </div>

        <div v-if="canManage" class="card space-y-3">
          <div v-if="detail?.quick_replies?.length" class="flex flex-wrap gap-1.5">
            <button
              v-for="item in detail.quick_replies"
              :key="item.id"
              class="chip"
              @click="useQuickReply(item.id || 0)"
            >
              {{ item.title }}
            </button>
          </div>
          <textarea v-model="reply" class="input min-h-[120px]" placeholder="输入回复内容，支持多行" />
          <div class="flex flex-wrap items-center gap-3">
            <label class="flex items-center gap-2 text-sm">
              <input v-model="internal" type="checkbox" />
              作为内部备注（买家不可见）
            </label>
            <button class="btn btn-primary btn-sm ml-auto" :disabled="busy || !reply.trim()" @click="send">
              {{ busy ? '发送中…' : '发送' }}
            </button>
          </div>
        </div>
      </div>

      <aside class="space-y-4">
        <div class="card space-y-3">
          <p class="eyebrow">工单信息</p>
          <dl class="space-y-2 text-sm">
            <div class="flex justify-between gap-3"><dt class="quiet">类型</dt><dd>{{ ticket.type }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="quiet">来源</dt><dd>{{ ticket.source || 'web' }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="quiet">消息数</dt><dd class="nums">{{ ticket.message_count }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="quiet">附件</dt><dd class="nums">{{ ticket.attachment_count }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="quiet">创建</dt><dd>{{ when(ticket.created_at) }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="quiet">首响</dt><dd>{{ ticket.first_response_at ? when(ticket.first_response_at) : '—' }}</dd></div>
            <div class="flex justify-between gap-3"><dt class="quiet">时限</dt><dd>{{ ticket.due_at ? when(ticket.due_at) : '—' }}</dd></div>
          </dl>
        </div>

        <div v-if="ticket.satisfaction" class="card">
          <p class="eyebrow">满意度</p>
          <p class="nums mt-2 text-2xl font-bold">{{ ticket.satisfaction }} / 5</p>
          <p v-if="ticket.satisfaction_note" class="quiet mt-2 text-xs">{{ ticket.satisfaction_note }}</p>
        </div>

        <div v-if="detail?.related?.length" class="card">
          <p class="eyebrow">历史工单</p>
          <ul class="mt-2 space-y-1.5 text-sm">
            <li v-for="item in detail.related" :key="item.id">
              <RouterLink :to="'/tickets/' + item.id" class="hover:accent-text">{{ item.subject }}</RouterLink>
              <span class="quiet ml-1 text-xs">{{ ticketStatusLabels[item.status] || item.status }}</span>
            </li>
          </ul>
        </div>

        <div v-if="detail?.assignments?.length" class="card">
          <p class="eyebrow">分配记录</p>
          <ul class="mt-2 space-y-1 text-xs">
            <li v-for="item in detail.assignments" :key="item.id">
              {{ item.strategy }} · 客服 #{{ item.agent_id }} · {{ when(item.assigned_at) }}
            </li>
          </ul>
        </div>
      </aside>
    </div>
  </section>
</template>
