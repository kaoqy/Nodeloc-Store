<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import PaginationFooter from '../components/PaginationFooter.vue'
import { listOrders } from '../api/orders'
import { errorMessage, fulfillmentStatus, money, orderStatus, when } from '../utils/format'
import type { Order } from '../types'

const PageSize = 20

const loading = ref(true)
const orders = ref<Order[]>([])
const total = ref(0)
const offset = ref(0)
const search = ref('')
const statusFilter = ref('')
const error = ref('')

const page = computed(() => Math.floor(offset.value / PageSize) + 1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))
const from = computed(() => (orders.value.length ? offset.value + 1 : 0))
const to = computed(() => offset.value + orders.value.length)

const statuses = [
  { value: 'pending', label: '待支付' },
  { value: 'paid', label: '已支付' },
  { value: 'completed', label: '已完成' },
  { value: 'cancelled', label: '已取消' },
  { value: 'refunded', label: '已退款' },
]

const filtered = computed(() => Boolean(search.value.trim() || statusFilter.value))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listOrders({
      limit: PageSize,
      offset: offset.value,
      status: statusFilter.value || undefined,
      q: search.value.trim() || undefined,
    })
    orders.value = result.data
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载订单失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  offset.value = 0
  load()
}

function goTo(target: number) {
  offset.value = Math.min(pages.value - 1, Math.max(0, target - 1)) * PageSize
  load()
}

const route = useRoute()

onMounted(() => {
  const status = route.query.status
  if (typeof status === 'string') statusFilter.value = status
  load()
})
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="search"
          class="input w-64"
          type="search"
          placeholder="订单号 / 用户 / 商品 / 交易号"
          aria-label="搜索订单"
          @keyup.enter="applyFilters"
        />
        <select v-model="statusFilter" class="input w-32" aria-label="按订单状态筛选" @change="applyFilters">
          <option value="">全部状态</option>
          <option v-for="item in statuses" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
        <button class="btn btn-secondary btn-sm" @click="applyFilters">筛选</button>
      </div>
      <RouterLink to="/orders?status=pending" class="quiet text-xs">只看待支付 →</RouterLink>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>订单号</th>
            <th>用户</th>
            <th>商品</th>
            <th>金额</th>
            <th>订单状态</th>
            <th>交付</th>
            <th>创建时间</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 5" :key="`skeleton-${i}`">
              <td colspan="8"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!orders.length">
            <td colspan="8">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">{{ filtered ? '没有符合条件的订单' : '还没有订单' }}</p>
                <p class="empty-hint">
                  {{ filtered ? '换个关键词或选择全部状态。' : '买家在前台下单后会出现在这里。' }}
                </p>
              </div>
            </td>
          </tr>
          <tr v-for="order in orders" :key="order.order_no">
            <td>
              <RouterLink :to="`/orders/${order.order_no}`" class="mono text-sm accent-text">
                {{ order.order_no }}
              </RouterLink>
            </td>
            <td class="text-sm">{{ order.user?.username || `#${order.user_id}` }}</td>
            <td class="max-w-[220px] truncate text-sm">{{ order.product?.name || `#${order.product_id}` }}</td>
            <td class="nums text-sm">{{ money(order.total_amount) }} <span class="quiet">×{{ order.quantity }}</span></td>
            <td>
              <span class="badge" :class="orderStatus(order.status).badge">{{ orderStatus(order.status).label }}</span>
            </td>
            <td>
              <span class="badge" :class="fulfillmentStatus(order.fulfillment_status, order.status).badge">
                {{ fulfillmentStatus(order.fulfillment_status, order.status).label }}
              </span>
            </td>
            <td class="text-sm quiet">{{ when(order.created_at) }}</td>
            <td class="text-right">
              <RouterLink :to="`/orders/${order.order_no}`" class="btn btn-ghost btn-sm">详情</RouterLink>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <PaginationFooter
      :page="page"
      :pages="pages"
      :loading="loading"
      :summary="`第 ${from}–${to} 条 · 共 ${total} 条`"
      @change="goTo"
    />
  </section>
</template>
