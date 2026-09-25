<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { createPayment, getOrder } from '../api/payment'
import type { Order } from '../types'

const route = useRoute()
const order = ref<Order | null>(null)
const loading = ref(true)
const paying = ref(false)
const error = ref('')

const money = (value: number) =>
  new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)

const date = (value?: string | null) =>
  value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '暂无'

const statusText = (status?: string) =>
  ({ pending: '待支付', paid: '已支付', processing: '处理中', delivered: '已交付', completed: '已完成', cancelled: '已取消', failed: '失败' }[status || ''] || status || '未知')

const statusBadge = (status?: string) =>
  ({ pending: 'badge-warning', paid: 'badge-info', processing: 'badge-info', delivered: 'badge-success', completed: 'badge-success', cancelled: 'badge-danger', failed: 'badge-danger' }[status || ''] || 'badge-neutral')

async function pay() {
  if (!order.value) return
  paying.value = true
  error.value = ''
  try {
    const result = await createPayment({
      order_no: order.value.order_no,
      description: order.value.description || undefined,
    })
    window.location.href = result.payment_order.payment_url
  } catch {
    error.value = '创建支付失败'
  } finally {
    paying.value = false
  }
}

onMounted(async () => {
  try {
    order.value = (await getOrder(String(route.params.orderNo))).order
  } catch {
    error.value = '订单详情加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mx-auto max-w-4xl px-4 py-10 sm:px-6">
    <!-- Loading -->
    <div v-if="loading" class="py-24 text-center text-[#b4b2c3]">
      <div class="mx-auto mb-4 size-10 animate-spin rounded-full border-4 border-white/10 border-t-purple-400" />
      正在加载订单…
    </div>

    <!-- Content -->
    <div v-else-if="order" class="space-y-6 fade-in">
      <!-- Order Header -->
      <div class="glass p-7 sm:p-9">
        <div class="flex flex-col gap-5 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <p class="text-xs tracking-widest text-[#7b7990] uppercase">订单号</p>
            <h1 class="mt-1 break-all text-xl font-bold">{{ order.order_no }}</h1>
            <p class="mt-2 text-sm text-[#b4b2c3]">创建于 {{ date(order.created_at) }}</p>
          </div>
          <span :class="['badge', statusBadge(order.status)]">{{ statusText(order.status) }}</span>
        </div>

        <div class="mt-7 h-px bg-gradient-to-r from-transparent via-white/15 to-transparent" />

        <div class="mt-7 grid gap-5 sm:grid-cols-3">
          <div class="rounded-2xl border border-white/[0.08] bg-white/[0.04] p-4">
            <p class="text-xs text-[#7b7990]">商品</p>
            <p class="mt-1.5 font-medium">{{ order.product?.name || order.product_name || '数字商品' }}</p>
          </div>
          <div class="rounded-2xl border border-purple-400/20 bg-gradient-to-br from-purple-500/12 to-indigo-500/8 p-4">
            <p class="text-xs text-[#7b7990]">金额</p>
            <p class="mt-1.5 text-xl font-black gradient-text">{{ money(order.amount) }}</p>
          </div>
          <div class="rounded-2xl border border-white/[0.08] bg-white/[0.04] p-4">
            <p class="text-xs text-[#7b7990]">交付状态</p>
            <p class="mt-1.5 font-medium">{{ statusText(order.delivery_status) }}</p>
          </div>
        </div>

        <button v-if="order.status === 'pending'" class="btn btn-primary mt-7 px-8" :disabled="paying" @click="pay">
          {{ paying ? '跳转支付中…' : '继续支付' }}
        </button>
        <p v-if="error" class="fade-in mt-4 rounded-2xl border border-rose-400/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{{ error }}</p>
      </div>

      <!-- Delivery Content -->
      <div class="glass p-7 sm:p-9">
        <h2 class="text-lg font-bold tracking-tight">交付内容</h2>
        <p v-if="!order.cards?.length" class="mt-4 rounded-2xl border border-white/[0.07] bg-white/[0.035] p-6 text-[#b4b2c3]">
          商品尚未交付。支付成功后，交付内容会显示在这里。
        </p>
        <div v-else class="mt-5 space-y-4">
          <div v-for="card in order.cards" :key="card.id" class="rounded-2xl border border-white/[0.1] bg-[#0a0916]/60 p-5 backdrop-blur-md">
            <div class="flex items-center justify-between gap-3">
              <span class="text-xs tracking-widest text-[#7b7990] uppercase">卡密 / 数字内容</span>
              <span class="badge badge-success">{{ card.status || '已交付' }}</span>
            </div>
            <code class="mt-3 block break-all whitespace-pre-wrap rounded-xl bg-black/30 p-3.5 text-sm text-[#e4e4e7]">{{ card.code || card.content }}</code>
          </div>
        </div>
      </div>
    </div>

    <!-- Error -->
    <p v-else class="glass p-10 text-center text-rose-300">{{ error || '未找到订单' }}</p>
  </div>
</template>
