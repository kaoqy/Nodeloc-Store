<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import StatCard from '../components/StatCard.vue'
import { listOrders } from '../api/orders'
import { listProducts } from '../api/products'
import { listUsers } from '../api/users'
import { listAuditLogs } from '../api/logs'
import type { AuditLog, Order } from '../types'

const loading = ref(true)
const orders = ref<Order[]>([])
const logs = ref<AuditLog[]>([])
const productCount = ref(0)
const userCount = ref(0)

const revenue = computed(() => orders.value.reduce((sum, order) => sum + Number(order.total_amount || 0), 0))
const pendingCount = computed(() => orders.value.filter(order => ['pending', 'paid'].includes(order.status)).length)

const statusText: Record<string, string> = {
  pending: '待支付', paid: '已支付', delivered: '已发货', completed: '已完成',
  cancelled: '已取消', refunded: '已退款',
}

onMounted(async () => {
  try {
    const [orderResult, productResult, userResult, logResult] = await Promise.all([
      listOrders({ page: 1, per_page: 8 }),
      listProducts({ page: 1, per_page: 100 }),
      listUsers({ page: 1, per_page: 100 }),
      listAuditLogs({ page: 1, per_page: 8 }),
    ])
    orders.value = orderResult.data
    productCount.value = productResult.data.length
    userCount.value = userResult.data.length
    logs.value = logResult.items
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <section class="space-y-6">
    <!-- Stats Grid -->
    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <StatCard title="订单收入" :value="`¥${revenue.toFixed(2)}`" subtitle="本月累计" icon="¥" />
      <StatCard title="待处理订单" :value="pendingCount" subtitle="需要关注" icon="◎" />
      <StatCard title="商品数量" :value="productCount" subtitle="在售中" icon="▣" />
      <StatCard title="用户数量" :value="userCount" subtitle="注册用户" icon="♙" />
    </div>

    <!-- Charts Row -->
    <div class="grid gap-6 xl:grid-cols-3">
      <!-- Sales Chart -->
      <div class="card xl:col-span-2">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="font-semibold">销售趋势</h3>
          <div class="flex gap-2">
            <button class="btn-secondary text-xs">日</button>
            <button class="btn-primary text-xs">周</button>
            <button class="btn-secondary text-xs">月</button>
          </div>
        </div>
        <div class="relative h-64 flex items-end justify-between gap-2">
          <!-- Chart bars placeholder -->
          <div v-for="(h, i) in [60, 45, 78, 52, 90, 65, 85]" :key="i"
            class="flex-1 rounded-t-lg bg-gradient-to-t from-indigo-600/30 to-indigo-400/10 border border-indigo-500/20"
            :style="{ height: h + '%' }"
          />
        </div>
        <div class="mt-4 flex justify-between text-xs text-[#6b6b80]">
          <span>Mon</span><span>Tue</span><span>Wed</span><span>Thu</span><span>Fri</span><span>Sat</span><span>Sun</span>
        </div>
      </div>

      <!-- Top Products -->
      <div class="card">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="font-semibold">热销商品</h3>
          <RouterLink to="/products" class="text-sm text-indigo-300 hover:text-indigo-200">查看全部</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 5" :key="i" class="skeleton h-8" />
        </div>
        <div v-else class="space-y-3">
          <div v-for="i in 5" :key="i" class="flex items-center gap-3">
            <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10 text-sm font-bold text-indigo-300">
              {{ i }}
            </div>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm">示例商品 {{ i }}</p>
              <p class="text-xs text-[#6b6b80]">¥{{ (i * 10).toFixed(2) }}</p>
            </div>
            <span class="text-xs text-[#22c55e]">+{{ i * 5 }}%</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Tables Row -->
    <div class="grid gap-6 xl:grid-cols-3">
      <!-- Recent Orders -->
      <div class="card overflow-hidden xl:col-span-2">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="font-semibold">近期订单</h3>
          <RouterLink to="/orders" class="text-sm text-indigo-300 hover:text-indigo-200">查看全部</RouterLink>
        </div>
        <div v-if="loading" class="space-y-3">
          <div v-for="i in 4" :key="i" class="skeleton h-12" />
        </div>
        <div v-else-if="!orders.length" class="py-10 text-center text-[#6b6b80]">暂无订单</div>
        <div v-else class="table-container">
          <table>
            <thead>
              <tr>
                <th>订单号</th>
                <th>商品</th>
                <th>金额</th>
                <th>状态</th>
                <th>时间</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="order in orders.slice(0, 6)" :key="order.order_no">
                <td>
                  <RouterLink :to="`/orders/${order.order_no}`" class="text-indigo-300 hover:text-indigo-200">
                    {{ order.order_no }}
                  </RouterLink>
                </td>
                <td>{{ order.product_name || order.product?.name || '数字商品' }}</td>
                <td>¥{{ Number(order.total_amount).toFixed(2) }}</td>
                <td>
                  <span
                    :class="[
                      'badge',
                      order.status === 'pending' ? 'badge-warning' :
                      order.status === 'paid' ? 'badge-info' :
                      order.status === 'delivered' ? 'badge-success' :
                      order.status === 'completed' ? 'badge-success' :
                      order.status === 'cancelled' ? 'badge-danger' : 'badge-neutral'
                    ]"
                  >
                    {{ statusText[order.status] || order.status }}
                  </span>
                </td>
                <td class="text-[#a1a1b5]">{{ order.created_at || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- Recent Activity -->
      <div class="card">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="font-semibold">近期操作</h3>
          <RouterLink to="/logs" class="text-sm text-indigo-300 hover:text-indigo-200">审计日志</RouterLink>
        </div>
        <div v-if="!logs.length" class="py-10 text-center text-[#6b6b80]">暂无日志</div>
        <div v-else class="space-y-4">
          <div v-for="log in logs.slice(0, 6)" :key="log.id" class="border-l-2 border-indigo-500/40 pl-3">
            <p class="text-sm">{{ log.action }}</p>
            <p class="mt-1 text-xs text-[#6b6b80]">{{ log.user?.name || log.user?.email || '系统' }} · {{ log.created_at }}</p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
