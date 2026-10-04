<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import AdminIcon from '../../components/AdminIcon.vue'
import AppDrawer from '../../components/AppDrawer.vue'
import StatusBadge from '../../components/StatusBadge.vue'
import {
  assignTicket,
  autoAssignTicket,
  claimTicket,
  getTicket,
  getTicketSummary,
  listAgents,
  markTicketRead,
  renderQuickReply,
  replyTicket,
  setTicketStatus,
  ticketPriorityLabels,
  ticketStatusLabels,
  type CustomerServiceAgent,
  type TicketDetail,
} from '../../api/support'
import { errorMessage, when } from '../../utils/format'
import { useAuthStore } from '../../stores/auth'

/**
 * 工单处理面板：左侧队列只负责选一张单，全部上下文都在这里——
 * 对话、内部备注、时间线、快捷回复、转交与状态流转。
 */

const props = defineProps<{ ticketId: number }>()
const emit = defineEmits<{ changed: [] }>()

const auth = useAuthStore()
const canManage = computed(() => auth.allows('tickets', 'manage'))
const canAssign = computed(() => auth.allows('tickets', 'assign'))

const loading = ref(true)
const busy = ref(false)
const action = ref('')
const error = ref('')
const notice = ref('')
const detail = ref<TicketDetail | null>(null)
const summary = ref<{ summary: string; suggested_plan: string } | null>(null)
const agents = ref<{ agent: CustomerServiceAgent; current_load: number }[]>([])
const reply = ref('')
const internal = ref(false)
const tab = ref<'timeline' | 'logs'>('timeline')
const showSummary = ref(false)
const showAssign = ref(false)

const ticket = computed(() => detail.value?.ticket ?? null)
const messages = computed(() => detail.value?.messages ?? [])
const quickReplies = computed(() => detail.value?.quick_replies ?? [])

async function load() {
  loading.value = true
  error.value = ''
  try {
    detail.value = await getTicket(props.ticketId)
    // 打开即视为已读：未读角标跟着消失，不必再点一次「标记已读」。
    await markTicketRead(props.ticketId).catch(() => undefined)
    if (canAssign.value) {
      agents.value = await listAgents().catch(() => [])
    }
    emit('changed')
  } catch (err) {
    error.value = errorMessage(err, '加载工单失败')
  } finally {
    loading.value = false
  }
}

async function run(name: string, task: () => Promise<unknown>, message: string) {
  if (busy.value) return
  busy.value = true
  action.value = name
  error.value = ''
  notice.value = ''
  try {
    await task()
    notice.value = message
    detail.value = await getTicket(props.ticketId)
    emit('changed')
  } catch (err) {
    error.value = errorMessage(err, '操作失败')
  } finally {
    busy.value = false
    action.value = ''
  }
}

function changeStatus(next: string) {
  void run(
    'status',
    () => setTicketStatus(props.ticketId, next, '客服在客服中心更新状态'),
    '工单状态已更新为「' + (ticketStatusLabels[next] ?? next) + '」',
  )
}

function send() {
  if (!reply.value.trim()) return
  const body = reply.value.trim()
  const isInternal = internal.value
  void run(isInternal ? 'note' : 'reply', async () => {
    await replyTicket(props.ticketId, body, isInternal)
    reply.value = ''
    internal.value = false
  }, isInternal ? '内部备注已保存。' : '回复已发送。')
}

async function useQuickReply(id: number) {
  try {
    reply.value = await renderQuickReply(id, {
      ticket_no: ticket.value?.ticket_no ?? '',
      user_name: detail.value?.username ?? '',
      order_no: ticket.value?.order_no ?? '',
    })
  } catch (err) {
    error.value = errorMessage(err, '读取快捷回复失败')
  }
}

async function openSummary() {
  if (!summary.value) {
    summary.value = await getTicketSummary(props.ticketId).catch(() => null)
  }
  showSummary.value = true
}

function assignTo(agentId: number) {
  showAssign.value = false
  void run('assign', () => assignTicket(props.ticketId, agentId), '工单已指派')
}

function autoAssign() {
  void run('auto-assign', () => autoAssignTicket(props.ticketId), '已自动分配给负载最低的在线客服')
}

function claim() {
  void run('claim', () => claimTicket(props.ticketId), '已接管这张工单')
}

const senderMeta = (type: string) => {
  switch (type) {
    case 'ai': return { label: '历史记录', tone: 'badge-neutral' }
    case 'agent': return { label: '人工客服', tone: 'badge-success' }
    case 'system': return { label: '系统', tone: 'badge-neutral' }
    default: return { label: '用户', tone: 'badge-warning' }
  }
}

