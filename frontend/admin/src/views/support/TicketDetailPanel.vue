<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  assignTicket,
  getTicket,
  getTicketSummary,
  markTicketRead,
  renderQuickReply,
  replyTicket,
  setTicketStatus,
  ticketPriorityLabels,
  ticketStatusLabels,
  listAgents,
  type CustomerServiceAgent,
  type TicketDetail,
  type TicketMessage,
} from '../../api/support'
import { errorMessage, when } from '../../utils/format'
import { useAuthStore } from '../../stores/auth'

// 工单处理面板：这是管理员真正工作的地方。左侧列表只负责选一张单，
// 全部上下文（对话、AI 工具调用、内部备注、时间线、快捷回复）都在这一个
// 面板里，服务员不必在多个页面之间跳来跳去才能拼出事情的全貌。
const props = defineProps<{ ticketId: number }>()
const emit = defineEmits<{ closed: [] }>()

const auth = useAuthStore()
const canManage = computed(() => auth.allows('tickets', 'manage'))
const canAssign = computed(() => auth.allows('tickets', 'assign'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const detail = ref<TicketDetail | null>(null)
const reply = ref('')
const internal = ref(false)
const tab = ref<'timeline' | 'tools' | 'logs'>('timeline')
const agents = ref<{ agent: CustomerServiceAgent; current_load: number }[]>([])
const summary = ref<{ summary: string; suggested_plan: string } | null>(null)

const ticket = computed(() => detail.value?.ticket ?? null)

const senderLabel = (type: string) => {
  switch (type) {
    case 'ai': return 'AI 客服'
    case 'agent': return '人工客服'
    case 'system': return '系统'
    default: return '用户'
  }
}

const senderTone = (type: string) => {
  switch (type) {
    case 'ai': return 'badge-info'
    case 'agent': return 'badge-success'
    case 'system': return 'badge'
    default: return 'badge-warning'
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    detail.value = await getTicket(props.ticketId)
    summary.value = await getTicketSummary(props.ticketId).catch(() => null)
    await markTicketRead(props.ticketId).catch(() => undefined)
    if (canAssign.value) {
      agents.value = await listAgents().catch(() => [])
    }
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
    await replyTicket(props.ticketId, reply.value.trim(), internal.value)
    notice.value = internal.value ? '内部备注已保存。' : '回复已发送。'
    const wasInternal = internal.value
    internal.value = false
    reply.value = ''
    if (!wasInternal) {
      // 客服首响后工单会进入人工处理中，刷新一次让状态跟着动。
      detail.value = await getTicket(props.ticketId)
    }
  } catch (err) {
    error.value = errorMessage(err, '发送失败')
  } finally {
    busy.value = false
  }
}

async function changeStatus(next: string) {
  busy.value = true
  error.value = ''
  try {
    await setTicketStatus(props.ticketId, next, '客服在客服中心更新状态')
    notice.value = '工单状态已更新为「' + (ticketStatusLabels[next] || next) + '」。'
    detail.value = await getTicket(props.ticketId)
    if (next === 'resolved' || next === 'closed') emit('closed')
  } catch (err) {
    error.value = errorMessage(err, '更新状态失败')
  } finally {
    busy.value = false
  }
}

async function assign(agentId: number) {
  if (!agentId) return
  busy.value = true
  try {
    await assignTicket(props.ticketId, agentId)
    notice.value = '工单已指派。'
    detail.value = await getTicket(props.ticketId)
  } catch (err) {
    error.value = errorMessage(err, '指派失败')
  } finally {
    busy.value = false
  }
}

async function useQuickReply(replyId: number) {
  try {
    reply.value = await renderQuickReply(replyId, {
      ticket_no: ticket.value?.ticket_no ?? '',
      user_name: detail.value?.username ?? '',
      order_no: ticket.value?.order_no ?? '',
    })
  } catch (err) {
    error.value = errorMessage(err, '读取快捷回复失败')
  }
}

const quickReplies = computed(() => detail.value?.quick_replies ?? [])

watch(() => props.ticketId, load)
onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
    </div>

    <div v-else-if="ticket" class="space-y-4">
      <div class="card space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <h3 class="text-base font-bold">{{ ticket.subject }}</h3>
          <span :class="ticket.status === 'resolved' || ticket.status === 'closed' ? 'badge-success' : 'badge-info'">
            {{ ticketStatusLabels[ticket.status] || ticket.status }}
          </span>
          <span class="badge-warning">{{ ticketPriorityLabels[ticket.priority] || ticket.priority }}</span>
          <span class="badge">{{ ticket.handler === 'ai' ? 'AI 处理' : '人工处理' }}</span>
        </div>
        <p class="mono quiet text-xs">
          {{ ticket.ticket_no }}
          <span v-if="ticket.order_no"> · 订单 {{ ticket.order_no }}</span>
          <span v-if="detail?.username"> · {{ detail.username }}</span>
        </p>

        <div v-if="summary?.summary" class="card-quiet">
          <p class="eyebrow">问题摘要</p>
          <p class="mt-1.5 text-sm leading-relaxed">{{ summary.summary }}</p>
          <p v-if="summary.suggested_plan" class="quiet mt-2 text-xs">推荐处理：{{ summary.suggested_plan }}</p>
        </div>

        <div class="flex flex-wrap gap-1.5">
          <button v-if="canManage" class="btn btn-secondary btn-sm" :disabled="busy" @click="changeStatus('human_handling')">接管处理</button>
          <button v-if="canManage" class="btn btn-secondary btn-sm" :disabled="busy" @click="changeStatus('resolved')">标记已解决</button>
          <button v-if="canManage" class="btn btn-secondary btn-sm" :disabled="busy" @click="changeStatus('closed')">关闭</button>
          <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="changeStatus('ai_processing')">重新交给 AI</button>
          <select
            v-if="canAssign && agents.length"
            class="input !w-auto !py-1.5 text-[12px]"
            aria-label="指派客服"
            @change="assign(Number(($event.target as HTMLSelectElement).value))"
          >
            <option value="">转交客服…</option>
            <option v-for="row in agents" :key="row.agent.id" :value="row.agent.id">
              {{ row.agent.nickname || '客服 #' + row.agent.id }}（{{ row.current_load }}/{{ row.agent.max_concurrent }}）
            </option>
          </select>
        </div>
      </div>

      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'timeline' ? 'chip-active' : ''" @click="tab = 'timeline'">对话</button>
        <button class="chip" :class="tab === 'tools' ? 'chip-active' : ''" @click="tab = 'tools'">
          AI 工具调用 <span class="nums">{{ detail?.tool_calls.length ?? 0 }}</span>
        </button>
        <button class="chip" :class="tab === 'logs' ? 'chip-active' : ''" @click="tab = 'logs'">操作时间线</button>
      </div>

      <div v-if="tab === 'timeline'" class="card max-h-[520px] space-y-3 overflow-y-auto">
        <div v-if="!detail?.messages.length" class="py-8 text-center text-sm text-[var(--text-quiet)]">还没有消息</div>
        <article
          v-for="message in (detail?.messages ?? [])"
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

      <div v-else-if="tab === 'tools'" class="card max-h-[420px] overflow-y-auto !p-0">
        <table class="table">
          <thead>
            <tr><th>工具</th><th>参数</th><th>结果</th><th>状态</th><th class="nums">耗时</th></tr>
          </thead>
          <tbody>
            <tr v-if="!detail?.tool_calls.length">
              <td colspan="5" class="py-10 text-center text-[var(--text-quiet)]">这次工单没有调用工具</td>
            </tr>
            <tr v-for="call in (detail?.tool_calls ?? [])" :key="call.id">
              <td>
                <p class="text-xs font-semibold">{{ call.tool_name || call.tool_key }}</p>
                <p class="mono quiet text-[11px]">{{ call.tool_key }}</p>
              </td>
              <td class="mono max-w-[180px] truncate text-[11px]">{{ call.params || '—' }}</td>
              <td class="mono max-w-[220px] truncate text-[11px]">{{ call.result || call.error || '—' }}</td>
              <td><span :class="call.status === 'ok' ? 'badge-success' : 'badge-danger'">{{ call.status }}</span></td>
              <td class="nums text-xs">{{ call.duration_ms }} ms</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="card max-h-[420px] space-y-2 overflow-y-auto">
        <div v-for="log in (detail?.logs ?? [])" :key="log.id" class="flex flex-wrap items-center gap-2 text-xs">
          <span class="mono">{{ log.action }}</span>
          <span class="quiet">{{ log.detail || log.after || '—' }}</span>
          <span class="quiet ml-auto">{{ when(log.created_at) }}</span>
        </div>
        <p v-if="!detail?.logs.length" class="py-8 text-center text-sm text-[var(--text-quiet)]">还没有操作记录</p>
      </div>

      <div v-if="canManage" class="card space-y-3">
        <div v-if="quickReplies.length" class="flex flex-wrap gap-1.5">
          <button v-for="item in quickReplies" :key="item.id" class="chip" @click="useQuickReply(item.id || 0)">
            {{ item.title }}
          </button>
        </div>
        <textarea v-model="reply" class="input min-h-[110px]" placeholder="输入回复内容，支持多行" />
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
  </div>
</template>
