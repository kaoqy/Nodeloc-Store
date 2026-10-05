<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import WorkbenchHeader from '../components/WorkbenchHeader.vue'
import { cancelOrder, deliverOrder, fulfillOrder, getOrder, reconcileOrder, refundOrder } from '../api/orders'
import { errorMessage, fulfillmentStatus, money, orderStatus, providerStatus, reconcileMessage, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { closeOnEscape } from '../utils/dialog'
import type { Order } from '../types'

const route = useRoute()
const auth = useAuthStore()

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const order = ref<Order | null>(null)
const showDeliver = ref(false)
closeOnEscape(showDeliver, false)
const deliveryContent = ref('')
const copied = ref(false)
const formValueEntries = computed<[string, string][]>(() => {
  const raw = order.value?.form_values
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>
    return Object.entries(parsed).map(([key, value]) => [key, String(value ?? '')])
  } catch {
    return []
  }
})

// Every button below moves the order or the money, so they belong to
// orders:manage. A 客服 account with only orders:view reads this same screen and
// is told why there is nothing to press, rather than meeting a 403 on click.
const canManage = computed(() => auth.allows('orders', 'manage'))

const orderNo = computed(() => String(route.params.orderNo || ''))
const status = computed(() => orderStatus(order.value?.status || ''))
const fulfilment = computed(() => fulfillmentStatus(order.value?.fulfillment_status, order.value?.status))

const isPaid = computed(() => ['paid', 'completed'].includes(order.value?.status || ''))
const delivered = computed(() => ['delivered', 'completed'].includes(order.value?.fulfillment_status || ''))
const waitingStock = computed(() => order.value?.fulfillment_status === 'waiting_stock')
// 插件交付中: the goods are being delivered by an installed plugin, which is a
// different queue from 等待补货 and from 等待人工发货.
const pluginPending = computed(() => order.value?.fulfillment_status === 'plugin_pending')
const pluginReview = computed(() => order.value?.fulfillment_status === 'plugin_review')
const canDeliver = computed(() => isPaid.value && !delivered.value)

const steps = computed(() => {
  const item = order.value
  return [
    { label: '创建订单', at: item?.created_at, done: Boolean(item) },
    { label: '完成支付', at: item?.paid_at, done: Boolean(item?.paid_at) },
    { label: '交付商品', at: item?.delivered_at, done: Boolean(item?.delivered_at) },
  ]
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    order.value = await getOrder(orderNo.value)
  } catch (err) {
    order.value = null
    error.value = errorMessage(err, '订单加载失败')
  } finally {
    loading.value = false
  }
}

async function run(
  action: () => Promise<Order>,
  success: string,
  failure: (error: unknown) => string = (err) => errorMessage(err, '操作失败'),
) {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    order.value = await action()
    notice.value = success
  } catch (err) {
    error.value = failure(err)
  } finally {
    busy.value = false
  }
}

// 退款 and 取消订单 are the two buttons here that cannot be un-pressed: the first
// moves real NL back through NodeLoc, the second releases the order and its cards
// from under a buyer who may still be at the cashier. A single click should not
// be enough to do either.
function askRefund() {
  if (!confirm(`给订单 ${orderNo.value} 退款？商店会向 NodeLoc 发起转账，这一步撤不回来。`)) return
  void run(() => refundOrder(orderNo.value), '订单已退款')
}

function askCancel() {
  if (!order.value) return
  if (!confirm(`取消订单 ${orderNo.value}？买家可能还在付款路上，取消后这一单不再发货。`)) return
  void run(() => cancelOrder(orderNo.value), '订单已取消')
}

/** A review-state retry can create a second external code, so it is explicit. */
function confirmExternalRetry(): boolean {
  return confirm(
    '这次重试会再次请求 New-API 创建兑换码。只有在确认第三方后台没有已创建的兑换码时才继续。是否继续？',
  )
}

async function submitDelivery() {
  const content = deliveryContent.value.trim()
  if (!content) return
  await run(() => deliverOrder(orderNo.value, content), '已发货并完结订单')
  deliveryContent.value = ''
  showDeliver.value = false
}

/**
 * 查单对账。NodeLoc 答「还没到账」不是错误而是结果，所以这里分开说：确认到账、
 * 还是服务商记为失败/取消（那种单子不会自己变好）。
 */
