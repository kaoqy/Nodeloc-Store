<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { getOrder } from '../api/orders'
import type { Order } from '../types'

const route = useRoute()
const loading = ref(true)
const order = ref<Order | null>(null)

const statusText: Record<string, string> = {
  pending: '待支付', paid: '已支付', delivered: '已发货', completed: '已完成',
  cancelled: '已取消', refunded: '已退款',
}

onMounted(async () => {
  try {
    order.value = await getOrder(route.params.order_no as string)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-48" />
    <div class="grid gap-4 sm:grid-cols-2">
      <div v-for="i in 4" :key="i" class="skeleton h-24" />
    </div>
  </section>

  <section v-else-if="order" class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <RouterLink to="/orders" class="text-sm text-[#6b6b80] hover:text-white">← 返回订单列表</RouterLink>
        <h2 class="mt-2 text-xl font-bold">订单详情</h2>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn-secondary">打印订单</button>
        <button class="btn-primary">标记发货</button>
      </div>
    </div>

    <div class="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <div class="card">
        <p class="text-xs text-[#6b6b80]">订单号</p>
        <p class="mt-1 text-sm font-medium">{{ order.order_no }}</p>
      </div>
      <div class="card">
        <p class="text-xs text-[#6b6b80]">总金额</p>
        <p class="mt-1 text-lg font-bold text-[#22c55e]">¥{{ Number(order.total_amount).toFixed(2) }}</p>
      </div>
      <div class="card">
        <p class="text-xs text-[#6b6b80]">状态</p>
        <p class="mt-1">
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
        </p>
      </div>
      <div class="card">
        <p class="text-xs text-[#6b6b80]">创建时间</p>
        <p class="mt-1 text-sm font-medium">{{ order.created_at }}</p>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-2">
      <div class="card">
        <h3 class="mb-4 font-semibold">商品信息</h3>
        <div class="space-y-3">
          <div class="flex justify-between">
            <span class="text-sm text-[#6b6b80]">商品名称</span>
            <span class="text-sm font-medium">{{ order.product?.name || order.product_name || '-' }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-sm text-[#6b6b80]">数量</span>
            <span class="text-sm font-medium">{{ order.quantity || 1 }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-sm text-[#6b6b80]">单价</span>
            <span class="text-sm font-medium">¥{{ Number(order.unit_price || order.total_amount).toFixed(2) }}</span>
          </div>
        </div>
      </div>

      <div class="card">
        <h3 class="mb-4 font-semibold">用户信息</h3>
        <div class="space-y-3">
          <div class="flex justify-between">
            <span class="text-sm text-[#6b6b80]">用户名</span>
            <span class="text-sm font-medium">{{ order.user?.username || '-' }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-sm text-[#6b6b80]">邮箱</span>
            <span class="text-sm font-medium">{{ order.user?.email || '-' }}</span>
          </div>
          <div class="flex justify-between">
            <span class="text-sm text-[#6b6b80]">联系方式</span>
            <span class="text-sm font-medium">{{ order.customer_contact || '-' }}</span>
          </div>
        </div>
      </div>
    </div>

    <div v-if="order.delivery_content" class="card">
      <h3 class="mb-4 font-semibold">交付内容</h3>
      <div class="rounded-lg bg-[#0a0a0f] p-4">
        <pre class="whitespace-pre-wrap text-sm text-[#a1a1b5]">{{ order.delivery_content }}</pre>
      </div>
    </div>
  </section>

  <section v-else class="py-20 text-center text-[#6b6b80]">
    订单不存在
  </section>
</template>
