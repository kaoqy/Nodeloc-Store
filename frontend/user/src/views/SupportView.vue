<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  claimCoupon,
  createTicket,
  getTicket,
  listActivities,
  listMyTickets,
  rateTicket,
  reopenTicket,
  replyTicket,
  ticketStatusLabels,
  ticketTypeOptions,
  transferTicket,
  type Activity,
  type Ticket,
  type TicketDetail,
} from '../api/support'
import { errorMessage } from '../api/client'
import { when } from '../utils/format'

const route = useRoute()
const router = useRouter()
const tab = ref<'activities' | 'tickets'>('activities')

const activities = ref<Activity[]>([])
const activityError = ref('')
const activityLoading = ref(true)
const claiming = ref('')

const tickets = ref<Ticket[]>([])
const ticketLoading = ref(true)
const ticketError = ref('')
const statusFilter = ref('all')
const showCreate = ref(false)
const creating = ref(false)
const detail = ref<TicketDetail | null>(null)
const detailLoading = ref(false)
const reply = ref('')
const sendingReply = ref(false)
const rating = ref(5)
const ratingComment = ref('')

const form = ref({ type: 'order', subject: '', content: '', order_no: '' })

const filteredTickets = computed(() =>
  statusFilter.value === 'all' ? tickets.value : tickets.value.filter((item) => item.status === statusFilter.value),
)

const statusTone: Record<string, string> = {
  ai_processing: 'badge-info',
  waiting_user: 'badge',
  ai_solved: 'badge-success',
  user_requested_human: 'badge-warning',
  pending_human: 'badge-warning',
  human_handling: 'badge-info',
  waiting_confirm: 'badge',
  resolved: 'badge-success',
  closed: 'badge',
  rejected: 'badge-danger',
  cancelled: 'badge',
}

const typeLabel = (value: string) =>
  ticketTypeOptions.find((option) => option.value === value)?.label ?? value

async function loadActivities() {
  activityLoading.value = true
  activityError.value = ''
  try {
    activities.value = await listActivities()
  } catch (err) {
    activityError.value = errorMessage(err, '加载活动失败')
  } finally {
    activityLoading.value = false
  }
}

async function loadTickets() {
  ticketLoading.value = true
  ticketError.value = ''
  try {
    const page = await listMyTickets({ limit: 50 })
    tickets.value = page.data
  } catch (err) {
    ticketError.value = errorMessage(err, '加载工单失败')
  } finally {
    ticketLoading.value = false
  }
}

async function claim(activity: Activity) {
  claiming.value = String(activity.id)
  try {
    await claimCoupon(activity.id)
    activityError.value = ''
    alert('优惠券已领取，可在下单时使用。')
  } catch (err) {
    activityError.value = errorMessage(err, '领取失败')
  } finally {
    claiming.value = ''
  }
}

async function submitTicket() {
  if (!form.value.subject.trim()) {
    ticketError.value = '请填写工单标题。'
    return
  }
  creating.value = true
  ticketError.value = ''
  try {
    const created = await createTicket({
      type: form.value.type,
      subject: form.value.subject,
      content: form.value.content,
      order_no: form.value.order_no,
    })
    showCreate.value = false
    form.value = { type: 'order', subject: '', content: '', order_no: '' }
    await loadTickets()
    await openTicket(created.id)
  } catch (err) {
    ticketError.value = errorMessage(err, '提交工单失败')
  } finally {
    creating.value = false
  }
}

async function openTicket(id: number) {
  detailLoading.value = true
  ticketError.value = ''
  try {
    detail.value = await getTicket(id)
    rating.value = detail.value.ticket.satisfaction || 5
    ratingComment.value = ''
  } catch (err) {
    ticketError.value = errorMessage(err, '加载工单失败')
  } finally {
    detailLoading.value = false
  }
}

async function sendReply() {
  if (!detail.value || !reply.value.trim()) return
  sendingReply.value = true
  try {
    await replyTicket(detail.value.ticket.id, reply.value.trim())
    reply.value = ''
    await openTicket(detail.value.ticket.id)
    await loadTickets()
  } catch (err) {
    ticketError.value = errorMessage(err, '发送失败')
  } finally {
    sendingReply.value = false
  }
}

async function askHuman() {
  if (!detail.value) return
  try {
    await transferTicket(detail.value.ticket.id, '用户在小程序页申请转人工')
    await openTicket(detail.value.ticket.id)
    await loadTickets()
  } catch (err) {
    ticketError.value = errorMessage(err, '转人工失败')
  }
}

