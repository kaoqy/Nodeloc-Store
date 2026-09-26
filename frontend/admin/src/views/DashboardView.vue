<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import StatCard from '../components/StatCard.vue'
import { listAuditLogs } from '../api/logs'
import { getStats } from '../api/system'
import { errorMessage, fulfillmentStatus, money, orderStatus, when, dayLabel } from '../utils/format'
import type { AuditLog, DashboardStats } from '../types'

type Metric = 'revenue' | 'orders' | 'users'

const RANGES = [
  { days: 7, label: '近 7 天' },
  { days: 30, label: '近 30 天' },
  { days: 90, label: '近 90 天' },
]

const METRICS: Record<Metric, { label: string; unit: string; format: (value: number) => string }> = {
  revenue: { label: '收入', unit: '', format: money },
  orders: { label: '订单', unit: '单', format: (value) => String(value) },
  users: { label: '新客', unit: '人', format: (value) => String(value) },
}

// Moving average window over the trend. Seven days smooths the weekday spikes a
// small store gets from batch promotions without hiding a real change of pace.
const AverageWindow = 7
const AutoRefreshMs = 60_000
const PrefsKey = 'admin.dashboard.view'

interface ViewPrefs {
  days: number
  metric: Metric
  auto: boolean
}

function readPrefs(): ViewPrefs {
  const fallback: ViewPrefs = { days: 30, metric: 'revenue', auto: false }
  try {
    const raw = localStorage.getItem(PrefsKey)
    if (!raw) return fallback
    const saved = JSON.parse(raw) as Partial<ViewPrefs>
    return {
      days: RANGES.some((range) => range.days === saved.days) ? saved.days! : fallback.days,
      metric: saved.metric && saved.metric in METRICS ? saved.metric : fallback.metric,
      auto: typeof saved.auto === 'boolean' ? saved.auto : fallback.auto,
    }
  } catch {
    return fallback
  }
}

const prefs = readPrefs()
const loading = ref(true)
const error = ref('')
const days = ref(prefs.days)
const metric = ref<Metric>(prefs.metric)
const auto = ref(prefs.auto)
const stats = ref<DashboardStats | null>(null)
const logs = ref<AuditLog[]>([])
const updatedAt = ref('')
let refreshTimer: number | undefined

const series = computed(() => stats.value?.revenue_series ?? [])
const topProducts = computed(() => stats.value?.top_products ?? [])
const topBuyers = computed(() => stats.value?.top_buyers ?? [])
const stockAlerts = computed(() => stats.value?.stock_alerts ?? [])
const funnel = computed(() => stats.value?.funnel ?? [])
const recentOrders = computed(() => stats.value?.recent_orders ?? [])
const onboarding = computed(() => Boolean(stats.value) && (stats.value?.orders_total ?? 0) === 0)

const onboardingSteps = [
  { to: '/products/new', label: '上架第一件商品', hint: '定价、描述并公开可见' },
  { to: '/cards', label: '导入卡密库存', hint: '卡密类商品付款后自动交付' },
  { to: '/settings', label: '核对支付与登录', hint: 'NodeLoc Payments 与 OAuth 回调' },
]

const bars = computed(() => {
  const key = metric.value
  const peak = Math.max(1, ...series.value.map((point) => point[key]))
  return series.value.map((point) => {
    const value = point[key]
    return { ...point, value, pct: value ? Math.max((value / peak) * 100, 3) : 0.8 }
  })
})

// Coordinates share the bars' viewBox scale, so the line sits on the columns
// instead of being a second, differently-scaled chart.
const averageLine = computed(() => {
  const key = metric.value
  if (series.value.length < 3) return ''
  const peak = Math.max(1, ...series.value.map((point) => point[key]))
  const step = 100 / (series.value.length - 1)
  return series.value
    .map((_, index) => {
      const window = series.value.slice(Math.max(0, index - AverageWindow + 1), index + 1)
      const mean = window.reduce((sum, point) => sum + point[key], 0) / window.length
      return `${(index * step).toFixed(2)},${(100 - (mean / peak) * 100).toFixed(2)}`
    })
    .join(' ')
})

