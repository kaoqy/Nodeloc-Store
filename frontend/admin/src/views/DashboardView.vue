<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { RouterLink } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { getStats } from '../api/system'
import {
  getTicketStats,
  listTickets,
  ticketPriorityLabels,
  ticketStatusLabels,
  type TicketStats,
  type TicketView,
} from '../api/support'
import type { DashboardStats } from '../types'
import { errorMessage, money, when, dayLabel } from '../utils/format'
import { useAuthStore } from '../stores/auth'

/**
 * 总览：按「先看待办，再看经营」的顺序组织。
 *
 * 店主打开后台的第一件事通常是「有没有要处理的」，所以工单与待办放在图表
 * 之前；销售趋势等分析性内容放在后面。所有卡片点进去都带着筛选条件落到
 * 对应列表，卡片上的数字与列表里的结果来自同一批参数。
 */

type Metric = 'revenue' | 'orders' | 'users'

const RANGES = [
  { days: 7, label: '近 7 天' },
  { days: 30, label: '近 30 天' },
  { days: 90, label: '近 90 天' },
]

const METRICS: Record<Metric, { label: string; unit: string; format: (v: number) => string }> = {
  revenue: { label: '收入', unit: '', format: money },
  orders: { label: '订单', unit: '单', format: (v) => String(v) },
  users: { label: '新客', unit: '人', format: (v) => String(v) },
}

const auth = useAuthStore()
const loading = ref(true)
const error = ref('')
const stats = ref<DashboardStats | null>(null)
const days = ref(30)
const metric = ref<Metric>('revenue')
const updatedAt = ref('')
const auto = ref(false)
const alerting = ref(false)
const alertNotice = ref('')
const alertFailed = ref(false)

const ticketStats = ref<TicketStats | null>(null)
const recentTickets = ref<TicketView[]>([])
const ticketsLoading = ref(false)
const ticketsError = ref('')
const showTodos = ref(false)

const series = computed(() => stats.value?.revenue_series ?? [])
const topProducts = computed(() => stats.value?.top_products ?? [])
const stockAlerts = computed(() => stats.value?.stock_alerts ?? [])
const recentOrders = computed(() => stats.value?.recent_orders ?? [])

const canSeeTickets = computed(() => auth.allows('tickets', 'view'))

// 待办：只列真的有事要做的项，数量为 0 的不出现，避免一屏都是「0」。
const todos = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { to: '/orders?status=pending', label: '待支付订单', count: s.orders_pending ?? 0, tone: 'warning', permission: ['orders', 'view'] as const },
    { to: '/orders?status=paid', label: '等待人工发货', count: s.orders_manual_pending ?? 0, tone: 'info', permission: ['orders', 'view'] as const },
    { to: '/cards', label: '等待补货', count: s.orders_waiting ?? 0, tone: 'danger', permission: ['cards', 'view'] as const },
    { to: '/service?status=pending_human', label: '待人工工单', count: s.tickets_pending_human ?? 0, tone: 'danger', permission: ['tickets', 'view'] as const },
    { to: '/service?attention=urgent', label: '紧急工单', count: s.tickets_urgent ?? 0, tone: 'danger', permission: ['tickets', 'view'] as const },
    { to: '/service?attention=overdue', label: '即将超时工单', count: s.tickets_overdue ?? 0, tone: 'warning', permission: ['tickets', 'view'] as const },
    { to: '/orders?attention=undelivered', label: '自动发货异常', count: s.auto_delivery_failed ?? 0, tone: 'danger', permission: ['orders', 'view'] as const },
  ].filter((item) => item.count > 0 && auth.allows(item.permission[0], item.permission[1]))
})

const backlogTotal = computed(() => todos.value.reduce((sum, item) => sum + item.count, 0))

