<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { createPayment, getOrder } from '../api/payment'
import { errorMessage } from '../api/client'
import { fulfillmentStatus, money, orderStatus, paymentNotice, when } from '../utils/format'
import type { Order } from '../types'

const route = useRoute()
const order = ref<Order | null>(null)
const loading = ref(true)
const paying = ref(false)
const error = ref('')
const payError = ref('')
const copied = ref(false)
const copiedNo = ref(false)
const notice = computed(() => paymentNotice(typeof route.query.pay === 'string' ? route.query.pay : ''))
let settleTimer: number | undefined

const isPaid = computed(() => {
  const status = order.value?.status ?? ''
  return status !== 'pending' && status !== 'cancelled' && status !== 'failed'
})

const delivered = computed(() => {
  const current = order.value
  return Boolean(current?.delivery_content) || current?.fulfillment_status === 'delivered' || current?.fulfillment_status === 'completed'
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    order.value = await getOrder(String(route.params.orderNo))
  } catch (e) {
    error.value = errorMessage(e, '订单加载失败')
  } finally {
    loading.value = false
  }
}

async function pay() {
  const current = order.value
  if (!current || paying.value) return
  paying.value = true
  payError.value = ''
  try {
    const payment = await createPayment(current.order_no, current.product?.name)
    if (!payment.payment_url) throw new Error('支付通道未返回付款地址，请稍后重试')
    window.location.href = payment.payment_url
  } catch (e) {
    payError.value = errorMessage(e, '发起支付失败')
    paying.value = false
  }
}

async function copyContent() {
  const content = order.value?.delivery_content
  if (!content) return
  try {
    await navigator.clipboard.writeText(content)
    copied.value = true
    window.setTimeout(() => (copied.value = false), 1800)
  } catch {
    payError.value = '浏览器不允许自动复制，请长按选中后手动复制'
  }
}

async function copyOrderNo() {
  const current = order.value
  if (!current) return
  try {
    await navigator.clipboard.writeText(current.order_no)
    copiedNo.value = true
    window.setTimeout(() => (copiedNo.value = false), 1800)
  } catch {
    payError.value = '浏览器不允许自动复制，请长按选中订单号后手动复制'
  }
}

onMounted(async () => {
  await load()
  watchSettlement()
})

onUnmounted(() => {
  if (settleTimer) window.clearInterval(settleTimer)
})