const sparklines = computed(() => {
  const points = series.value
  return {
    revenue: points.map((point) => point.revenue),
    orders: points.map((point) => point.orders),
    users: points.map((point) => point.users),
    aov: points.map((point) => (point.orders ? point.revenue / point.orders : 0)),
  }
})

const bestDay = computed(() => {
  const key = metric.value
  let best = null as { date: string; value: number } | null
  for (const point of series.value) {
    if (point[key] > 0 && (!best || point[key] > best.value)) best = { date: point.date, value: point[key] }
  }
  return best
})

const periodTotal = computed(() => {
  const key = metric.value
  return series.value.reduce((sum, point) => sum + point[key], 0)
})

const activeDays = computed(() => series.value.filter((point) => point[metric.value] > 0).length)
const dailyAverage = computed(() => (activeDays.value ? Math.round(periodTotal.value / activeDays.value) : 0))
const metricFormat = computed(() => METRICS[metric.value].format)

const backlog = computed(() => {
  const s = stats.value
  if (!s) return []
  return [
    { to: '/orders?status=pending', label: '待支付订单', count: s.orders_pending, tone: 'warning' },
    { to: '/orders?status=paid', label: '等待人工发货', count: s.orders_manual_pending, tone: 'info' },
    { to: '/cards', label: '等待补货', count: s.orders_waiting, tone: 'danger' },
    { to: '/orders', label: '期间退款', count: s.refunded_period, tone: 'neutral' },
  ].filter((item) => item.count > 0)
})

const buyerPeak = computed(() => Math.max(1, ...topBuyers.value.map((item) => item.revenue)))
const funnelPeak = computed(() => Math.max(1, ...funnel.value.map((stage) => stage.count)))
const revenuePeak = computed(() => Math.max(1, ...topProducts.value.map((item) => item.revenue)))

async function load(silent = false) {
  if (!silent) loading.value = true
  error.value = ''
  try {
    const [statsResult, logResult] = await Promise.all([getStats(days.value), listAuditLogs({ page: 1, limit: 6 })])
    stats.value = statsResult
    logs.value = logResult.items
    updatedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  } catch (err) {
    error.value = errorMessage(err, '加载概览数据失败')
  } finally {
    loading.value = false
  }
}

async function switchRange(value: number) {
  if (days.value === value) return
  days.value = value
  await load()
}

function stopAutoRefresh() {
  if (refreshTimer) window.clearInterval(refreshTimer)
  refreshTimer = undefined
}

function syncAutoRefresh() {
  stopAutoRefresh()
  if (!auto.value) return
  // A hidden tab refreshing every minute only spends requests, so the tick is
  // skipped there and the data reloads as soon as the tab is visible again.
  refreshTimer = window.setInterval(() => {
    if (document.hidden || loading.value) return
    void load(true)
  }, AutoRefreshMs)
}

watch([days, metric, auto], () => {
  localStorage.setItem(PrefsKey, JSON.stringify({ days: days.value, metric: metric.value, auto: auto.value }))
  syncAutoRefresh()
})

function onVisible() {
  if (!document.hidden && auto.value && !loading.value) void load(true)
}

onMounted(() => {
  void load()
  syncAutoRefresh()
  document.addEventListener('visibilitychange', onVisible)
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', onVisible)
  stopAutoRefresh()
})
</script>

