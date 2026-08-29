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
      listOrders({ page: 1, per_page: 6 }),
      listProducts({ page: 1, per_page: 100 }),
      listUsers({ page: 1, per_page: 100 }),
      listAuditLogs({ page: 1, per_page: 6 }),
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
    <div>
      <h2 class="text-2xl font-bold">运营概览</h2>
      <p class="mt-1 text-sm text-slate-400">查看商城的实时经营数据和近期动态</p>
    </div>

    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <StatCard title="订单收入" :value="`¥${revenue.toFixed(2)}`" icon="¥" />
      <StatCard title="待处理订单" :value="pendingCount" icon="◎" />
      <StatCard title="商品数量" :value="productCount" icon="▣" />
      <StatCard title="用户数量" :value="userCount" icon="♙" />
    </div>

    <div class="grid gap-6 xl:grid-cols-3">
      <div class="panel overflow-hidden xl:col-span-2">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="font-semibold">近期订单</h3>
          <RouterLink to="/orders" class="text-sm text-indigo-300 hover:text-indigo-200">查看全部</RouterLink>
        </div>
        <div v-if="loading" class="py-10 text-center text-slate-500">数据加载中…</div>
        <div v-else-if="!orders.length" class="py-10 text-center text-slate-500">暂无订单</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[620px] text-left text-sm">
            <thead class="text-slate-500"><tr><th class="pb-3">订单号</th><th>商品</th><th>金额</th><th>状态</th><th>时间</th></tr></thead>
            <tbody class="divide-y divide-white/5">
              <tr v-for="order in orders.slice(0, 6)" :key="order.order_no" class="hover:bg-white/[0.03]">
                <td class="py-3"><RouterLink :to="`/orders/${order.order_no}`" class="text-indigo-300">{{ order.order_no }}</RouterLink></td>
                <td>{{ order.product_name || order.product?.name || '数字商品' }}</td>
                <td>¥{{ Number(order.total_amount).toFixed(2) }}</td>
                <td><span class="rounded-full bg-indigo-500/15 px-2.5 py-1 text-xs text-indigo-300">{{ statusText[order.status] || order.status }}</span></td>
                <td class="text-slate-400">{{ order.created_at || '-' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="panel">
        <div class="mb-4 flex items-center justify-between">
          <h3 class="font-semibold">近期操作</h3>
          <RouterLink to="/logs" class="text-sm text-indigo-300 hover:text-indigo-200">审计日志</RouterLink>
        </div>
        <div v-if="!logs.length" class="py-10 text-center text-slate-500">暂无日志</div>
        <div v-else class="space-y-4">
          <div v-for="log in logs.slice(0, 6)" :key="log.id" class="border-l-2 border-indigo-500/40 pl-3">
            <p class="text-sm">{{ log.action }}</p>
            <p class="mt-1 text-xs text-slate-500">{{ log.user?.name || log.user?.email || '系统' }} · {{ log.created_at }}</p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