async function submitRating() {
  if (!detail.value) return
  try {
    await rateTicket(detail.value.ticket.id, rating.value, ratingComment.value)
    await openTicket(detail.value.ticket.id)
  } catch (err) {
    ticketError.value = errorMessage(err, '评价失败')
  }
}

async function reopen() {
  if (!detail.value) return
  try {
    await reopenTicket(detail.value.ticket.id)
    await openTicket(detail.value.ticket.id)
    await loadTickets()
  } catch (err) {
    ticketError.value = errorMessage(err, '重新打开失败')
  }
}

onMounted(() => {
  if (route.query.tab === 'tickets') tab.value = 'tickets'
  void loadActivities()
  void loadTickets()
  const order = typeof route.query.order === 'string' ? route.query.order : ''
  if (order) {
    showCreate.value = true
    tab.value = 'tickets'
    form.value.order_no = order
  }
})
</script>

<template>
  <div class="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="eyebrow">活动与客服</p>
        <h1 class="mt-2 text-2xl font-bold">活动中心 · 我的工单</h1>
      </div>
      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'activities' ? 'chip-active' : ''" @click="tab = 'activities'">活动中心</button>
        <button class="chip" :class="tab === 'tickets' ? 'chip-active' : ''" @click="tab = 'tickets'">我的工单</button>
      </div>
    </header>

    <template v-if="tab === 'activities'">
      <p v-if="activityError" class="alert alert-danger mt-6" role="alert">{{ activityError }}</p>
      <div v-if="activityLoading" class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <div v-for="i in 3" :key="i" class="card space-y-3"><div class="skeleton h-5 w-2/3" /><div class="skeleton h-4 w-full" /></div>
      </div>
      <div v-else-if="!activities.length" class="card mt-6 py-16 text-center">
        <p class="font-semibold">暂时没有进行中的活动</p>
        <p class="mt-1.5 text-sm text-[var(--text-quiet)]">有新的促销活动时会显示在这里。</p>
      </div>
      <div v-else class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
        <article v-for="activity in activities" :key="activity.id" class="card space-y-3">
          <img v-if="activity.cover_image" :src="activity.cover_image" :alt="activity.name" class="aspect-[16/9] w-full rounded-[var(--radius-sm)] object-cover" />
          <div>
            <h2 class="font-semibold">{{ activity.name }}</h2>
            <p v-if="activity.subtitle" class="quiet mt-1 text-sm">{{ activity.subtitle }}</p>
          </div>
          <p v-if="activity.description" class="line-clamp-3 text-sm text-[var(--text-dim)]">{{ activity.description }}</p>
          <p class="quiet text-xs">
            <span v-if="activity.start_at">开始 {{ when(activity.start_at) }}</span>
            <span v-if="activity.end_at"> · 结束 {{ when(activity.end_at) }}</span>
          </p>
          <button
            v-if="activity.type === 'coupon_claim'"
            class="btn btn-primary btn-sm"
            :disabled="claiming === String(activity.id)"
            @click="claim(activity)"
          >
            {{ claiming === String(activity.id) ? '领取中…' : '领取优惠券' }}
          </button>
          <RouterLink v-else to="/" class="btn btn-secondary btn-sm">去逛逛</RouterLink>
        </article>
      </div>
    </template>

    <template v-else>
      <div class="mt-6 flex flex-wrap items-center gap-2">
        <select v-model="statusFilter" class="input !w-auto" aria-label="工单状态">
          <option value="all">全部状态</option>
          <option v-for="(label, value) in ticketStatusLabels" :key="value" :value="value">{{ label }}</option>
        </select>
        <button class="btn btn-secondary btn-sm" @click="loadTickets">刷新</button>
        <button class="btn btn-primary btn-sm ml-auto" @click="showCreate = true">+ 提交工单</button>
      </div>

      <p v-if="ticketError" class="alert alert-danger mt-4" role="alert">{{ ticketError }}</p>

      <div class="mt-4 grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
        <div class="space-y-2">
          <div v-if="ticketLoading" class="card space-y-2"><div v-for="i in 3" :key="i" class="skeleton h-8 w-full" /></div>
          <div v-else-if="!filteredTickets.length" class="card py-10 text-center text-sm text-[var(--text-quiet)]">还没有工单</div>
          <button
            v-for="item in filteredTickets"
            :key="item.id"
            class="card-quiet w-full text-left transition-colors"
            :class="detail?.ticket.id === item.id ? 'border-[var(--accent-line)]' : ''"
            @click="openTicket(item.id)"
          >
            <p class="truncate text-sm font-semibold">{{ item.subject }}</p>
            <p class="mono quiet mt-1 text-xs">{{ item.ticket_no }}</p>
            <p class="mt-1.5 flex items-center gap-2 text-xs">
              <span :class="statusTone[item.status] || 'badge'">{{ ticketStatusLabels[item.status] || item.status }}</span>
              <span class="quiet">{{ typeLabel(item.type) }}</span>
            </p>
          </button>
        </div>

        <div>
          <div v-if="detailLoading" class="card space-y-3"><div v-for="i in 4" :key="i" class="skeleton h-8 w-full" /></div>
          <div v-else-if="!detail" class="card py-16 text-center text-sm text-[var(--text-quiet)]">选择左侧工单查看对话</div>
          <div v-else class="space-y-4">
            <div class="card space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-lg font-bold">{{ detail.ticket.subject }}</h2>
                <span :class="statusTone[detail.ticket.status] || 'badge'">{{ ticketStatusLabels[detail.ticket.status] || detail.ticket.status }}</span>
                <span class="badge">{{ detail.ticket.handler === 'ai' ? 'AI 处理' : '人工处理' }}</span>
              </div>
              <p class="mono quiet text-xs">
                {{ detail.ticket.ticket_no }}
                <span v-if="detail.ticket.order_no"> · 订单 {{ detail.ticket.order_no }}</span>
              </p>
              <p v-if="detail.ticket.summary" class="quiet text-xs">{{ detail.ticket.summary }}</p>
              <div class="flex flex-wrap gap-2">
                <button class="btn btn-secondary btn-sm" @click="askHuman">转人工客服</button>
                <button v-if="['resolved','closed'].includes(detail.ticket.status)" class="btn btn-secondary btn-sm" @click="reopen">重新打开</button>
              </div>
            </div>

            <div class="card space-y-3">
              <article v-for="message in detail.messages" :key="message.id" class="card-quiet">
                <div class="flex items-center gap-2 text-xs">
                  <span :class="message.sender_type === 'user' ? 'badge-warning' : message.sender_type === 'ai' ? 'badge-info' : 'badge-success'">
                    {{ message.sender_type === 'user' ? '我' : message.sender_type === 'ai' ? 'AI 客服' : message.sender_type === 'agent' ? '人工客服' : '系统' }}
                  </span>
                  <span class="quiet ml-auto">{{ when(message.created_at) }}</span>
                </div>
                <p class="mt-2 whitespace-pre-line text-sm">{{ message.content }}</p>
              </article>
              <div class="flex items-end gap-2">
                <textarea v-model="reply" class="input min-h-[70px]" placeholder="继续追问，AI 会先接待；需要人工时点上方按钮" />
                <button class="btn btn-primary btn-sm" :disabled="sendingReply || !reply.trim()" @click="sendReply">发送</button>
              </div>
            </div>

            <div v-if="['resolved','closed'].includes(detail.ticket.status)" class="card space-y-3">
              <p class="eyebrow">满意度评价</p>
              <div class="flex gap-1.5">
                <button v-for="score in 5" :key="score" class="chip" :class="rating === score ? 'chip-active' : ''" @click="rating = score">
                  {{ score }} 分
                </button>
              </div>
              <input v-model="ratingComment" class="input" placeholder="补充说明（可选）" />
              <button class="btn btn-primary btn-sm" @click="submitRating">提交评价</button>
            </div>
          </div>
        </div>
      </div>
    </template>

    <div v-if="showCreate" class="scrim fixed inset-0 z-50 grid place-items-center p-4" @click.self="showCreate = false">
      <div class="card w-full max-w-lg space-y-3">
        <h2 class="text-lg font-bold">提交工单</h2>
        <select v-model="form.type" class="input">
          <option v-for="option in ticketTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
        <input v-model="form.order_no" class="input" placeholder="关联订单号（可选）" />
        <input v-model="form.subject" class="input" placeholder="问题标题（必填）" />
        <textarea v-model="form.content" class="input min-h-[120px]" placeholder="详细描述你遇到的问题" />
        <p class="quiet text-xs">提交后会由 AI 客服先接待，你可以随时转人工。</p>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showCreate = false">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="creating" @click="submitTicket">{{ creating ? '提交中…' : '提交' }}</button>
        </div>
      </div>
    </div>
  </div>
</template>
