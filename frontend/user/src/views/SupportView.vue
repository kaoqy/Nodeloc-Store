<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import {
  createTicket,
  getTicket,
  listMyTickets,
  cancelTicket,
  rateTicket,
  reopenTicket,
  replyTicket,
  transferTicket,
  ticketStatusLabels,
  ticketTypeOptions,
  type Ticket,
  type TicketDetail,
} from '../api/support'
import { errorMessage } from '../api/client'
import { when } from '../utils/format'

// 客服中心：把「帮助文档 / 我的工单」收进一个页面，
// 买家用一个入口就能找到全部售后路径。
const route = useRoute()

const tab = ref<'help' | 'tickets'>('help')

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
const closingTicket = ref(false)
const closingConfirm = ref(false)
const rating = ref(5)
const ratingComment = ref('')

const form = ref({ type: 'order', subject: '', content: '', order_no: '' })

const helpSections = [
  { id: 'buy', title: '如何购买商品', body: '在首页搜索或选择分类，打开商品详情页确认价格、库存和交付方式，填写必要信息后点击立即购买。未登录时会先进入登录流程。' },
  { id: 'payment', title: '支付完成后在哪里查看', body: '支付完成后回到订单详情页。商店以服务端确认结果为准自动核实支付状态；请不要因为页面暂未更新而重复付款。' },
  { id: 'delivery', title: '卡密什么时候发放', body: '自动发货商品在支付确认且有可用库存后交付。若库存暂时不足，订单会显示等待补货；人工交付商品由商家在订单中完成发货。' },
  { id: 'card-issue', title: '卡密无效或已被使用', body: '带着订单号提交工单，客服会先核对交付记录。核实为无效卡后会按售后规则重新发货或退款。' },
  { id: 'refund', title: '退款与售后规则', body: '数字商品具有一次性交付属性，已交付且可正常使用的卡密原则上不支持退款。未交付、卡密无效或重复交付的情况可以申请售后。' },
  { id: 'oauth', title: 'NodeLoc 登录失败', body: '请从登录页重新发起一次授权。若持续失败，请把登录时间与页面提示提供给客服，不要提供 Client Secret、授权码或 Token。' },
  { id: 'human', title: '需要人工客服', body: '在本页「我的工单」里提交问题，客服会按顺序跟进。提交时可以填写订单号，方便客服直接核对。' },
]

const openHelp = ref('buy')

const queueHint = ref('30 分钟')
const filteredTickets = computed(() =>
  statusFilter.value === 'all' ? tickets.value : tickets.value.filter((item) => item.status === statusFilter.value),
)

const statusTone: Record<string, string> = {
  user_requested_human: 'badge-warning',
  pending_human: 'badge-warning',
  human_handling: 'badge-info',
  resolved: 'badge-success',
  closed: 'badge',
  rejected: 'badge-danger',
  cancelled: 'badge',
}

const typeLabel = (value: string) =>
  ticketTypeOptions.find((option) => option.value === value)?.label ?? value

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
    await transferTicket(detail.value.ticket.id, '用户在客服中心申请转人工')
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

async function closeTicket() {
  if (!detail.value || closingTicket.value) return
  closingTicket.value = true
  ticketError.value = ''
  try {
    await cancelTicket(detail.value.ticket.id)
    closingConfirm.value = false
    await openTicket(detail.value.ticket.id)
    await loadTickets()
  } catch (err) {
    ticketError.value = errorMessage(err, '关闭工单失败')
  } finally {
    closingTicket.value = false
  }
}

const canClose = computed(() =>
  Boolean(detail.value && !['closed', 'cancelled', 'rejected'].includes(detail.value.ticket.status)),
)

// 客服窗口已下线，联系客服统一走工单：切到工单标签并打开提交表单。
function startTicket() {
  tab.value = 'tickets'
  ticketError.value = ''
  showCreate.value = true
}

