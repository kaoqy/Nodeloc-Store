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
      <h1 class="text-3xl font-black tracking-tight">我的<span class="gradient-text">订单</span></h1>
      <p class="mt-2 text-sm text-[#7b7990]">查看支付进度与数字商品交付状态</p>
    </div>

    <!-- Loading -->
    <div v-if="loading" class="space-y-4">
      <div v-for="i in 3" :key="i" class="skeleton h-28" />
    </div>

    <!-- Error -->
    <div v-else-if="error" class="glass fade-in p-6 text-rose-300">{{ error }}</div>

    <!-- Orders -->
    <div v-else-if="orders.length" class="space-y-4 fade-in">
      <RouterLink
        v-for="order in orders"
        :key="order.id"
        :to="`/orders/${order.order_no}`"
        class="sheen block rounded-[22px] border border-white/[0.12] bg-gradient-to-b from-white/[0.075] to-white/[0.03] p-5 shadow-[inset_0_1px_0_rgba(255,255,255,0.13)] backdrop-blur-2xl transition-all duration-400 [transition-timing-function:cubic-bezier(0.34,1.56,0.64,1)] hover:-translate-y-0.5 hover:border-purple-400/35 hover:shadow-[inset_0_1px_0_rgba(255,255,255,0.18),0_18px_44px_-12px_rgba(0,0,0,0.5),0_0_36px_-10px_rgba(168,85,247,0.3)] sm:p-6"
      >
        <div class="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-3">
              <h2 class="truncate font-semibold">{{ order.product?.name || order.product_name || '数字商品订单' }}</h2>
              <span :class="['badge', statusBadge(order.status)]">{{ statusText(order.status) }}</span>
            </div>
            <p class="mt-2 text-sm text-[#7b7990]">订单号：{{ order.order_no }}</p>
            <p class="mt-1 text-sm text-[#7b7990]">{{ date(order.created_at) }}</p>
          </div>
          <div class="sm:text-right">
            <p class="text-xl font-bold gradient-text">{{ money(order.amount) }}</p>
            <p class="mt-1 text-sm text-[#7b7990] transition group-hover:text-white">查看详情 →</p>
          </div>
        </div>
      </RouterLink>
    </div>

    <!-- Empty -->
    <div v-else class="glass rise-in py-24 text-center">
      <div class="mx-auto mb-6 grid size-16 place-items-center rounded-full border border-white/15 bg-white/[0.06] text-2xl backdrop-blur-md">🧾</div>
      <p class="text-xl font-semibold">暂时没有订单</p>
      <RouterLink to="/" class="mt-4 inline-block font-medium text-purple-300 transition hover:text-purple-200">去逛逛商品 →</RouterLink>
    </div>
  </div>
</template>