async function reconcile() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await reconcileOrder(orderNo.value)
    order.value = result.order
    notice.value = result.settled
      ? `已查单：NodeLoc 确认到账${result.order.delivery_content ? '，交付内容已写入本单' : '，交付已进入队列'}。`
      : `已向 NodeLoc 查询：这笔付款记为「${providerStatus(result.provider_status)}」，商店未入账。${result.retryable ? '可在买家确认后再次查询。' : '这单不会自动到账，请人工处理。'}`
    if (result.provider_via === 'reprocess') {
      // The money answer is the same either way, but the shop owner has to be able
      // to see that NodeLoc's 查单 route is not what gave it.
      notice.value += `（本单经由「下单」核实：${result.provider_note || 'NodeLoc 的查单接口不接受服务器端调用'}）`
    }
  } catch (err) {
    error.value = reconcileMessage(err)
  } finally {
    busy.value = false
  }
}

async function copyContent() {
  const content = order.value?.delivery_content || ''
  if (!content) return
  try {
    await navigator.clipboard.writeText(content)
    copied.value = true
    setTimeout(() => (copied.value = false), 1600)
  } catch {
    error.value = '浏览器拒绝了剪贴板访问，请手动选择内容复制。'
  }
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-52" />
    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div v-for="i in 4" :key="i" class="skeleton h-24" />
    </div>
    <div class="skeleton h-64" />
  </section>

  <section v-else-if="!order" class="card py-16 text-center">
    <p class="muted">{{ error || '订单不存在' }}</p>
    <RouterLink to="/orders" class="btn btn-secondary btn-sm mt-4">返回订单列表</RouterLink>
  </section>

  <section v-else class="space-y-5">
    <WorkbenchHeader
      :title="order.order_no"
      description="查看支付、交付与售后状态，所有金额和交易信息均以服务端记录为准。"
      eyebrow="订单详情"
      back-to="/orders"
      back-label="返回订单列表"
    >
      <template #meta>
        <div class="mt-2 flex flex-wrap items-center gap-2.5">
          <span class="badge" :class="status.badge">{{ status.label }}</span>
          <span class="badge" :class="fulfilment.badge">{{ fulfilment.label }}</span>
        </div>
      </template>
      <template #actions>
        <p v-if="!canManage" class="quiet max-w-56 text-right text-xs">
          当前账号只有查看订单的权限，需要处理本单请向店家申请「订单管理」。
        </p>
        <button
          v-if="canManage && order.status === 'pending'"
          class="btn btn-primary btn-sm"
          :disabled="busy"
          @click="reconcile"
        >
          查单对账
        </button>
        <button
          v-if="canManage && order.status === 'pending'"
          class="btn btn-secondary btn-sm"
          :disabled="busy"
          @click="askCancel"
        >
          取消订单
        </button>
        <button
          v-if="canManage && (waitingStock || pluginPending || pluginReview)"
          class="btn btn-primary btn-sm"
          :disabled="busy"
          @click="run(
            () => fulfillOrder(orderNo, pluginReview ? confirmExternalRetry() : false),
            pluginReview ? '已重新请求并尝试确认 New-API 兑换码' : pluginPending ? '已重试插件交付' : '已重试自动交付',
          )"
        >
          {{ pluginReview ? '再次请求并确认' : pluginPending ? '重试插件交付' : '重试自动交付' }}
        </button>
        <button v-if="canManage && canDeliver" class="btn btn-primary btn-sm" :disabled="busy" @click="showDeliver = true">
          人工发货
        </button>
        <button
          v-if="canManage && isPaid && order.status !== 'refunded'"
          class="btn btn-danger btn-sm"
          :disabled="busy"
          @click="askRefund"
        >
          退款
        </button>
      </template>
    </WorkbenchHeader>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <div v-if="notice" class="alert alert-success" role="status">{{ notice }}</div>
    <div v-if="pluginReview" class="alert alert-warning" role="status">
      New-API 请求可能已经发出，但响应无法确认兑换码是否创建。为避免重复创建，
      系统没有自动重试。请先到 New-API 后台核对是否已有兑换码；只有确认不会重复时，
      再使用「再次请求并确认」。
    </div>

    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card !p-4">
        <p class="eyebrow">金额</p>
        <p class="nums mt-2 text-xl font-bold accent-text">{{ money(order.total_amount) }}</p>
        <p class="quiet mt-1 text-xs mono">单价 {{ money(order.unit_price ?? 0) }} × {{ order.quantity }}</p>
        <!-- The shop needs to see the same three numbers the buyer sees: what the
             shelf priced, what the code took, what NodeLoc actually collected. -->
        <p v-if="order.discount_amount" class="mt-1 text-xs text-[var(--success)]">
          优惠码 {{ order.coupon_code || '—' }} 减 {{ money(order.discount_amount) }}
        </p>
      </div>
      <div class="card !p-4">
        <p class="eyebrow">买家</p>
        <p class="mt-2 truncate text-sm font-semibold">{{ order.user?.username || `用户 #${order.user_id}` }}</p>
        <p v-if="order.user?.email" class="quiet mt-1 truncate text-xs mono">{{ order.user.email }}</p>
        <RouterLink v-if="order.user_id" :to="`/orders?user=${order.user_id}`" class="quiet text-xs hover:text-[var(--text)]">
          只看他的订单 →
        </RouterLink>
      </div>
      <div class="card !p-4">
        <p class="eyebrow">商品</p>
        <p class="mt-2 truncate text-sm font-semibold">{{ order.product?.name || `商品 #${order.product_id}` }}</p>
        <p class="quiet mt-1 truncate text-xs mono">{{ order.product?.slug || '—' }}</p>
      </div>
      <div class="card !p-4">
        <p class="eyebrow">交易号</p>
        <p class="mono mt-2 truncate text-sm">{{ order.transaction_id || '—' }}</p>
        <p class="quiet mt-1 text-xs">{{ when(order.created_at) }}</p>
      </div>
    </div>

    <div class="grid gap-5 lg:grid-cols-3">
      <div class="card lg:col-span-2">
        <div class="mb-3 flex items-center justify-between">
          <h3 class="text-sm font-semibold">交付内容</h3>
          <button v-if="order.delivery_content" class="btn btn-ghost btn-sm" @click="copyContent">
            {{ copied ? '已复制' : '复制' }}
          </button>
        </div>
        <pre v-if="order.delivery_content" class="codebox">{{ order.delivery_content }}</pre>
        <p v-else class="py-8 text-center text-sm quiet">尚未交付</p>

        <template v-if="order.delivery_note">
          <h3 class="mb-3 mt-6 text-sm font-semibold">商家说明</h3>
          <p class="break-words whitespace-pre-wrap text-sm muted">{{ order.delivery_note }}</p>
        </template>
      </div>

      <div class="space-y-5">
        <div class="card">
          <h3 class="mb-4 text-sm font-semibold">订单进度</h3>
          <div
            v-for="step in steps"
            :key="step.label"
            class="timeline-item"
            :class="step.done ? 'timeline-done' : ''"
          >
            <p class="text-sm" :class="step.done ? '' : 'quiet'">{{ step.label }}</p>
            <p class="quiet mt-0.5 text-xs">{{ step.at ? when(step.at) : '待处理' }}</p>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-3 text-sm font-semibold">买家备注</h3>
          <dl class="space-y-2.5 text-sm">
            <div class="flex justify-between gap-3">
              <dt class="quiet">联系方式</dt>
              <dd class="truncate">{{ order.customer_contact || '—' }}</dd>
            </div>
            <div class="flex justify-between gap-3">
              <dt class="quiet">备注</dt>
              <dd class="min-w-0 break-words text-right">{{ order.customer_note || '—' }}</dd>
            </div>
            <div v-if="order.topup_amount" class="flex justify-between gap-3 border-t border-[var(--stroke)] pt-2">
              <dt class="quiet">本次充值额度</dt>
              <dd class="nums font-semibold">{{ order.topup_amount }}</dd>
            </div>
            <div v-if="formValueEntries.length" class="border-t border-[var(--stroke)] pt-2">
              <dt class="quiet">购买信息</dt>
              <dd class="mt-1 text-right"><div v-for="([key, value]) in formValueEntries" :key="key" class="break-words">{{ key }}：{{ value }}</div></dd>
            </div>
          </dl>
        </div>
      </div>
    </div>

    <div
      v-if="showDeliver"
      class="overlay" role="dialog" aria-modal="true" aria-label="人工发货"
      @click.self="showDeliver = false"
    >
      <div class="card w-full max-w-lg !p-5">
        <h3 class="text-base font-semibold">人工发货 · {{ order.order_no }}</h3>
        <p class="quiet mt-1 text-xs">内容将展示给买家，可包含卡密、链接或处理说明。提交后订单标记为已交付。</p>
        <textarea
          v-model="deliveryContent"
          class="input mono mt-4 h-44 resize-none text-xs"
          placeholder="CARD-XXXX-XXXX&#10;使用说明……"
        />
        <div class="mt-4 flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showDeliver = false">取消</button>
          <button
            class="btn btn-primary btn-sm"
            :disabled="busy || !deliveryContent.trim()"
            @click="submitDelivery"
          >
            {{ busy ? '提交中…' : '确认发货' }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
