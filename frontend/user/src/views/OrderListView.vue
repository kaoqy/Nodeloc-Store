<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { listOrders, reconcileMessage, reconcileOrder } from '../api/payment'
import { errorMessage } from '../api/client'
import { useInboxStore } from '../stores/inbox'
import { fulfillmentStatus, money, orderStatus, paymentNotice, when } from '../utils/format'
import type { Order } from '../types'

const PageSize = 20

/** Statuses the buyer can narrow the list to; must match the server's allow-list. */
const FILTERS: { key: string; label: string }[] = ['', 'pending', 'paid', 'completed', 'failed', 'cancelled', 'refunded'].map(
  (key) => ({ key, label: key ? orderStatus(key).label : '全部' }),
)

const route = useRoute()
const router = useRouter()
const inbox = useInboxStore()
const orders = ref<Order[]>([])
const total = ref(0)
const loading = ref(true)
const loadingMore = ref(false)
const error = ref('')
const query = ref(typeof route.query.q === 'string' ? route.query.q : '')
const reconciling = ref(false)
const reconcileNote = ref('')
const status = ref(FILTERS.some((item) => item.key === route.query.status) ? String(route.query.status) : '')
const notice = computed(() => paymentNotice(typeof route.query.pay === 'string' ? route.query.pay : ''))

function wanted(): { q: string; status: string } | null {
  const next = {
    q: typeof route.query.q === 'string' ? route.query.q : '',
    status: FILTERS.some((item) => item.key === route.query.status) ? String(route.query.status) : '',
  }
  // Only a link that no longer matches the list is a navigation; 筛选 and 搜索
  // push the address themselves right after setting these refs.
  return next.q === query.value && next.status === status.value ? null : next
}

// The chips and the search box write the address, so the address has to be able
// to write them back: 后退 is how a buyer leaves a filter they just applied.
watch(route.query, () => {
  const next = wanted()
  if (!next) return
  query.value = next.q
  status.value = next.status
  void load(0)
})
const pending = computed(() => orders.value.filter((item) => item.status === 'pending'))
// A pending order that got as far as NodeLoc has a transaction id, and only then
// can 查单 say anything about it. The rest were never opened for payment, so
// sending the buyer to 继续支付 is the only useful action.
const awaitingConfirm = computed(() => pending.value.filter((item) => item.transaction_id))
const unstarted = computed(() => pending.value.filter((item) => !item.transaction_id))

async function load(offset: number) {
  if (offset === 0) {
    loading.value = true
    error.value = ''
  } else {
    loadingMore.value = true
  }
  try {
    const page = await listOrders(PageSize, offset, status.value, query.value)
    orders.value = offset === 0 ? page.orders : [...orders.value, ...page.orders]
    total.value = page.total ?? orders.value.length
  } catch (e) {
    if (offset === 0) error.value = errorMessage(e, '订单加载失败，请稍后重试')
    else error.value = errorMessage(e, '加载更多失败，请稍后重试')
  } finally {
    loading.value = false
    loadingMore.value = false
  }
}

/**
 * The filter lives in the address, so a refresh or a link sent to the shop owner
 * lands on the same list — and 后退 undoes it the way the buyer expects.
 */
function syncUrl() {
  const next: Record<string, string> = {}
  if (status.value) next.status = status.value
  if (query.value.trim()) next.q = query.value.trim()
  void router.replace({ path: '/orders', query: next })
}

function search() {
  syncUrl()
  void load(0)
}

/**
 * 查单：把列表里已发起支付却仍显示待支付的订单交给服务端逐个核实，NodeLoc 已
 * 记为已付的当场入账。回跳丢失时这是唯一的自救入口，不需要用户重新付款。
 */
async function reconcilePending() {
  const queue = awaitingConfirm.value
  if (reconciling.value || !queue.length) return
  reconciling.value = true
  reconcileNote.value = '正在向 NodeLoc 核实…'
  let settled = 0
  let checked = 0
  const failures: string[] = []
  for (const item of queue) {
    try {
      const result = await reconcileOrder(item.order_no)
      checked += 1
      if (result.settled) settled += 1
    } catch (e) {
      checked += 1
      const reason = reconcileMessage(e)
      if (!failures.includes(reason)) failures.push(reason)
    }
    reconcileNote.value = `正在向 NodeLoc 核实…（${checked}/${queue.length}）`
  }
  // One confirmed payment must not bury the two that errored: those are the ones
  // whose reason the shop owner needs when they ask why 到账 is missing.
  const refused = failures.length ? `，另有 ${failures.length} 笔核实失败：${failures[0]}` : ''
  reconcileNote.value = settled
    ? `已确认 ${settled} 笔订单到账${refused}${unstarted.value.length && !failures.length ? '，另有未付款订单见下方' : ''}。`
    : failures[0] || 'NodeLoc 暂无这些订单的到账记录，商店会每隔几分钟自动再核实一次。'
  reconciling.value = false
  await load(0)
}

function choose(next: string) {
  if (next === status.value) return
  status.value = next
  syncUrl()
  load(0)
}
onMounted(() => {
  void load(0)
  // The buyer usually lands here straight from NodeLoc, which is when the shop
  // has had a moment to write the 支付已确认 / 商品已交付 message.
  void inbox.refresh()
})
</script>