function watchSettlement() {
  // NodeLoc can notify server-side a moment after the browser lands here, so an
  // unpaid-looking redirect is polled quietly instead of leaving 待支付 on screen.
  if (settleTimer || order.value?.status !== 'pending') return
  let waited = 0
  settleTimer = window.setInterval(async () => {
    waited += 8
    try {
      order.value = await getOrder(String(route.params.orderNo))
    } catch {
      // keep showing the last known state and retry on the next tick
    }
    if (order.value?.status !== 'pending' || waited >= 120) {
      if (settleTimer) window.clearInterval(settleTimer)
      settleTimer = undefined
    }
  }, 8000)
}
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-4 py-10 sm:px-6">
    <div v-if="loading" class="space-y-5">
      <div class="skeleton h-28 w-full !rounded-lg" />
      <div class="skeleton h-52 w-full !rounded-lg" />
    </div>

    <p v-else-if="!order" class="alert alert-danger" role="alert">{{ error || '未找到该订单' }}</p>

    <div v-else class="fade-in space-y-5">
      <RouterLink to="/orders" class="hint inline-flex items-center gap-1.5 transition-colors hover:text-[var(--text)]">
        ← 我的订单
      </RouterLink>

      <p v-if="notice" class="alert" :class="notice.badge" role="status">{{ notice.label }}</p>

      <section class="card">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="min-w-0">
            <p class="eyebrow">Order</p>
            <div class="flex flex-wrap items-center gap-2">
              <h1 class="mono mt-1.5 break-all text-lg font-semibold">{{ order.order_no }}</h1>
              <button
                class="btn btn-quiet btn-sm shrink-0"
                :aria-label="copiedNo ? '订单号已复制' : '复制订单号'"
                @click="copyOrderNo"
              >
                {{ copiedNo ? '已复制' : '复制单号' }}
              </button>
            </div>
          </div>
          <span class="badge" :class="orderStatus(order.status).badge">{{ orderStatus(order.status).label }}</span>
        </div>

        <div class="my-5 divider" />

        <div class="flex items-start gap-4">
          <div class="grid size-16 shrink-0 place-items-center overflow-hidden rounded-md border border-[var(--stroke-quiet)] bg-[var(--surface-sunken)]">
            <img
              v-if="order.product?.image_path"
              :src="order.product.image_path"
              :alt="order.product.name"
              class="size-full object-cover"
            />
            <span v-else class="mono text-xs text-[var(--text-quiet)]">
              {{ (order.product?.name || 'NL').slice(0, 2).toUpperCase() }}
            </span>
          </div>
          <div class="min-w-0 flex-1">
            <RouterLink
              v-if="order.product"
              :to="`/products/${order.product.slug}`"
              class="text-[15px] font-semibold underline-offset-4 hover:underline"
            >
              {{ order.product.name }}
            </RouterLink>
            <p v-else class="text-[15px] font-semibold">数字商品</p>
            <p class="hint mt-1 nums">
              {{ money(order.unit_price) }} × {{ order.quantity }}
            </p>
          </div>
          <div class="text-right">
            <p class="hint">实付</p>
            <p class="nums accent-text text-xl font-bold">{{ money(order.total_amount) }}</p>
          </div>
        </div>

        <div v-if="payError" class="alert alert-danger mt-5" role="alert">{{ payError }}</div>
        <p v-if="error" class="alert alert-warning mt-5" role="alert">{{ error }}</p>

        <div v-if="order.status === 'pending'" class="mt-5 flex flex-wrap items-center gap-3">
          <button class="btn btn-primary" :disabled="paying" @click="pay">
            <span v-if="paying" class="spinner spinner-light" />
            {{ paying ? '跳转支付中…' : '继续支付' }}
          </button>
          <RouterLink to="/" class="btn btn-quiet btn-sm">返回挑选</RouterLink>
        </div>
        <div
          v-else-if="!delivered && (order.status === 'paid' || order.status === 'completed')"
          class="mt-5 flex flex-wrap items-center gap-3"
        >
          <p class="flex-1 text-sm text-[var(--text-dim)]">
            支付已完成，交付通常几秒内到达；人工交付会进入商家队列。
          </p>
          <button class="btn btn-quiet btn-sm" @click="load">刷新状态</button>
        </div>
      </section>

      <!-- Delivery -->
      <section class="card">
        <div class="flex items-center justify-between gap-3">
          <h2 class="text-[15px] font-bold">交付内容</h2>
          <span class="badge" :class="fulfillmentStatus(order.fulfillment_status, order.status).badge">
            {{ fulfillmentStatus(order.fulfillment_status, order.status).label }}
          </span>
        </div>

        <template v-if="order.delivery_content">
          <p class="hint mt-4">
            请妥善保存，商品仅交付一次。
            <span v-if="order.product?.product_type === 'card' && order.product.auto_deliver">（卡密类商品不支持退换）</span>
          </p>
          <div class="codebox mt-3">{{ order.delivery_content }}</div>
          <div class="mt-3 flex flex-wrap gap-2">
            <button class="btn btn-secondary btn-sm" @click="copyContent">
              {{ copied ? '已复制' : '复制全部内容' }}
            </button>
          </div>
        </template>

        <div v-else class="card-quiet mt-4 text-sm text-[var(--text-dim)]">
          <template v-if="order.status === 'pending'">支付成功后即可查看交付内容。</template>
          <template v-else-if="order.fulfillment_status === 'waiting_stock'">
            卡密库存已临时售罄，补货后商家会立即为你发货。
          </template>
          <template v-else-if="order.fulfillment_status === 'manual_pending'">
            商家正在人工交付，完成后这里会显示结果与说明。
          </template>
          <template v-else-if="order.status === 'refunded'">款项已退回，本单不再交付。</template>
          <template v-else-if="order.status === 'cancelled' || order.status === 'failed'">
            订单已关闭，不会发货。
          </template>
          <template v-else>暂无交付内容，可稍后刷新或联系管理员。</template>
        </div>

        <template v-if="order.delivery_note">
          <h3 class="mt-6 text-[13px] font-semibold text-[var(--text-dim)]">商家说明</h3>
          <p class="mt-2 whitespace-pre-line text-sm leading-7 text-[var(--text-dim)]">{{ order.delivery_note }}</p>
        </template>
      </section>

      <!-- Timeline + meta -->
      <section class="card">
        <h2 class="text-[15px] font-bold">订单进度</h2>
        <div class="mt-5">
          <div class="timeline-item timeline-done">
            <p class="text-sm font-medium">下单成功</p>
            <p class="hint nums mt-0.5">{{ when(order.created_at) }}</p>
          </div>
          <div class="timeline-item" :class="order.paid_at ? 'timeline-done' : 'timeline-active'">
            <p class="text-sm font-medium">支付完成</p>
            <p class="hint nums mt-0.5">{{ when(order.paid_at) }}</p>
          </div>
          <div class="timeline-item" :class="delivered ? 'timeline-done' : ''">
            <p class="text-sm font-medium">商品交付</p>
            <p class="hint nums mt-0.5">{{ when(order.delivered_at) }}</p>
          </div>
        </div>

        <dl class="mt-6 grid gap-x-6 gap-y-3 text-sm sm:grid-cols-2">
          <div>
            <dt class="text-[var(--text-quiet)]">交易号</dt>
            <dd class="mono mt-0.5 truncate">{{ order.transaction_id || '—' }}</dd>
          </div>
          <div>
            <dt class="text-[var(--text-quiet)]">交付状态</dt>
            <dd class="mt-0.5">{{ fulfillmentStatus(order.fulfillment_status, order.status).label }}</dd>
          </div>
          <div v-if="order.customer_contact">
            <dt class="text-[var(--text-quiet)]">联系方式</dt>
            <dd class="mono mt-0.5 break-all">{{ order.customer_contact }}</dd>
          </div>
          <div v-if="order.customer_note">
            <dt class="text-[var(--text-quiet)]">我的备注</dt>
            <dd class="mt-0.5 whitespace-pre-line text-[var(--text-dim)]">{{ order.customer_note }}</dd>
          </div>
        </dl>
      </section>
    </div>
  </div>
</template>
