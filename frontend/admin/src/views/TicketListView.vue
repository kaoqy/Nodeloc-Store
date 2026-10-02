<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import {
  getTicketStats,
  listTickets,
  ticketPriorityLabels,
  ticketStatusLabels,
  ticketTypeOptions,
  type TicketStats,
  type TicketView,
} from '../api/support'
import { errorMessage, when } from '../utils/format'

const PageSize = 15

const loading = ref(true)
const error = ref('')
const rows = ref<TicketView[]>([])
const total = ref(0)
const page = ref(1)
const search = ref('')
const status = ref('all')
const priority = ref('all')
const type = ref('all')
const attention = ref('')
const mine = ref(false)
const stats = ref<TicketStats | null>(null)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))

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

const priorityTone: Record<string, string> = {
  low: 'badge',
  normal: 'badge',
  high: 'badge-warning',
  urgent: 'badge-danger',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, metric] = await Promise.all([
      listTickets({
        status: status.value,
        priority: priority.value,
        type: type.value,
        attention: attention.value,
        mine: mine.value,
        q: search.value,
        limit: PageSize,
        offset: (page.value - 1) * PageSize,
      }),
      getTicketStats(mine.value).catch(() => null),
    ])
    rows.value = list.data
    total.value = list.total
    stats.value = metric
  } catch (err) {
    error.value = errorMessage(err, '加载工单失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

function quickView(key: string) {
  attention.value = key
  status.value = 'all'
  page.value = 1
  void load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card !p-5">
        <p class="eyebrow">AI 处理中</p>
        <p class="nums mt-2 text-2xl font-bold">{{ stats?.ai_processing ?? 0 }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">待人工处理</p>
        <p class="nums mt-2 text-2xl font-bold accent-text">{{ stats?.pending_human ?? 0 }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">解决率</p>
        <p class="nums mt-2 text-2xl font-bold">
          {{ stats ? (stats.resolve_rate * 100).toFixed(1) : '0.0' }}%
        </p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">满意度</p>
        <p class="nums mt-2 text-2xl font-bold">
          {{ stats ? stats.satisfaction_avg.toFixed(1) : '—' }}
        </p>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <button class="chip" :class="attention === '' ? 'chip-active' : ''" @click="quickView('')">全部</button>
      <button class="chip" :class="attention === 'unread' ? 'chip-active' : ''" @click="quickView('unread')">
        未读 <span class="nums">{{ stats?.unread ?? 0 }}</span>
      </button>
      <button class="chip" :class="attention === 'overdue' ? 'chip-active' : ''" @click="quickView('overdue')">
        即将超时 <span class="nums">{{ stats?.overdue ?? 0 }}</span>
      </button>
      <button class="chip" :class="attention === 'urgent' ? 'chip-active' : ''" @click="quickView('urgent')">
        紧急 <span class="nums">{{ stats?.urgent ?? 0 }}</span>
      </button>
      <button class="chip" :class="attention === 'refund' ? 'chip-active' : ''" @click="quickView('refund')">退款相关</button>
      <button class="chip" :class="attention === 'card' ? 'chip-active' : ''" @click="quickView('card')">卡密相关</button>
      <button class="chip" :class="attention === 'payment' ? 'chip-active' : ''" @click="quickView('payment')">支付相关</button>
      <button class="chip" :class="mine ? 'chip-active' : ''" @click="mine = !mine; applyFilters()">我的工单</button>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <input
        v-model="search"
        class="input w-56"
        type="search"
        placeholder="工单号 / 标题 / 订单号"
        aria-label="搜索工单"
        @keyup.enter="applyFilters"
      />
      <select v-model="status" class="input !w-auto" aria-label="工单状态" @change="applyFilters">
        <option value="all">全部状态</option>
        <option v-for="(label, value) in ticketStatusLabels" :key="value" :value="value">{{ label }}</option>
      </select>
      <select v-model="priority" class="input !w-auto" aria-label="优先级" @change="applyFilters">
        <option value="all">全部优先级</option>
        <option v-for="(label, value) in ticketPriorityLabels" :key="value" :value="value">{{ label }}</option>
      </select>
      <select v-model="type" class="input !w-auto" aria-label="工单类型" @change="applyFilters">
        <option value="all">全部类型</option>
        <option v-for="option in ticketTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="applyFilters">查询</button>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
    </div>

    <div v-else-if="!rows.length" class="card py-16 text-center">
      <p class="font-semibold">没有符合条件的工单</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">换个筛选条件，或等待买家提交新问题。</p>
    </div>

    <div v-else class="table-container">
      <table class="table">
        <thead>
          <tr>
            <th>工单</th>
            <th>用户</th>
            <th>状态</th>
            <th>处理方</th>
            <th>优先级</th>
            <th>客服</th>
            <th>最近消息</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.id">
            <td>
              <RouterLink :to="'/tickets/' + row.id" class="font-semibold hover:accent-text">
                {{ row.subject }}
              </RouterLink>
              <p class="mono quiet mt-0.5 text-xs">
                {{ row.ticket_no }}
                <span v-if="row.order_no"> · {{ row.order_no }}</span>
              </p>
            </td>
            <td class="text-xs">{{ row.username || '#' + row.user_id }}</td>
            <td>
              <span :class="statusTone[row.status] || 'badge'">{{ ticketStatusLabels[row.status] || row.status }}</span>
              <span v-if="row.overdue" class="badge-danger ml-1">超时</span>
            </td>
            <td class="text-xs">{{ row.handler === 'ai' ? 'AI' : row.handler === 'human' ? '人工' : '系统' }}</td>
            <td><span :class="priorityTone[row.priority] || 'badge'">{{ ticketPriorityLabels[row.priority] || row.priority }}</span></td>
            <td class="quiet text-xs">{{ row.agent_name || '未分配' }}</td>
            <td class="quiet text-xs">{{ when(row.last_message_at || row.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <PaginationFooter
      :page="page"
      :pages="pageCount"
      :loading="loading"
      :summary="'共 ' + total + ' 张工单'"
      @change="goPage"
    />
  </section>
</template>