<template>
  <div class="mx-auto w-full max-w-4xl px-4 py-10 sm:px-6">
    <header class="mb-8 flex items-end justify-between gap-4">
      <div>
        <p class="eyebrow">进行中与历史订单</p>
        <h1 class="mt-2 text-2xl font-bold">我的订单</h1>
      </div>
      <p v-if="!loading && orders.length" class="hint nums whitespace-nowrap">共 {{ total }} 笔</p>
    </header>

    <p v-if="notice" class="alert mb-4" :class="notice.badge" role="status">{{ notice.label }}</p>

    <div v-if="awaitingConfirm.length" class="alert alert-warning mb-4 flex flex-wrap items-center gap-3" role="status">
      <span class="flex-1">
        {{ notice ? '列表里有' : '有' }} {{ awaitingConfirm.length }} 笔已发起支付但还没确认到账。
        商店每几分钟会向 NodeLoc 自动核实一次，也可以立刻查一次；已扣款请勿重复付款。
      </span>
      <button class="btn btn-secondary btn-sm shrink-0" :disabled="reconciling" @click="reconcilePending">
        <span v-if="reconciling" class="spinner" />
        {{ reconciling ? '核实中…' : '核实支付结果' }}
      </button>
    </div>
    <p v-if="reconcileNote" class="hint -mt-2 mb-5" role="status">{{ reconcileNote }}</p>

    <p v-if="unstarted.length && !reconciling" class="alert alert-info mb-4" role="status">
      另有 {{ unstarted.length }} 笔订单尚未在 NodeLoc 生成交易，付款后才会开始核实。
      点开订单选「继续支付」即可。
    </p>

    <div class="mb-4 flex flex-wrap items-center gap-2">
      <input
        v-model="query"
        class="input w-full max-w-xs"
        type="search"
        placeholder="按订单号 / 交易号 / 商品名搜索"
        aria-label="搜索我的订单"
        @keyup.enter="search"
      />
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="search">搜索</button>
      <button v-if="query" class="btn btn-quiet btn-sm" @click="query = ''; search()">清除</button>
    </div>

    <div class="mb-6 flex flex-wrap gap-2" role="group" aria-label="按订单状态筛选">
      <button
        v-for="item in FILTERS"
        :key="item.key || 'all'"
        class="chip"
        :class="{ 'chip-active': status === item.key }"
        :aria-pressed="status === item.key"
        :disabled="loading"
        @click="choose(item.key)"
      >
        {{ item.label }}
      </button>
    </div>

    <div v-if="loading" class="space-y-3">
      <div v-for="i in 4" :key="i" class="card flex items-center gap-4 !py-5">
        <div class="skeleton size-14 !rounded-md" />
        <div class="flex-1 space-y-2">
          <div class="skeleton h-4 w-1/3" />
          <div class="skeleton h-3 w-1/2" />
        </div>
        <div class="skeleton h-5 w-16 !rounded-full" />
      </div>
    </div>

    <p v-else-if="error && !orders.length" class="alert alert-danger" role="alert">{{ error }}</p>

    <template v-else>
      <ul class="stagger space-y-3">
        <li v-for="order in orders" :key="order.id">
          <RouterLink
            :to="`/orders/${order.order_no}`"
            class="card flex items-center gap-4 !p-4 transition-colors hover:border-[var(--stroke-hi)]"
          >
            <div class="grid size-14 shrink-0 place-items-center overflow-hidden rounded-md border border-[var(--stroke-quiet)] bg-[var(--surface-sunken)]">
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
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="truncate text-[15px] font-semibold">
                  {{ order.product?.name || '数字商品' }}
                </h2>
                <span v-if="order.quantity > 1" class="badge badge-neutral nums">×{{ order.quantity }}</span>
              </div>
              <p class="mono mt-1 truncate text-xs text-[var(--text-quiet)]">{{ order.order_no }}</p>
              <p class="mt-1.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-[var(--text-quiet)]">
                <span class="nums">{{ when(order.created_at) }}</span>
                <!-- The right-hand badge already carries the order status, so the
                     row says only what that badge cannot: how far an unpaid order
                     actually got, or how delivery is going once it is paid. -->
                <span v-if="order.status === 'pending'" class="badge" :class="order.transaction_id ? 'badge-warning' : 'badge-neutral'">
                  {{ order.transaction_id ? '已发起支付 · 待核实' : '尚未付款' }}
                </span>
                <span v-else-if="order.fulfillment_status !== 'delivered' && order.fulfillment_status !== 'completed'" class="badge" :class="fulfillmentStatus(order.fulfillment_status, order.status).badge">
                  {{ fulfillmentStatus(order.fulfillment_status, order.status).label }}
                </span>
              </p>
            </div>

            <div class="shrink-0 text-right">
              <p class="nums text-[15px] font-bold">{{ money(order.total_amount) }}</p>
              <span class="mt-1.5 badge" :class="orderStatus(order.status).badge">
                {{ orderStatus(order.status).label }}
              </span>
            </div>
          </RouterLink>
        </li>
      </ul>

      <p v-if="error" class="alert alert-warning mt-4" role="alert">{{ error }}</p>

      <div v-if="orders.length < total" class="mt-6 text-center">
        <button class="btn btn-quiet btn-sm" :disabled="loadingMore" @click="load(orders.length)">
          {{ loadingMore ? '加载中…' : '加载更多订单' }}
        </button>
      </div>
    </template>

    <div v-if="!loading && !orders.length && !error" class="card py-20 text-center">
      <p class="text-[var(--text-quiet)]" aria-hidden="true">◌</p>
      <p class="mt-3 font-semibold">{{ status ? `${orderStatus(status).label}暂无订单` : '还没有订单' }}</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">
        {{ status ? '换个筛选条件看看，其余订单不会消失。' : '购买支付后，订单与交付内容都会出现在这里。' }}
      </p>
      <button v-if="status" class="btn btn-secondary btn-sm mt-6" @click="choose('')">查看全部订单</button>
      <RouterLink v-else to="/" class="btn btn-primary btn-sm mt-6">去挑选商品</RouterLink>
    </div>
  </div>
</template>
