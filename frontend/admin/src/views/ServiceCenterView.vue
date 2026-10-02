<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
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
import PaginationFooter from '../components/PaginationFooter.vue'
import TicketDetailPanel from './support/TicketDetailPanel.vue'

// 客服中心把「工单处理」与「AI 客服配置」放在同一个工作台里。
// 买家的一条问题从 AI 接待到人工接手都发生在这条流水线上，管理员不必在
// 两个一级菜单之间来回切换才能看懂一张工单现在到底卡在哪一步。
const PageSize = 15
const route = useRoute()
const router = useRouter()

const tab = ref<'queue' | 'config'>('queue')
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
const activeTicketID = ref(0)
const panelKey = ref(0)

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
    if (!activeTicketID.value && rows.value.length) {
      activeTicketID.value = rows.value[0].id
    }
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

function openTicket(id: number) {
  activeTicketID.value = id
  panelKey.value += 1
}

function onPanelClosed() {
  void load()
}

// 侧栏的「AI 客服」入口通过 ?tab=config 落到本页的配置页签，
// 旧地址仍然可用，避免已经把链接发出去了的管理员点进来看到空页面。
function syncTabFromRoute() {
  if (route.query.tab === 'config') tab.value = 'config'
  // 总览的待办卡片会带 attention 过来，直接落到对应的快速筛选上。
  const incoming = typeof route.query.attention === 'string' ? route.query.attention : ''
  if (incoming) attention.value = incoming
}

watch(() => route.query.tab, syncTabFromRoute)
watch(() => route.query.attention, syncTabFromRoute)

async function switchTab(next: 'queue' | 'config') {
  tab.value = next
  const query = next === 'config' ? { tab: 'config' } : {}
  await router.replace({ path: route.path, query })
}

onMounted(() => {
  syncTabFromRoute()
  void load()
})
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-bold">客服中心</h2>
        <p class="quiet mt-1 text-xs">AI 先接待、需要时转人工；工单队列与 AI 配置在同一处维护。</p>
      </div>
      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'queue' ? 'chip-active' : ''" @click="switchTab('queue')">工单处理</button>
        <button class="chip" :class="tab === 'config' ? 'chip-active' : ''" @click="switchTab('config')">AI 与客服配置</button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <template v-if="tab === 'queue'">
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
          <p class="nums mt-2 text-2xl font-bold">{{ stats ? (stats.resolve_rate * 100).toFixed(1) : '0.0' }}%</p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">满意度</p>
          <p class="nums mt-2 text-2xl font-bold">{{ stats ? stats.satisfaction_avg.toFixed(1) : '—' }}</p>
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
        <button class="chip" :class="attention === 'refund' ? 'chip-active' : ''" @click="quickView('refund')">退款</button>
        <button class="chip" :class="attention === 'card' ? 'chip-active' : ''" @click="quickView('card')">卡密</button>
        <button class="chip" :class="attention === 'payment' ? 'chip-active' : ''" @click="quickView('payment')">支付</button>
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

      <div class="grid gap-4 xl:grid-cols-[minmax(0,380px)_minmax(0,1fr)]">
        <div class="space-y-3">
          <div v-if="loading" class="card space-y-3">
            <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
          </div>
          <div v-else-if="!rows.length" class="card py-16 text-center">
            <p class="font-semibold">没有符合条件的工单</p>
            <p class="mt-1.5 text-sm text-[var(--text-quiet)]">换个筛选条件，或等待买家提交新问题。</p>
          </div>
          <template v-else>
            <button
              v-for="row in rows"
              :key="row.id"
              class="card-quiet w-full text-left transition-colors"
              :class="activeTicketID === row.id ? 'border-[var(--accent-line)]' : ''"
              @click="openTicket(row.id)"
            >
              <div class="flex items-start gap-2">
                <p class="min-w-0 flex-1 truncate text-sm font-semibold">{{ row.subject }}</p>
                <span v-if="row.unread" class="badge-info shrink-0">未读</span>
              </div>
              <p class="mono quiet mt-1 text-xs">
                {{ row.ticket_no }} · {{ row.username || '#' + row.user_id }}
              </p>
              <div class="mt-1.5 flex flex-wrap items-center gap-1.5 text-xs">
                <span :class="statusTone[row.status] || 'badge'">{{ ticketStatusLabels[row.status] || row.status }}</span>
                <span :class="priorityTone[row.priority] || 'badge'">{{ ticketPriorityLabels[row.priority] || row.priority }}</span>
                <span v-if="row.handler === 'ai'" class="badge-info">AI</span>
                <span v-else class="badge-success">人工</span>
                <span class="quiet ml-auto">{{ when(row.last_message_at || row.created_at) }}</span>
              </div>
            </button>
            <PaginationFooter
              :page="page"
              :pages="pageCount"
              :loading="loading"
              :summary="'共 ' + total + ' 张工单'"
              @change="goPage"
            />
          </template>
        </div>

        <TicketDetailPanel
          v-if="activeTicketID"
          :key="activeTicketID + '-' + panelKey"
          :ticket-id="activeTicketID"
          @closed="onPanelClosed"
        />
        <div v-else class="card py-16 text-center text-sm text-[var(--text-quiet)]">选择左侧工单开始处理</div>
      </div>
    </template>

    <div v-else class="card py-16 text-center text-sm text-[var(--text-quiet)]">
      AI 与客服配置已移入「配置中心」，侧栏可直接打开。
      <RouterLink to="/config?tab=service" class="btn btn-secondary btn-sm ml-3">前往配置中心</RouterLink>
    </div>
  </section>
</template>
