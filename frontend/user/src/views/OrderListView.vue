<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listOrders } from '../api/payment'
import type { Order } from '../types'

const orders = ref<Order[]>([])
const loading = ref(true)
const error = ref('')

const money = (value: number) =>
  new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)

const date = (value: string) =>
  new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))

const statusText = (status: string) =>
  ({ pending: '待支付', paid: '已支付', processing: '处理中', delivered: '已交付', completed: '已完成', cancelled: '已取消', failed: '失败' }[status] || status)

const statusBadge = (status: string) =>
  ({ pending: 'badge-warning', paid: 'badge-info', processing: 'badge-info', delivered: 'badge-success', completed: 'badge-success', cancelled: 'badge-danger', failed: 'badge-danger' }[status] || 'badge-neutral')

onMounted(async () => {
  try {
    orders.value = (await listOrders()).orders
  } catch {
    error.value = '订单加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mx-auto max-w-5xl px-4 py-10 sm:px-6">
    <div class="mb-8">
      <h1 class="text-2xl font-bold">我的订单</h1>
      <p class="mt-2 text-sm text-[#71717a]">查看支付进度与数字商品交付状态</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="space-y-3">
      <div v-for="i in 3" :key="i" class="skeleton h-24 rounded-2xl" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="rounded-2xl border border-red-500/20 bg-red-500/10 p-6 text-red-300">{{ error }}</div>

    <!-- Orders -->
    <div v-else-if="orders.length" class="space-y-3 fade-in">
      <RouterLink
        v-for="order in orders"
        :key="order.id"
        :to="`/orders/${order.order_no}`"
        class="block rounded-2xl border border-white/[0.08] bg-[#111114] p-5 transition hover:border-purple-400/30 hover:shadow-lg sm:p-6"
      >
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-3">
              <h2 class="font-semibold truncate">{{ order.product?.name || order.product_name || '数字商品订单' }}</h2>
              <span :class="['badge', statusBadge(order.status)]">{{ statusText(order.status) }}</span>
            </div>
            <p class="mt-2 text-sm text-[#71717a]">订单号：{{ order.order_no }}</p>
            <p class="mt-1 text-sm text-[#71717a]">{{ date(order.created_at) }}</p>
          </div>
          <div class="sm:text-right">
            <p class="text-xl font-bold text-white">{{ money(order.amount) }}</p>
            <p class="mt-1 text-sm text-[#71717a]">查看详情 →</p>
          </div>
        </div>
      </RouterLink>
    </div>

    <!-- Empty -->
    <div v-else class="rounded-3xl border border-white/[0.08] bg-[#111114] py-24 text-center">
      <p class="text-xl font-semibold">暂时没有订单</p>
      <RouterLink to="/" class="mt-4 inline-block text-purple-300 hover:text-purple-200">去逛逛商品 →</RouterLink>
    </div>
  </div>
</template>