onMounted(() => {
  if (route.query.tab === 'tickets') tab.value = 'tickets'
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
  <div class="mx-auto w-full max-w-6xl px-4 py-9 sm:px-6">
    <header class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="eyebrow">客服中心</p>
        <h1 class="mt-2 text-2xl font-bold sm:text-3xl">有问题，从这里开始</h1>
        <p class="mt-3 max-w-2xl text-sm leading-relaxed text-[var(--text-dim)]">
          先看下面的常见问题；没解决就提交工单，客服会带着你的描述和订单信息跟进。
        </p>
      </div>
      <button class="btn btn-primary" @click="startTicket">
        提交工单
      </button>
    </header>

    <div class="support-actions mt-6 grid gap-3 sm:grid-cols-3">
      <button class="support-action text-left" @click="startTicket">
        <p class="font-semibold">联系客服</p>
        <p class="quiet mt-1 text-xs">描述问题，提交后由客服跟进</p>
      </button>
      <button
        class="support-action text-left"
        @click="tab = 'tickets'; showCreate = true"
      >
        <p class="font-semibold">提交工单</p>
        <p class="quiet mt-1 text-xs">复杂问题留档，客服按顺序跟进</p>
      </button>
      <!-- 活动是独立页面：这里只给一个指路卡片，不再把促销内容混进客服页。 -->
      <RouterLink to="/activities" class="support-action text-left">
        <p class="font-semibold">活动中心</p>
        <p class="quiet mt-1 text-xs">查看正在进行的促销与领券活动</p>
      </RouterLink>
    </div>

    <div class="mt-8 flex flex-wrap gap-1.5" role="tablist" aria-label="客服中心板块">
      <button class="chip" :class="tab === 'help' ? 'chip-active' : ''" role="tab" :aria-selected="tab === 'help'" @click="tab = 'help'">
        帮助文档
      </button>
      <button class="chip" :class="tab === 'tickets' ? 'chip-active' : ''" role="tab" :aria-selected="tab === 'tickets'" @click="tab = 'tickets'">
        我的工单 <span v-if="tickets.length" class="nums opacity-70">{{ tickets.length }}</span>
      </button>
    </div>

    <!-- 帮助文档 -->
    <section v-if="tab === 'help'" class="mt-5 grid gap-3">
      <article v-for="item in helpSections" :key="item.id" class="card overflow-hidden !p-0">
        <button
          class="flex min-h-14 w-full items-center justify-between gap-4 px-5 py-4 text-left font-semibold transition-colors hover:bg-[var(--surface-hi)]"
          type="button"
          :aria-expanded="openHelp === item.id"
          @click="openHelp = openHelp === item.id ? '' : item.id"
        >
          <span>{{ item.title }}</span>
          <span class="mono text-lg text-[var(--text-quiet)]" aria-hidden="true">{{ openHelp === item.id ? '−' : '+' }}</span>
        </button>
        <p v-if="openHelp === item.id" class="border-t border-[var(--stroke-quiet)] px-5 py-4 text-sm leading-7 text-[var(--text-dim)]">
          {{ item.body }}
        </p>
      </article>
      <div class="card flex flex-wrap items-center gap-3">
        <p class="text-sm text-[var(--text-dim)]">没有找到答案？直接提交工单，客服会帮你查订单。</p>
        <button class="btn btn-secondary btn-sm ml-auto" @click="startTicket">提交工单</button>
      </div>
    </section>

    <!-- 我的工单 -->
    <section v-else class="mt-5">
      <div class="flex flex-wrap items-center gap-2">
        <select v-model="statusFilter" class="input !w-auto" aria-label="工单状态">
          <option value="all">全部状态</option>
          <option v-for="(label, value) in ticketStatusLabels" :key="value" :value="value">{{ label }}</option>
        </select>
        <button class="btn btn-secondary btn-sm" @click="loadTickets">刷新</button>
        <button class="btn btn-primary btn-sm ml-auto" @click="showCreate = true">+ 提交工单</button>
      </div>

      <p v-if="ticketError" class="alert alert-danger mt-4" role="alert">{{ ticketError }}</p>

      <div class="mt-4 grid gap-4 xl:grid-cols-[320px_minmax(0,1fr)]">
        <div class="support-ticket-list space-y-2">
          <div v-if="ticketLoading" class="card space-y-2">
            <div v-for="i in 3" :key="i" class="skeleton h-8 w-full" />
          </div>
          <div v-else-if="!filteredTickets.length" class="card py-10 text-center text-sm text-[var(--text-quiet)]">
            还没有工单
          </div>
          <button
            v-for="item in filteredTickets"
            :key="item.id"
            class="support-ticket-row w-full text-left"
            :class="detail?.ticket.id === item.id ? 'support-ticket-row-active' : ''"
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
          <div v-if="detailLoading" class="card space-y-3">
            <div v-for="i in 4" :key="i" class="skeleton h-8 w-full" />
          </div>
          <div v-else-if="!detail" class="card py-16 text-center text-sm text-[var(--text-quiet)]">
            选择左侧工单查看对话
          </div>
          <div v-else class="space-y-4">
            <div class="card support-ticket-detail space-y-2">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="text-lg font-bold">{{ detail.ticket.subject }}</h2>
                <span :class="statusTone[detail.ticket.status] || 'badge'">{{ ticketStatusLabels[detail.ticket.status] || detail.ticket.status }}</span>
                <span class="badge">人工处理</span>
              </div>
              <p class="mono quiet text-xs">
                {{ detail.ticket.ticket_no }}
                <span v-if="detail.ticket.order_no"> · 订单 {{ detail.ticket.order_no }}</span>
              </p>
              <p v-if="detail.ticket.summary" class="quiet text-xs">{{ detail.ticket.summary }}</p>
              <p class="quiet text-xs">
                客服会按队列顺序跟进，一般 {{ queueHint }} 内回复。
              </p>
              <div class="flex flex-wrap gap-2">
                <span class="badge-success self-center">
                  {{ ['closed', 'cancelled'].includes(detail.ticket.status) ? '工单已结束' : '人工客服跟进中' }}
                </span>
                <button v-if="['resolved','closed'].includes(detail.ticket.status)" class="btn btn-secondary btn-sm" @click="reopen">
                  重新打开
                </button>
                <button v-if="canClose" class="btn btn-quiet btn-sm" :disabled="closingTicket" @click="closingConfirm = true">
                  关闭工单
                </button>
              </div>
              <p v-if="detail.ticket.status === 'cancelled'" class="alert alert-info" role="status">
                这张工单已由你关闭。如仍需处理，可以重新打开并补充说明。
              </p>
            </div>

            <div class="card space-y-3">
              <article v-for="message in detail.messages" :key="message.id" class="card-quiet">
                <div class="flex items-center gap-2 text-xs">
                  <span :class="message.sender_type === 'user' ? 'badge-warning' : message.sender_type === 'ai' ? 'badge-info' : 'badge-success'">
                    {{ message.sender_type === 'user' ? '我' : message.sender_type === 'ai' ? '历史记录' : message.sender_type === 'agent' ? '人工客服' : '系统' }}
                  </span>
                  <span class="quiet ml-auto">{{ when(message.created_at) }}</span>
                </div>
                <p class="mt-2 whitespace-pre-line text-sm">{{ message.content }}</p>
              </article>
              <div class="flex items-end gap-2">
                <textarea v-model="reply" class="input min-h-[70px]" placeholder="继续补充信息，客服会看到" />
                <button class="btn btn-primary btn-sm" :disabled="sendingReply || !reply.trim()" @click="sendReply">发送</button>
              </div>
            </div>

            <div v-if="['resolved','closed'].includes(detail.ticket.status)" class="card space-y-3">
              <p class="eyebrow">满意度评价</p>
              <div class="flex gap-1.5">
                <button
                  v-for="score in 5"
                  :key="score"
                  class="chip"
                  :class="rating === score ? 'chip-active' : ''"
                  @click="rating = score"
                >
                  {{ score }} 分
                </button>
              </div>
              <input v-model="ratingComment" class="input" placeholder="补充说明（可选）" />
              <button class="btn btn-primary btn-sm" @click="submitRating">提交评价</button>
            </div>
          </div>
        </div>
      </div>
    </section>

    <div v-if="showCreate" class="scrim fixed inset-0 z-50 grid place-items-center p-4" @click.self="showCreate = false">
      <div class="card w-full max-w-lg space-y-3">
        <h2 class="text-lg font-bold">提交工单</h2>
        <select v-model="form.type" class="input">
          <option v-for="option in ticketTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
        <input v-model="form.order_no" class="input" placeholder="关联订单号（可选）" />
        <input v-model="form.subject" class="input" placeholder="问题标题（必填）" />
        <textarea v-model="form.content" class="input min-h-[120px]" placeholder="详细描述你遇到的问题" />
        <p class="quiet text-xs">提交后客服会按顺序跟进，你可以在本页查看进度。</p>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showCreate = false">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="creating" @click="submitTicket">{{ creating ? '提交中…' : '提交' }}</button>
        </div>
      </div>
    </div>

    <div v-if="closingConfirm" class="scrim fixed inset-0 z-50 grid place-items-center p-4" @click.self="closingConfirm = false">
      <div class="card w-full max-w-md space-y-3">
        <h2 class="text-lg font-bold">关闭这张工单？</h2>
        <p class="text-sm leading-relaxed text-[var(--text-dim)]">
          关闭后可再次打开。如果问题还需要客服继续处理，建议先补充说明再关闭。
        </p>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="closingTicket" @click="closingConfirm = false">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="closingTicket" @click="closeTicket">
            <span v-if="closingTicket" class="spinner spinner-light" />
            {{ closingTicket ? '关闭中…' : '确认关闭' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.support-action {
  min-height: 78px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-md);
  background: var(--surface);
  padding: 14px 15px;
  transition: border-color var(--fast), background var(--fast), transform 200ms var(--spring);
}

.support-action:hover {
  border-color: var(--stroke-hi);
  background: var(--surface-hi);
  transform: translateY(-1px);
}

.support-ticket-list {
  max-height: 68vh;
  overflow-y: auto;
  padding-right: 2px;
}

.support-ticket-row {
  border: 1px solid var(--stroke);
  border-radius: var(--radius-sm);
  background: var(--surface);
  padding: 12px 13px;
  transition: border-color var(--fast), background var(--fast);
}

.support-ticket-row:hover {
  border-color: var(--stroke-hi);
  background: var(--surface-hi);
}

.support-ticket-row-active {
  border-color: var(--accent-line);
  background: var(--accent-soft);
}

.support-ticket-detail {
  position: sticky;
  top: 82px;
}

@media (max-width: 1279px) {
  .support-ticket-list {
    max-height: none;
  }

  .support-ticket-detail {
    position: static;
  }
}
</style>