<template>
  <section class="space-y-5">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex gap-1.5">
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
      </div>
      <div class="flex items-center gap-3">
        <button
          class="btn btn-sm"
          :class="auto ? 'btn-secondary border-[var(--accent-line)] accent-text' : 'btn-quiet'"
          :aria-pressed="auto"
          title="开启后每 60 秒静默刷新一次"
          @click="auto = !auto"
        >
          自动刷新 · {{ auto ? '开' : '关' }}
        </button>
        <span v-if="updatedAt" class="hint mono">更新于 {{ updatedAt }}</span>
        <button class="btn btn-quiet btn-sm" :disabled="loading" @click="load()">
          {{ loading ? '加载中…' : '刷新' }}
        </button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div v-if="loading && !stats" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="i in 4" :key="i" class="card !p-5">
        <div class="skeleton h-3 w-16" />
        <div class="skeleton mt-4 h-7 w-24" />
        <div class="skeleton mt-3 h-3 w-32" />
      </div>
    </div>

    <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="期间收入"
        :value="money(stats?.revenue_period ?? 0)"
        :hint="`累计 ${money(stats?.revenue_total ?? 0)} · 上期 ${money(stats?.revenue_prev ?? 0)}`"
        :delta="(stats?.revenue_prev ?? 0) > 0 ? (stats?.revenue_delta ?? null) : null"
        :spark="sparklines.revenue"
        accent
      />
      <StatCard
        label="支付订单"
        :value="stats?.paid_period ?? 0"
        :hint="`下单 ${stats?.orders_period ?? 0} 笔 · 转化率 ${Math.round(stats?.conversion ?? 0)}%`"
        :delta="(stats?.paid_prev ?? 0) > 0 ? (stats?.paid_delta ?? null) : null"
        :spark="sparklines.orders"
      />
      <StatCard
        label="客单价"
        :value="money(stats?.aov ?? 0)"
        :hint="`已交付 ${stats?.delivered_period ?? 0} 笔 · 退款 ${stats?.refunded_period ?? 0} 笔`"
        :spark="sparklines.aov"
      />
      <StatCard
        label="期间新客"
        :value="stats?.new_users_period ?? 0"
        :hint="`购买用户 ${stats?.active_buyers_period ?? 0} 人 · 复购 ${stats?.repeat_buyers_period ?? 0} 人`"
        :delta="(stats?.new_users_prev ?? 0) > 0 ? (stats?.new_users_delta ?? null) : null"
        :spark="sparklines.users"
      />
    </div>

    <div v-if="!loading && onboarding" class="card">
      <h2 class="text-base font-semibold">开张三件事</h2>
      <p class="hint mt-1">还没有任何订单。按下面顺序把店铺跑起来，这一步跑完看板就会有数据。</p>
      <ol class="mt-4 grid gap-3 sm:grid-cols-3">
        <li v-for="(step, index) in onboardingSteps" :key="step.to">
          <RouterLink
            :to="step.to"
            class="flex h-full flex-col gap-1.5 rounded-md border border-[var(--stroke-quiet)] px-4 py-3.5 transition-colors hover:border-[var(--accent-line)]"
          >
            <span class="mono quiet text-xs">{{ String(index + 1).padStart(2, '0') }}</span>
            <span class="text-[13px] font-semibold">{{ step.label }}</span>
            <span class="hint">{{ step.hint }}</span>
          </RouterLink>
        </li>
      </ol>
    </div>

    <div v-if="!loading && backlog.length" class="card !p-4">
      <div class="flex flex-wrap items-center gap-2.5">
        <span class="eyebrow shrink-0">需要处理</span>
        <RouterLink
          v-for="item in backlog"
          :key="item.label"
          :to="item.to"
          class="badge"
          :class="`badge-${item.tone}`"
        >
          {{ item.label }} <span class="nums font-semibold">{{ item.count }}</span>
        </RouterLink>
      </div>
    </div>

    <div class="grid gap-5 xl:grid-cols-3">
      <div class="card xl:col-span-2">
        <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold">{{ METRICS[metric].label }}趋势</h2>
            <p class="quiet mt-1 text-xs">
              期间合计 {{ metricFormat(periodTotal) }}{{ METRICS[metric].unit }} · 活跃 {{ activeDays }} 天 · 日均
              {{ metricFormat(dailyAverage) }}{{ METRICS[metric].unit }}
              <span v-if="averageLine" class="mono"> · <span class="accent-text">┅</span> {{ AverageWindow }} 日均线</span>
            </p>
          </div>
          <div class="flex gap-1.5">
            <button
              v-for="option in (['revenue', 'orders', 'users'] as Metric[])"
              :key="option"
              class="btn btn-sm"
              :class="metric === option ? 'btn-secondary border-[var(--accent-line)] accent-text' : 'btn-quiet'"
              :aria-pressed="metric === option"
              @click="metric = option"
            >
              {{ METRICS[option].label }}
            </button>
          </div>
        </div>

        <div v-if="loading" class="skeleton h-56" />
        <div v-else-if="!bars.length" class="py-16 text-center text-sm quiet">暂无数据</div>
        <div v-else>
          <div class="relative h-56">
            <div class="absolute inset-0 flex flex-col justify-between" aria-hidden="true">
              <span v-for="i in 4" :key="i" class="block border-t border-dashed border-[var(--stroke-quiet)]" />
            </div>
            <div class="relative flex h-full items-end gap-[3px]" role="img" :aria-label="`${METRICS[metric].label}趋势，共 ${series.length} 天，含 ${AverageWindow} 日均线`">
              <div
                v-for="point in bars"
                :key="point.date"
                class="group relative flex h-full flex-1 items-end"
                :title="`${point.date} · ${metricFormat(point.value)}${METRICS[metric].unit}`"
              >
                <div
                  class="w-full rounded-t-sm border border-b-0 transition-colors"
                  :class="
                    point.value
                      ? 'border-[var(--accent-line)] bg-[var(--accent-soft)] group-hover:bg-[var(--accent-line)]'
                      : 'border-[var(--stroke-quiet)] bg-[var(--surface-sunken)]'
                  "
                  :style="{ height: `${point.pct}%` }"
                />
                <div
                  class="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1.5 hidden -translate-x-1/2 whitespace-nowrap rounded-md border border-[var(--stroke)] bg-[var(--surface-hi)] px-2 py-1.5 text-[11px] shadow-lg group-hover:block"
                >
                  <span class="mono quiet">{{ point.date }}</span>
                  <span class="nums font-semibold"> {{ metricFormat(point.value) }}{{ METRICS[metric].unit }}</span>
                  <span v-if="metric !== 'orders'" class="quiet"> · {{ point.orders }} 单</span>
                </div>
              </div>
            </div>
            <svg
              v-if="averageLine"
              class="pointer-events-none absolute inset-0 h-full w-full"
              viewBox="0 0 100 100"
              preserveAspectRatio="none"
              aria-hidden="true"
              focusable="false"
            >
              <polyline
                :points="averageLine"
                fill="none"
                stroke="var(--accent)"
                stroke-width="1.5"
                stroke-dasharray="4 3"
                stroke-linejoin="round"
                vector-effect="non-scaling-stroke"
                opacity="0.85"
              />
            </svg>
          </div>
          <div class="mt-3 flex justify-between text-[11px] quiet mono">
            <span>{{ dayLabel(series[0].date) }}</span>
            <span v-if="bestDay" class="nums">
              峰值 {{ dayLabel(bestDay.date) }} · {{ metricFormat(bestDay.value) }}{{ METRICS[metric].unit }}
            </span>
            <span>{{ dayLabel(series[series.length - 1].date) }}</span>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold">订单结构</h2>
          <RouterLink to="/orders" class="text-sm accent-text">订单管理</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 4" :key="i" class="skeleton h-9" />
        </div>
        <div v-else-if="!funnel.length" class="py-12 text-center text-sm quiet">期间内还没有订单</div>
        <ul v-else class="space-y-3">
          <li v-for="stage in funnel" :key="stage.key">
            <div class="flex items-baseline justify-between gap-3 text-xs">
              <span class="muted">{{ stage.label }}</span>
              <span class="nums font-semibold">{{ stage.count }}</span>
            </div>
            <div class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-[var(--surface-sunken)]">
              <div
                class="h-full rounded-full"
                :class="stage.key === 'refunded' || stage.key === 'failed' ? 'bg-[var(--danger)]' : 'bg-[var(--accent-line)]'"
                :style="{ width: `${Math.max((stage.count / funnelPeak) * 100, 2)}%` }"
              />
            </div>
          </li>
        </ul>

        <div class="my-4 divider" />

        <div class="grid grid-cols-2 gap-3 text-center">
          <div class="panel">
            <p class="hint">在售商品</p>
            <p class="nums mt-1 text-lg font-bold">{{ stats?.products_total ?? 0 }}</p>
          </div>
          <div class="panel">
            <p class="hint">注册用户</p>
            <p class="nums mt-1 text-lg font-bold">{{ stats?.users_total ?? 0 }}</p>
          </div>
        </div>
      </div>
    </div>

    <div class="grid gap-5 md:grid-cols-2 xl:grid-cols-4">
      <div class="card">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold">热销商品</h2>
          <RouterLink to="/products" class="text-sm accent-text">商品管理</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 4" :key="i" class="skeleton h-10" />
        </div>
        <div v-else-if="!topProducts.length" class="py-12 text-center text-sm quiet">期间内还没有成交</div>
        <ol v-else class="space-y-3.5">
          <li v-for="(item, index) in topProducts" :key="item.product_id">
            <div class="flex items-baseline justify-between gap-3">
              <p class="min-w-0 truncate text-[13px]">
                <span class="mono quiet mr-1.5">{{ String(index + 1).padStart(2, '0') }}</span>
                {{ item.name || `#${item.product_id}` }}
              </p>
              <span class="nums shrink-0 text-[13px] font-semibold">{{ money(item.revenue) }}</span>
            </div>
            <div class="mt-1.5 flex items-center gap-2">
              <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--surface-sunken)]">
                <div
                  class="h-full rounded-full border border-[var(--accent-line)] bg-[var(--accent-soft)]"
                  :style="{ width: `${Math.max((item.revenue / revenuePeak) * 100, 2)}%` }"
                />
              </div>
              <span class="hint nums shrink-0">{{ item.orders }} 单</span>
            </div>
          </li>
        </ol>
      </div>

      <div class="card">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold">买家排行</h2>
          <RouterLink to="/users" class="text-sm accent-text">用户管理</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 4" :key="i" class="skeleton h-10" />
        </div>
        <div v-else-if="!topBuyers.length" class="py-12 text-center text-sm quiet">期间内还没有成交买家</div>
        <ol v-else class="space-y-3.5">
          <li v-for="(item, index) in topBuyers" :key="item.user_id">
            <RouterLink
              :to="`/users/${item.user_id}`"
              class="flex items-baseline justify-between gap-3 transition-colors hover:underline"
            >
              <p class="min-w-0 truncate text-[13px]">
                <span class="mono quiet mr-1.5">{{ String(index + 1).padStart(2, '0') }}</span>
                {{ item.name || `#${item.user_id}` }}
              </p>
              <span class="nums shrink-0 text-[13px] font-semibold">{{ money(item.revenue) }}</span>
            </RouterLink>
            <div class="mt-1.5 flex items-center gap-2">
              <div class="h-1.5 flex-1 overflow-hidden rounded-full bg-[var(--surface-sunken)]">
                <div
                  class="h-full rounded-full border border-[var(--accent-line)] bg-[var(--accent-soft)]"
                  :style="{ width: `${Math.max((item.revenue / buyerPeak) * 100, 2)}%` }"
                />
              </div>
              <span class="hint nums shrink-0">{{ item.orders }} 单</span>
            </div>
          </li>
        </ol>
      </div>

      <div class="card">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold">库存预警</h2>
          <RouterLink to="/cards" class="text-sm accent-text">卡密库存</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 4" :key="i" class="skeleton h-10" />
        </div>
        <p v-else-if="!stockAlerts.length" class="py-12 text-center text-sm quiet">卡密充足，无需补货</p>
        <ul v-else class="space-y-2.5">
          <li v-for="item in stockAlerts" :key="item.product_id">
            <RouterLink
              :to="`/cards/${item.product_id}`"
              class="flex items-center justify-between gap-3 rounded-md border border-[var(--stroke-quiet)] px-3 py-2.5 transition-colors hover:border-[var(--stroke-hi)]"
            >
              <span class="min-w-0">
                <span class="block truncate text-[13px]">{{ item.name || `#${item.product_id}` }}</span>
                <span class="hint nums block">已售 {{ item.sold }} 张</span>
              </span>
              <span class="badge shrink-0" :class="item.available ? 'badge-warning' : 'badge-danger'">
                余 {{ item.available }}
              </span>
            </RouterLink>
          </li>
        </ul>
      </div>

      <div class="card">
        <div class="mb-4 flex items-center justify-between gap-3">
          <h2 class="text-base font-semibold">近期操作</h2>
          <RouterLink to="/logs" class="text-sm accent-text">审计日志</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="skeleton h-10" />
        </div>
        <div v-else-if="!logs.length" class="py-12 text-center text-sm quiet">暂无日志</div>
        <div v-else class="space-y-3.5">
          <div v-for="log in logs" :key="log.id" class="border-l-2 border-[var(--accent-line)] pl-3">
            <p class="mono text-xs">{{ log.action }}</p>
            <p class="quiet mt-1 truncate text-xs">{{ log.target || '—' }} · {{ when(log.created_at) }}</p>
          </div>
        </div>
      </div>
    </div>

    <div class="card !p-0 overflow-hidden">
      <div class="flex items-center justify-between gap-3 px-5 py-4">
        <div>
          <h2 class="text-base font-semibold">最新订单</h2>
          <p class="hint mt-0.5">共 {{ stats?.orders_total ?? 0 }} 笔订单 · 可用卡密 {{ stats?.cards_available ?? 0 }} 张</p>
        </div>
        <RouterLink to="/orders" class="text-sm accent-text">查看全部</RouterLink>
      </div>
      <div v-if="loading" class="space-y-2 px-5 pb-5">
        <div v-for="i in 4" :key="i" class="skeleton h-11" />
      </div>
      <p v-else-if="!recentOrders.length" class="px-5 pb-8 text-center text-sm quiet">暂无订单</p>
      <div v-else class="table overflow-x-auto border-t border-[var(--stroke-quiet)]">
        <table>
          <thead>
            <tr>
              <th>订单号</th>
              <th>用户</th>
              <th>商品</th>
              <th>金额</th>
              <th>状态</th>
              <th>发货</th>
              <th>创建时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="order in recentOrders" :key="order.order_no">
              <td>
                <RouterLink :to="`/orders/${order.order_no}`" class="mono text-sm accent-text">
                  {{ order.order_no }}
                </RouterLink>
              </td>
              <td class="text-sm">{{ order.buyer || '—' }}</td>
              <td class="max-w-[220px] truncate text-sm">{{ order.product || '—' }}</td>
              <td class="nums text-sm">{{ money(order.amount) }}</td>
              <td>
                <span class="badge" :class="orderStatus(order.status).badge">{{ orderStatus(order.status).label }}</span>
              </td>
              <td>
                <span
                  class="badge"
                  :class="fulfillmentStatus(order.fulfillment_status, order.status).badge"
                >
                  {{ fulfillmentStatus(order.fulfillment_status, order.status).label }}
                </span>
              </td>
              <td class="text-sm quiet">{{ when(order.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