// 图表：柱状图取当前指标的峰值做比例，折线用 7 日移动平均。
const bars = computed(() => {
  const key = metric.value
  const peak = Math.max(1, ...series.value.map((p) => p[key]))
  return series.value.map((p) => ({
    ...p,
    value: p[key],
    pct: p[key] ? Math.max((p[key] / peak) * 100, 3) : 0.8,
    peak,
  }))
})
const peakValue = computed(() => Math.max(1, ...series.value.map((p) => p[metric.value])))
const periodTotal = computed(() => series.value.reduce((sum, p) => sum + p[metric.value], 0))
const activeDays = computed(() => series.value.filter((p) => p[metric.value] > 0).length)
const dailyAverage = computed(() => (activeDays.value ? Math.round(periodTotal.value / activeDays.value) : 0))
const metricMeta = computed(() => METRICS[metric.value])
const bestDay = computed(() => {
  const key = metric.value
  let best: { date: string; value: number } | null = null
  for (const p of series.value) {
    if (p[key] > 0 && (!best || p[key] > best.value)) best = { date: p.date, value: p[key] }
  }
  return best
})

const categoryPeak = computed(() => Math.max(1, ...(stats.value?.category_sales ?? []).map((c) => c.revenue)))
const productPeak = computed(() => Math.max(1, ...topProducts.value.map((p) => p.revenue)))
const cardHealth = computed(() => stats.value?.card_health ?? null)

async function load(silent = false) {
  if (!silent) loading.value = true
  error.value = ''
  try {
    stats.value = await getStats(days.value)
    updatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  } catch (err) {
    error.value = errorMessage(err, '加载总览数据失败')
  } finally {
    loading.value = false
  }
}

async function loadTickets() {
  if (!canSeeTickets.value) return
  ticketsLoading.value = true
  ticketsError.value = ''
  try {
    const [metric2, page] = await Promise.all([
      getTicketStats(false).catch(() => null),
      listTickets({ limit: 6, offset: 0, attention: 'unread' }).catch(() => ({ data: [], total: 0 })),
    ])
    ticketStats.value = metric2
    const rows: TicketView[] = [...page.data]
    if (rows.length < 6) {
      const latest = await listTickets({ limit: 6, offset: 0 }).catch(() => ({ data: [], total: 0 }))
      const seen = new Set(rows.map((r) => r.id))
      for (const row of latest.data) {
        if (rows.length >= 6) break
        if (!seen.has(row.id)) { rows.push(row); seen.add(row.id) }
      }
    }
    recentTickets.value = rows
  } catch (err) {
    ticketsError.value = errorMessage(err, '工单数据加载失败')
  } finally {
    ticketsLoading.value = false
  }
}

async function refreshAll() {
  await Promise.all([load(), loadTickets()])
}

async function switchRange(value: number) {
  if (days.value === value) return
  days.value = value
  await load()
}

// 补货提醒：同一天对同一件商品只发一次，第二次点击会说明「今天已经提醒过」。
async function warnRestock() {
  if (alerting.value) return
  alerting.value = true
  alertNotice.value = ''
  try {
    const { alertLowStock } = await import('../api/products')
    const result = await alertLowStock()
    alertFailed.value = false
    if (!result.checked) alertNotice.value = '巡检没有需要提醒的商品，卡密都够用。'
    else if (result.sent) alertNotice.value = '已发出 ' + result.sent + ' 条提醒，覆盖 ' + result.checked + ' 件缺货商品。'
    else alertNotice.value = '今天已经提醒过 ' + result.checked + ' 件缺货商品了，明天同一时间会再说一次。'
  } catch (err) {
    alertFailed.value = true
    alertNotice.value = errorMessage(err, '提醒补货失败')
  } finally {
    alerting.value = false
  }
}

let timer: number | undefined
function syncAuto() {
  if (timer) window.clearInterval(timer)
  timer = undefined
  if (!auto.value) return
  timer = window.setInterval(() => {
    if (document.hidden || loading.value) return
    void load(true)
    void loadTickets()
  }, 60_000)
}
watch(auto, syncAuto)
function onVisible() {
  if (!document.hidden && auto.value && !loading.value) {
    void load(true)
    void loadTickets()
  }
}

onMounted(() => {
  void load()
  void loadTickets()
  document.addEventListener('visibilitychange', onVisible)
})
onUnmounted(() => {
  document.removeEventListener('visibilitychange', onVisible)
  if (timer) window.clearInterval(timer)
})
</script>

