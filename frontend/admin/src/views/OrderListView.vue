<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listOrders } from '../api/orders'
import type { Order } from '../types'

const loading = ref(true)
const orders = ref<Order[]>([])
const search = ref('')
const statusFilter = ref('')
const page = ref(1)
const perPage = ref(10)

const statusText: Record<string, string> = {
  pending: '待支付', paid: '已支付', delivered: '已发货', completed: '已完成',
  cancelled: '已取消', refunded: '已退款',
}

async function load() {
  loading.value = true
  try {
    const params: any = { page: page.value, per_page: perPage.value }
    if (search.value) params.q = search.value
    if (statusFilter.value) params.status = statusFilter.value
    const result = await listOrders(params)
    orders.value = result.data
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <input v-model="search" class="input w-64" placeholder="搜索订单号..." @keyup.enter="load" />
        <select v-model="statusFilter" class="input w-32" @change="load">
          <option value="">全部状态</option>
          <option value="pending">待支付</option>
          <option value="paid">已支付</option>
          <option value="delivered">已发货</option>
          <option value="completed">已完成</option>
          <option value="cancelled">已取消</option>
        </select>
        <button class="btn-secondary" @click="load">筛选</button>
      </div>
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>订单号</th>
            <th>用户</th>
            <th>商品</th>
            <th>金额</th>
            <th>状态</th>
            <th>支付时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="7">
              <div class="space-y-2">
                <div v-for="i in 5" :key="i" class="skeleton h-8" />
              </div>
            </td>
          </tr>
          <tr v-else-if="!orders.length">
            <td colspan="7" class="py-12 text-center text-[#6b6b80]">暂无订单</td>
          </tr>
          <tr v-for="order in orders" :key="order.order_no">
            <RouterLink :to="`/orders/${order.order_no}`" class="text-indigo-300 hover:text-indigo-200">
              {{ order.order_no }}
            </RouterLink>
            <td>{{ order.user?.username || '-' }}</td>
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
            <td class="text-[#a1a1b5]">{{ order.paid_at || '-' }}</td>
            <td>
              <div class="flex items-center gap-2">
                <RouterLink :to="`/orders/${order.order_no}`" class="btn-ghost text-xs">详情</RouterLink>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">显示 {{ orders.length }} 条</p>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-sm text-[#a1a1b5]">第 {{ page }} 页</span>
        <button class="btn-secondary" @click="page++; load()">下一页</button>
      </div>
    </div>
  </section>
</template>