watch(() => props.ticketId, load)
onMounted(load)
</script>

<template>
  <div class="space-y-3">
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
    </div>

    <div v-else-if="error && !ticket" class="card text-center">
      <p class="alert alert-danger text-left" role="alert">{{ error }}</p>
      <button class="btn btn-secondary btn-sm mt-3" :disabled="loading" @click="load">重新加载工单</button>
    </div>

    <template v-else-if="ticket">
      <!-- 工单头部与操作 -->
      <div class="card ticket-panel-head space-y-4">
        <div class="ticket-panel-title">
          <div class="min-w-0">
            <p class="eyebrow">工单处理</p>
            <h3 class="mt-1 min-w-0 truncate text-[17px] font-bold">{{ ticket.subject }}</h3>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-1.5">
            <StatusBadge :value="ticket.status" :label="ticketStatusLabels[ticket.status]" />
            <StatusBadge :value="ticket.priority" :label="ticketPriorityLabels[ticket.priority]" />
            <span class="badge-neutral">{{ ticket.handler === 'human' ? (ticket.assigned_agent_id ? '已指派客服' : '人工待分派') : '历史工单' }}</span>
          </div>
        </div>
        <dl class="ticket-meta">
          <div>
            <dt>工单号</dt>
            <dd class="mono">{{ ticket.ticket_no }}</dd>
          </div>
          <div>
            <dt>买家</dt>
            <dd>{{ detail?.username || '#' + ticket.user_id }}</dd>
          </div>
          <div v-if="ticket.order_no">
            <dt>关联订单</dt>
            <dd class="mono">{{ ticket.order_no }}</dd>
          </div>
          <div>
            <dt>更新时间</dt>
            <dd>{{ when(ticket.updated_at || ticket.created_at) }}</dd>
          </div>
        </dl>

        <div v-if="canManage" class="ticket-actions">
          <!-- 按一天的处理顺序排：先接单，再流转，最后结单。
               主操作（接管）在前，转交类在中间，结单类靠右。 -->
          <button
            v-if="ticket.handler !== 'human'"
            class="btn btn-primary btn-sm"
            :disabled="busy"
            @click="claim"
          >
            <span v-if="action === 'claim'" class="spinner spinner-light !size-3.5" />
            {{ action === 'claim' ? '接管中…' : '接管处理' }}
          </button>
          <button
            v-if="ticket.status !== 'human_handling'"
            class="btn btn-secondary btn-sm"
            :disabled="busy"
            @click="changeStatus('human_handling')"
          >
            <span v-if="action === 'status'" class="spinner !size-3.5" />
            {{ action === 'status' ? '更新中…' : '我来处理' }}
          </button>
          <button v-if="canAssign" class="btn btn-quiet btn-sm" :disabled="busy" @click="showAssign = true">转交客服</button>
          <button v-if="canAssign" class="btn btn-quiet btn-sm" :disabled="busy" @click="autoAssign">
            {{ action === 'auto-assign' ? '分配中…' : '自动分配' }}
          </button>
          <button class="btn btn-quiet btn-sm" @click="openSummary">问题摘要</button>
          <button class="btn btn-secondary btn-sm ticket-action-end" :disabled="busy" @click="changeStatus('resolved')">标记已解决</button>
          <button class="btn btn-quiet btn-sm" :disabled="busy" @click="changeStatus('closed')">关闭</button>
        </div>
      </div>

      <!-- 对话 / 工具 / 时间线 -->
      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'timeline' ? 'chip-active' : ''" @click="tab = 'timeline'">
          对话 <span class="nums opacity-70">{{ messages.length }}</span>
        </button>
        <button class="chip" :class="tab === 'logs' ? 'chip-active' : ''" @click="tab = 'logs'">时间线</button>
      </div>

      <div v-if="tab === 'timeline'" class="card max-h-[460px] space-y-2.5 overflow-y-auto">
        <p v-if="!messages.length" class="quiet py-8 text-center text-sm">还没有消息</p>
        <article
          v-for="message in messages"
          :key="message.id"
          class="card-quiet"
          :class="message.is_internal ? 'border-[var(--warning)]' : ''"
        >
          <div class="flex flex-wrap items-center gap-2">
            <span :class="senderMeta(message.sender_type).tone">{{ senderMeta(message.sender_type).label }}</span>
            <span v-if="message.is_internal" class="badge-warning">内部备注</span>
            <span class="quiet ml-auto text-[11px]">{{ when(message.created_at) }}</span>
          </div>
          <p class="mt-2 whitespace-pre-line text-[13px] leading-relaxed">{{ message.content }}</p>
        </article>
      </div>

      <div v-else-if="tab === 'logs'" class="card max-h-[420px] space-y-2 overflow-y-auto">
        <p v-if="!detail?.logs?.length" class="quiet py-8 text-center text-sm">还没有操作记录</p>
        <div v-for="log in detail?.logs ?? []" :key="log.id" class="flex flex-wrap items-center gap-2 text-xs">
          <span class="mono">{{ log.action }}</span>
          <span class="quiet min-w-0 flex-1 truncate">{{ log.detail || log.after || '—' }}</span>
          <span class="quiet">{{ when(log.created_at) }}</span>
        </div>
      </div>

      <!-- 回复区 -->
      <div v-if="canManage" class="card space-y-3">
        <div class="card-head !mb-0">
          <div>
            <h4 class="text-[13.5px] font-bold">回复买家</h4>
            <p class="sub">回复会进入买家工单；内部备注只给客服看。</p>
          </div>
          <span v-if="busy && (action === 'reply' || action === 'note')" class="badge-info">
            {{ action === 'note' ? '保存备注中' : '发送中' }}
          </span>
        </div>
        <div v-if="quickReplies.length" class="flex flex-wrap gap-1.5">
          <button v-for="item in quickReplies" :key="item.id" class="chip" @click="useQuickReply(item.id ?? 0)">
            {{ item.title }}
          </button>
        </div>
        <textarea v-model="reply" class="input min-h-[104px]" placeholder="输入回复内容，支持多行" />
        <div class="flex flex-wrap items-center gap-3">
          <label class="flex items-center gap-2 text-sm">
            <input v-model="internal" type="checkbox" />
            作为内部备注（买家不可见）
          </label>
          <button class="btn btn-primary btn-sm ml-auto" :disabled="busy || !reply.trim()" @click="send">
            <span v-if="busy && (action === 'reply' || action === 'note')" class="spinner !size-3.5" />
            {{ action === 'note' ? '保存备注' : '发送回复' }}
          </button>
        </div>
      </div>
    </template>

    <!-- 问题摘要 -->
    <AppDrawer :open="showSummary" title="问题摘要" width="sm" @close="showSummary = false">
      <div v-if="summary" class="space-y-4">
        <div>
          <p class="eyebrow">摘要</p>
          <p class="mt-1.5 text-[13px] leading-relaxed">{{ summary.summary || '暂无可提炼的信息' }}</p>
        </div>
        <div>
          <p class="eyebrow">推荐处理</p>
          <p class="mt-1.5 text-[13px] leading-relaxed">{{ summary.suggested_plan || '按售后规则处理' }}</p>
        </div>
      </div>
      <p v-else class="quiet py-8 text-center text-sm">摘要暂时不可用</p>
    </AppDrawer>

    <!-- 转交客服 -->
    <AppDrawer :open="showAssign" title="转交客服" width="sm" @close="showAssign = false">
      <p v-if="!agents.length" class="quiet py-8 text-center text-sm">还没有配置客服坐席</p>
      <ul v-else class="space-y-2">
        <li v-for="row in agents" :key="row.agent.id">
          <button class="card-quiet flex w-full items-center gap-3 text-left" @click="assignTo(row.agent.id ?? 0)">
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-semibold">{{ row.agent.nickname || '客服 #' + row.agent.id }}</span>
              <span class="quiet text-xs">
                负载 {{ row.current_load }}/{{ row.agent.max_concurrent }}
                · {{ row.agent.status === 'online' ? '在线' : row.agent.status === 'busy' ? '忙碌' : '离线' }}
              </span>
            </span>
            <AdminIcon name="chevronRight" :size="15" class="text-[var(--text-quiet)]" />
          </button>
        </li>
      </ul>
    </AppDrawer>
  </div>
</template>

<style scoped>
.ticket-panel-head {
  border-top: 2px solid var(--accent-line);
}

.ticket-panel-title {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.ticket-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 1px;
  overflow: hidden;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-sm);
  background: var(--stroke-quiet);
}

.ticket-meta > div {
  min-width: 0;
  background: var(--surface-sunken);
  padding: 9px 11px;
}

.ticket-meta dt {
  font-size: 10.5px;
  color: var(--text-quiet);
}

.ticket-meta dd {
  margin-top: 2px;
  overflow: hidden;
  color: var(--text-dim);
  font-size: 12.5px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ticket-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 7px;
  border-top: 1px solid var(--stroke-quiet);
  padding-top: 13px;
}

.ticket-action-end {
  margin-left: auto;
}

@media (min-width: 640px) {
  .ticket-meta {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (max-width: 639px) {
  .ticket-action-end {
    margin-left: 0;
  }
}
</style>