<template>
  <section class="space-y-5">
    <PageHeader
      title="总览"
      description="先看今天要处理的事，再看生意怎么样。卡片上的每个数字都可以点开对应的列表。"
      bordered
    >
      <template #actions>
        <div class="flex flex-wrap items-center gap-1.5">
          <button
            v-for="option in RANGES"
            :key="option.days"
            class="btn btn-sm"
            :class="days === option.days ? 'btn-primary' : 'btn-secondary'"
            :aria-pressed="days === option.days"
            @click="switchRange(option.days)"
          >
            {{ option.label }}
          </button>
          <button
            class="btn btn-quiet btn-sm"
            :class="auto ? 'accent-text' : ''"
            :aria-pressed="auto"
            title="开启后每 60 秒静默刷新一次"
            @click="auto = !auto"
          >
            <AdminIcon name="refresh" :size="14" />
            自动刷新 · {{ auto ? '开' : '关' }}
          </button>
          <button class="btn btn-quiet btn-sm" :disabled="loading || ticketsLoading" @click="refreshAll">
            <span v-if="loading || ticketsLoading" class="spinner !size-3.5" />
            {{ loading || ticketsLoading ? '刷新中…' : '刷新' }}
          </button>
          <span v-if="updatedAt" class="hint mono hidden sm:inline">更新于 {{ updatedAt }}</span>
        </div>
      </template>
    </PageHeader>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <!-- 待办：数量为 0 的项不出现；没有待办时给一句明确的「都清完了」 -->
    <div class="card space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div class="flex items-center gap-2">
          <p class="eyebrow">待办</p>
          <span v-if="backlogTotal" class="badge-warning nums">{{ backlogTotal }}</span>
          <span v-else class="badge-success">已清空</span>
        </div>
        <button
          v-if="todos.length > 4"
          class="hint"
          type="button"
          @click="showTodos = true"
        >
          查看全部 {{ todos.length }} 项 →
        </button>
      </div>

      <div v-if="!stats" class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <div v-for="i in 4" :key="i" class="skeleton h-16" />
      </div>
      <div v-else-if="!todos.length" class="card-quiet py-6 text-center text-sm text-[var(--text-quiet)]">
        目前没有待处理的事项，可以看看下面的经营数据。
      </div>
      <div v-else class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <RouterLink
          v-for="item in todos.slice(0, 4)"
          :key="item.to"
          :to="item.to"
          class="todo-card"
        >
          <span class="min-w-0 flex-1">
            <span class="quiet block truncate text-xs">{{ item.label }}</span>
            <span class="nums mt-1 block text-xl font-bold">{{ item.count }}</span>
          </span>
          <AdminIcon name="chevronRight" :size="15" class="shrink-0 text-[var(--text-quiet)]" />
        </RouterLink>
      </div>
    </div>

    <!-- 工单中心 -->
    <div v-if="canSeeTickets" class="card space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="eyebrow">工单中心</p>
          <p class="quiet mt-1 text-xs">AI 先接待，需要人工时在这里接手</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button class="btn btn-quiet btn-sm" :disabled="ticketsLoading" @click="loadTickets">
            <AdminIcon name="refresh" :size="14" />
            {{ ticketsLoading ? '刷新中…' : '刷新工单' }}
          </button>
          <RouterLink to="/service" class="btn btn-secondary btn-sm">查看全部工单</RouterLink>
        </div>
      </div>

      <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        <RouterLink to="/service?attention=unread" class="todo-card">
          <span class="min-w-0 flex-1">
            <span class="quiet block text-xs">未读工单</span>
            <span class="nums mt-1 block text-lg font-bold">{{ ticketStats?.unread ?? 0 }}</span>
          </span>
        </RouterLink>
        <RouterLink to="/service" class="todo-card">
          <span class="min-w-0 flex-1">
            <span class="quiet block text-xs">AI 处理中</span>
            <span class="nums mt-1 block text-lg font-bold">{{ ticketStats?.ai_processing ?? 0 }}</span>
          </span>
        </RouterLink>
        <RouterLink to="/service?status=pending_human" class="todo-card">
          <span class="min-w-0 flex-1">
            <span class="quiet block text-xs">待人工处理</span>
            <span class="nums accent-text mt-1 block text-lg font-bold">{{ ticketStats?.pending_human ?? 0 }}</span>
          </span>
        </RouterLink>
        <RouterLink to="/service?attention=overdue" class="todo-card">
          <span class="min-w-0 flex-1">
            <span class="quiet block text-xs">即将超时</span>
            <span class="nums mt-1 block text-lg font-bold">{{ ticketStats?.overdue ?? 0 }}</span>
          </span>
        </RouterLink>
      </div>

      <div v-if="ticketsError" class="alert alert-danger flex flex-wrap items-center gap-2" role="alert">
        {{ ticketsError }}
        <button class="btn btn-secondary btn-sm ml-auto" @click="loadTickets">重试</button>
      </div>

      <div v-if="ticketsLoading" class="space-y-2">
        <div v-for="i in 3" :key="i" class="skeleton h-11 w-full" />
      </div>
      <div v-else-if="!recentTickets.length" class="card-quiet py-6 text-center text-sm text-[var(--text-quiet)]">
        当前没有待处理的工单
      </div>
      <ul v-else class="space-y-2">
        <li v-for="ticket in recentTickets" :key="ticket.id">
          <RouterLink
            :to="'/service?q=' + encodeURIComponent(ticket.ticket_no)"
            class="card-quiet flex flex-wrap items-center gap-2 transition-colors hover:border-[var(--stroke-hi)]"
          >
            <span v-if="ticket.unread" class="badge-info">未读</span>
            <span class="min-w-0 flex-1 truncate text-sm font-semibold">{{ ticket.subject }}</span>
            <StatusBadge :value="ticket.status" :label="ticketStatusLabels[ticket.status]" />
            <StatusBadge :value="ticket.priority" :label="ticketPriorityLabels[ticket.priority]" />
            <span class="mono quiet text-xs">{{ ticket.ticket_no }}</span>
          </RouterLink>
        </li>
      </ul>
    </div>

    <!-- 快捷操作：店主每天要做的动作，按权限显示，点进去就是可用的页面 -->
    <div class="card space-y-3">
      <p class="eyebrow">快捷操作</p>
      <div class="flex flex-wrap gap-2">
        <RouterLink v-if="auth.allows('products', 'manage')" to="/products/new" class="btn btn-secondary btn-sm">
          <AdminIcon name="plus" :size="14" />
          新增商品
        </RouterLink>
        <RouterLink v-if="auth.allows('cards', 'manage')" to="/cards" class="btn btn-secondary btn-sm">
          <AdminIcon name="plus" :size="14" />
          导入卡密
        </RouterLink>
        <RouterLink v-if="auth.allows('activities', 'manage')" to="/activities/new" class="btn btn-secondary btn-sm">
          <AdminIcon name="plus" :size="14" />
          创建活动
        </RouterLink>
        <RouterLink v-if="auth.allows('orders', 'view')" to="/orders" class="btn btn-quiet btn-sm">全部订单</RouterLink>
        <RouterLink v-if="auth.allows('tickets', 'view')" to="/service" class="btn btn-quiet btn-sm">工单中心</RouterLink>
        <RouterLink v-if="auth.allows('ai', 'view')" to="/config?tab=ai" class="btn btn-quiet btn-sm">AI 客服配置</RouterLink>
        <RouterLink v-if="auth.allows('knowledge', 'view')" to="/config?tab=knowledge" class="btn btn-quiet btn-sm">知识库</RouterLink>
        <RouterLink v-if="auth.allows('settings', 'view')" to="/settings" class="btn btn-quiet btn-sm">系统设置</RouterLink>
      </div>
    </div>

    <!-- 核心指标卡 -->
    <div v-if="loading && !stats" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="i in 8" :key="i" class="card !p-5">
        <div class="skeleton h-3 w-16" />
        <div class="skeleton mt-3 h-7 w-24" />
      </div>
    </div>
    <div v-else-if="stats" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card !p-5">
        <p class="eyebrow">期间收入</p>
        <p class="nums mt-2 text-2xl font-bold">{{ money(stats.revenue_period) }}</p>
        <p class="hint mt-1.5">累计 {{ money(stats.revenue_total) }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">支付订单</p>
        <p class="nums mt-2 text-2xl font-bold">{{ stats.paid_period }}</p>
        <p class="hint mt-1.5">下单 {{ stats.orders_period }} 笔 · 转化 {{ Math.round(stats.conversion) }}%</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">客单价</p>
        <p class="nums mt-2 text-2xl font-bold">{{ money(stats.aov) }}</p>
        <p class="hint mt-1.5">已交付 {{ stats.delivered_period }} 笔 · 退款 {{ stats.refunded_period }} 笔</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">期间新客</p>
        <p class="nums mt-2 text-2xl font-bold">{{ stats.new_users_period }}</p>
        <p class="hint mt-1.5">购买 {{ stats.active_buyers_period }} 人 · 复购 {{ stats.repeat_buyers_period }} 人</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">AI 处理中工单</p>
        <p class="nums mt-2 text-2xl font-bold">{{ stats.tickets_ai_processing }}</p>
        <p class="hint mt-1.5">工单 {{ stats.tickets_total }} 张</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">工单解决率</p>
        <p class="nums mt-2 text-2xl font-bold">{{ Math.round((stats.ticket_resolve_rate ?? 0) * 100) }}%</p>
        <p class="hint mt-1.5">满意度 {{ (stats.ticket_satisfaction ?? 0).toFixed(1) }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">进行中活动</p>
        <p class="nums mt-2 text-2xl font-bold">{{ stats.activities_running }}</p>
        <p class="hint mt-1.5">参与 {{ stats.activity_participants }} 人</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">库存预警</p>
        <p class="nums mt-2 text-2xl font-bold">{{ stockAlerts.length }}</p>
        <p class="hint mt-1.5">自动发货异常 {{ stats.auto_delivery_failed }} 笔</p>
      </div>
    </div>

    <!-- 经营数据：趋势 -->
    <div v-if="stats" class="card space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="eyebrow">经营趋势</p>
          <p class="quiet mt-1 text-xs">按天统计，空白日期为 0</p>
        </div>
        <div class="flex flex-wrap gap-1.5">
          <button
            v-for="(meta, key) in METRICS"
            :key="key"
            class="chip"
            :class="metric === key ? 'chip-active' : ''"
            @click="metric = key as Metric"
          >
            {{ meta.label }}
          </button>
        </div>
      </div>

      <div class="grid gap-4 sm:grid-cols-3">
        <div class="card-quiet">
          <p class="quiet text-xs">期间合计</p>
          <p class="nums mt-1 text-lg font-bold">
            {{ metricMeta.unit === '' ? money(periodTotal) : periodTotal + ' ' + metricMeta.unit }}
          </p>
        </div>
        <div class="card-quiet">
          <p class="quiet text-xs">日均（有数据的天）</p>
          <p class="nums mt-1 text-lg font-bold">
            {{ metricMeta.unit === '' ? money(dailyAverage) : dailyAverage + ' ' + metricMeta.unit }}
          </p>
        </div>
        <div class="card-quiet">
          <p class="quiet text-xs">最好的一天</p>
          <p class="nums mt-1 text-lg font-bold">
            {{ bestDay ? dayLabel(bestDay.date) + ' · ' + metricMeta.format(bestDay.value) : '—' }}
          </p>
        </div>
      </div>

      <div class="chart" role="img" :aria-label="metricMeta.label + ' 趋势图'">
        <div v-for="bar in bars" :key="bar.date" class="chart-col" :title="dayLabel(bar.date) + '：' + metricMeta.format(bar.value)">
          <span class="chart-bar" :style="{ height: bar.pct + '%' }" />
        </div>
      </div>
      <p class="quiet nums flex justify-between text-[11px]">
        <span>{{ series.length ? dayLabel(series[0].date) : '' }}</span>
        <span>{{ series.length ? dayLabel(series[series.length - 1].date) : '' }}</span>
      </p>
    </div>

    <!-- 热销商品与库存预警 -->
    <div v-if="stats" class="grid gap-4 lg:grid-cols-2">
      <div class="card space-y-3">
        <p class="eyebrow">热销商品</p>
        <div v-if="!topProducts.length" class="quiet py-6 text-center text-sm">期间还没有成交</div>
        <ul v-else class="space-y-2.5">
          <li v-for="item in topProducts.slice(0, 6)" :key="item.product_id">
            <div class="flex items-center justify-between gap-3 text-sm">
              <RouterLink :to="'/products/' + item.product_id + '/edit'" class="min-w-0 truncate hover:accent-text">
                {{ item.name }}
              </RouterLink>
              <span class="nums shrink-0">{{ money(item.revenue) }}</span>
            </div>
            <div class="bar-track mt-1.5">
              <span class="bar-fill" :style="{ width: Math.max((item.revenue / productPeak) * 100, 3) + '%' }" />
            </div>
          </li>
        </ul>
      </div>

      <div class="card space-y-3">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <p class="eyebrow">库存预警</p>
          <button class="btn btn-quiet btn-sm" :disabled="alerting" @click="warnRestock">
            {{ alerting ? '发送中…' : '提醒补货' }}
          </button>
        </div>
        <p v-if="alertNotice" class="text-xs" :class="alertFailed ? 'text-[var(--danger)]' : 'quiet'">{{ alertNotice }}</p>
        <div v-if="!stockAlerts.length" class="quiet py-6 text-center text-sm">卡密充足，无需补货</div>
        <ul v-else class="space-y-2">
          <li v-for="item in stockAlerts" :key="item.product_id" class="card-quiet flex items-center gap-3">
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm font-semibold">{{ item.name }}</span>
              <span class="quiet text-xs">
                剩余 <span class="nums">{{ item.available }}</span>
                <span v-if="item.waiting"> · 已付待发 <span class="nums text-[var(--warning)]">{{ item.waiting }}</span></span>
              </span>
            </span>
            <RouterLink :to="'/cards/' + item.product_id" class="btn btn-secondary btn-sm">去补货</RouterLink>
          </li>
        </ul>
      </div>
    </div>

    <!-- 最近订单 -->
    <div v-if="stats" class="card space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <p class="eyebrow">最近订单</p>
        <RouterLink to="/orders" class="hint">全部订单 →</RouterLink>
      </div>
      <div v-if="!recentOrders.length" class="quiet py-6 text-center text-sm">还没有订单</div>
      <div v-else class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th>订单</th>
              <th>商品</th>
              <th>买家</th>
              <th class="nums">金额</th>
              <th>状态</th>
              <th class="text-right">时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="order in recentOrders" :key="order.order_no">
              <td>
                <RouterLink :to="'/orders/' + order.order_no" class="mono hover:accent-text">{{ order.order_no }}</RouterLink>
              </td>
              <td class="max-w-[220px] truncate">{{ order.product }}</td>
              <td>{{ order.buyer }}</td>
              <td class="nums">{{ money(order.amount) }}</td>
              <td><StatusBadge :value="order.status" /></td>
              <td class="quiet text-right text-xs">{{ when(order.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- 全部待办抽屉 -->
    <AppDrawer :open="showTodos" title="全部待办" width="sm" @close="showTodos = false">
      <ul class="space-y-2">
        <li v-for="item in todos" :key="item.to">
          <RouterLink :to="item.to" class="card-quiet flex items-center gap-3" @click="showTodos = false">
            <span class="min-w-0 flex-1 text-sm">{{ item.label }}</span>
            <span class="nums font-bold">{{ item.count }}</span>
            <AdminIcon name="chevronRight" :size="15" class="text-[var(--text-quiet)]" />
          </RouterLink>
        </li>
      </ul>
    </AppDrawer>
  </section>
</template>

<style scoped>
/* 待办卡：hover 时整块抬起来，暗示可点击 */
.todo-card {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
  padding: 12px 14px;
  transition: border-color var(--fast), transform var(--normal) var(--spring);
}
.todo-card:hover { border-color: var(--stroke-hi); transform: translateY(-1px); }

/* 柱状趋势图：用百分比高度，容器高度固定 */
.chart {
  display: flex;
  align-items: flex-end;
  gap: 2px;
  height: 180px;
  padding-top: 8px;
}
.chart-col {
  flex: 1;
  display: flex;
  align-items: flex-end;
  height: 100%;
  min-width: 2px;
}
.chart-bar {
  width: 100%;
  border-radius: var(--radius-xs) var(--radius-xs) 2px 2px;
  background: linear-gradient(to top, var(--accent), var(--accent-hi));
  opacity: 0.9;
  transition: opacity var(--fast);
}
.chart-col:hover .chart-bar { opacity: 1; }

.bar-track {
  height: 4px;
  border-radius: var(--radius-pill);
  background: var(--surface-hi);
  overflow: hidden;
}
.bar-fill {
  display: block;
  height: 100%;
  border-radius: var(--radius-pill);
  background: var(--accent);
}
</style>
