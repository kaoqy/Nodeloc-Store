<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import StatCard from '../components/StatCard.vue'
import { listOrders } from '../api/orders'
import { listAuditLogs } from '../api/logs'
import { getStats } from '../api/system'
import { errorMessage, money, orderStatus, when, dayLabel } from '../utils/format'
import type { AuditLog, DashboardStats, Order } from '../types'

const loading = ref(true)
const error = ref('')
const days = ref(30)
const stats = ref<DashboardStats | null>(null)
const orders = ref<Order[]>([])
const logs = ref<AuditLog[]>([])

const series = computed(() => stats.value?.revenue_series ?? [])
const peak = computed(() => Math.max(1, ...series.value.map((point) => point.revenue)))
const activeDays = computed(() => series.value.filter((point) => point.revenue > 0).length)
const averageRevenue = computed(() =>
  activeDays.value ? Math.round((stats.value?.revenue_period ?? 0) / activeDays.value) : 0,
)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [statsResult, orderResult, logResult] = await Promise.all([
      getStats(days.value),
      listOrders({ limit: 6, offset: 0 }),
      listAuditLogs({ page: 1, limit: 6 }),
    ])
    stats.value = statsResult
    orders.value = orderResult.data
    logs.value = logResult.items
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

onMounted(load)
</script>

<template>
  <section class="space-y-5">
    <div v-if="error" class="alert alert-danger">{{ error }}</div>

    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <StatCard
        label="期间收入"
        :value="money(stats?.revenue_period ?? 0)"
        :hint="`近 ${days} 天 · 累计 ${money(stats?.revenue_total ?? 0)}`"
        accent
      />
      <StatCard label="订单总数" :value="stats?.orders_total ?? 0" :hint="`待支付 ${stats?.orders_pending ?? 0} 笔`" />
      <StatCard
        label="可用卡密"
        :value="stats?.cards_available ?? 0"
        :hint="`等待补货订单 ${stats?.orders_waiting ?? 0} 笔`"
      />
      <StatCard label="注册用户" :value="stats?.users_total ?? 0" :hint="`在售商品 ${stats?.products_total ?? 0} 个`" />
    </div>

    <div class="grid gap-5 xl:grid-cols-3">
      <div class="card xl:col-span-2">
        <div class="mb-5 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h2 class="text-base font-semibold">收入趋势</h2>
            <p class="quiet mt-1 text-xs">
              有成交 {{ activeDays }} 天 · 日均 {{ money(averageRevenue) }}
            </p>
          </div>
          <div class="flex gap-2">
            <button
              v-for="option in [{ days: 7, label: '近 7 天' }, { days: 30, label: '近 30 天' }]"
              :key="option.days"
              class="btn btn-sm"
              :class="days === option.days ? 'btn-primary' : 'btn-secondary'"
              @click="switchRange(option.days)"
            >
              {{ option.label }}
            </button>
          </div>
        </div>

        <div v-if="loading" class="skeleton h-52" />
        <div v-else-if="!series.length" class="py-16 text-center text-sm quiet">暂无数据</div>
        <div v-else>
          <div class="flex h-52 items-end gap-1.5">
            <div
              v-for="point in series"
              :key="point.date"
              class="group relative flex-1 rounded-t-sm bg-[var(--accent-soft)] transition-colors hover:bg-[var(--accent-line)]"
              :class="point.revenue ? 'border border-b-0 border-[var(--accent-line)]' : 'border border-b-0 border-[var(--stroke-quiet)]'"
              :style="{ height: `${Math.max((point.revenue / peak) * 100, 2)}%` }"
              :title="`${point.date} · ${money(point.revenue)} · ${point.orders} 单`"
            />
          </div>
          <div class="mt-3 flex justify-between text-[11px] quiet mono">
            <span>{{ dayLabel(series[0].date) }}</span>
            <span>{{ dayLabel(series[Math.floor(series.length / 2)].date) }}</span>
            <span>{{ dayLabel(series[series.length - 1].date) }}</span>
          </div>
        </div>
      </div>

      <div class="card">
        <div class="mb-4 flex items-center justify-between">
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
      <div class="flex items-center justify-between px-5 py-4">
        <h2 class="text-base font-semibold">最新订单</h2>
        <RouterLink to="/orders" class="text-sm accent-text">查看全部</RouterLink>
      </div>
      <div v-if="loading" class="space-y-2 px-5 pb-5">
        <div v-for="i in 4" :key="i" class="skeleton h-11" />
      </div>
      <p v-else-if="!orders.length" class="px-5 pb-8 text-center text-sm quiet">暂无订单</p>
      <div v-else class="table-container">
        <table>
          <thead>
            <tr>
              <th>订单号</th>
              <th>用户</th>
              <th>商品</th>
              <th>金额</th>
              <th>状态</th>
              <th>创建时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="order in orders" :key="order.order_no">
              <td>
                <RouterLink :to="`/orders/${order.order_no}`" class="mono text-sm accent-text">
                  {{ order.order_no }}
                </RouterLink>
              </td>
              <td class="text-sm">{{ order.user?.username || `#${order.user_id}` }}</td>
              <td class="truncate text-sm">{{ order.product?.name || `#${order.product_id}` }}</td>
              <td class="nums text-sm">{{ money(order.total_amount) }}</td>
              <td>
                <span class="badge" :class="orderStatus(order.status).badge">{{ orderStatus(order.status).label }}</span>
              </td>
              <td class="text-sm quiet">{{ when(order.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
