<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import PaginationFooter from '../components/PaginationFooter.vue'
import { listOrders, reconcilePendingOrders, exportOrders, type ReconcileReport } from '../api/orders'
import { errorMessage, fulfillmentStatus, money, orderStatus, providerStatus, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Order } from '../types'

const PageSize = 20

const auth = useAuthStore()
const canManage = computed(() => auth.allows('orders', 'manage'))

const loading = ref(true)
const orders = ref<Order[]>([])
const total = ref(0)
const offset = ref(0)
const search = ref('')
const statusFilter = ref('')
const buyerId = ref(0)
const buyerName = ref('')
const error = ref('')

const page = computed(() => Math.floor(offset.value / PageSize) + 1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))
const from = computed(() => (orders.value.length ? offset.value + 1 : 0))
const to = computed(() => offset.value + orders.value.length)
// 一行都没有时不报「第 0–20 条」那种范围，只说总数。
const summary = computed(() =>
  orders.value.length ? `第 ${from.value}–${to.value} 条 · 共 ${total.value} 条` : `共 ${total.value} 条`,
)

const statuses = [
  { value: 'pending', label: '待支付' },
  { value: 'paid', label: '已支付' },
  { value: 'completed', label: '已完成' },
  { value: 'cancelled', label: '已取消' },
  { value: 'refunded', label: '已退款' },
]

const filtered = computed(
  () => Boolean(search.value.trim() || statusFilter.value || buyerId.value || needAttention.value),
)

function queryParams() {
  return {
    limit: PageSize,
    offset: offset.value,
    status: statusFilter.value || undefined,
    q: search.value.trim() || undefined,
    user_id: buyerId.value || undefined,
    attention: needAttention.value ? 'undelivered' : undefined,
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    let result = await listOrders(queryParams())
    // 地址栏里的页码可能比实际页数还大（链接是手写的，或者刚筛完只剩一页）。
    // 有货就退回最后一页再取一次，一笔都没有就回到第 1 页，别把店家丢在
    // 一个看着像坏掉的空白页上（第 2 / 1 页 共 0 条）。
    if (!result.data.length && offset.value > 0) {
      if (!result.total) {
        offset.value = 0
      } else {
        offset.value = (Math.max(1, Math.ceil(result.total / PageSize)) - 1) * PageSize
        result = await listOrders(queryParams())
      }
      syncUrl()
    }
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
  syncUrl()
  load()
}

const exporting = ref(false)
/**
 * 导出的是屏幕上这一批单子：对账、报税、跟 NodeLoc 流水核对都要把整批带走，
 * 而不是照着列表手抄。所以这里传的是当前筛选条件，不含分页。
 */
async function downloadOrders() {
  if (exporting.value) return
  exporting.value = true
  error.value = ''
  notice.value = ''
  try {
    const { rows, truncated } = await exportOrders({
      status: statusFilter.value || undefined,
      q: search.value.trim() || undefined,
      user_id: buyerId.value || undefined,
      attention: needAttention.value ? 'undelivered' : undefined,
    })
    notice.value = truncated
      ? `已导出 ${rows} 笔订单，但筛选结果比这更多，文件只装了前面那些 —— 请缩小范围（例如按状态或日期）后再导一次。`
      : `已导出${filtered.value ? '当前筛选的' : '全部'} ${rows} 笔订单。`
  } catch (err) {
    error.value = errorMessage(err, '导出订单失败')
  } finally {
    exporting.value = false
  }
}

const reconciling = ref(false)
const reconcileReport = ref<ReconcileReport | null>(null)
const notice = ref('')
// 「需处理交付」：钱已到账、东西还没出去的单子。自动重试每几分钟跑一次，这里
// 是给店家看剩下那些确实需要人推一把的（人工发货、补货）。
const needAttention = ref(false)
const reconcileIssues = computed(() =>
  (reconcileReport.value?.items ?? []).filter((item) => !item.settled),
)
// 只要有一笔是用「下单」核实的，就说明 NodeLoc 的查单这条路当前走不通。店家需要
// 知道这一点，否则设置的开关看着正常，钱却从另一条路进来。
const reconcileViaCheckout = computed(() =>
  (reconcileReport.value?.items ?? []).some((item) => item.provider_via === 'reprocess'),
)

/**
 * 批量查单对账：买家关掉付款页、回调没回来时，这些单子会一直挂着「待支付」。
 * 一次问完 NodeLoc，已付的当场入账，剩下的逐条给出原因。
 */
async function reconcilePending() {
  if (reconciling.value) return
  reconciling.value = true
  error.value = ''
  notice.value = ''
  try {
    const report = await reconcilePendingOrders()
    reconcileReport.value = report
    notice.value = report.checked
      ? `已向 NodeLoc 核实 ${report.checked} 笔待支付订单，${report.settled} 笔确认到账并补发。`
      : '没有需要核实的订单：待支付订单里还没有拿到 NodeLoc 交易号的。'
    if (report.settled) await load()
  } catch (err) {
    error.value = errorMessage(err, '批量查单失败')
  } finally {
    reconciling.value = false
  }
}

/** Focus the list on one buyer; the order rows link here by user id. */
function pickBuyer(id: number, name: string) {
  if (!id) return
  buyerId.value = id
  buyerName.value = name
  search.value = ''
  applyFilters()
}

function clearBuyer() {
  buyerId.value = 0
  buyerName.value = ''
  applyFilters()
}

const route = useRoute()

/**
 * Every filter and the current page live in the address bar, so a list pointed
 * at 「已支付、这一位买家、第 3 页」 can be pasted into a support thread. The URL
 * is rewritten rather than routed to: the back-office shell keys its view by
 * fullPath, and a route change would throw away and rebuild this screen.
 */
function syncUrl() {
  const params = new URLSearchParams()
  if (statusFilter.value) params.set('status', statusFilter.value)
  if (search.value.trim()) params.set('q', search.value.trim())
  if (buyerId.value) params.set('user', String(buyerId.value))
  if (needAttention.value) params.set('attention', 'undelivered')
  if (page.value > 1) params.set('page', String(page.value))
  const query = params.toString()
  window.history.replaceState(null, '', query ? `/admin/orders?${query}` : '/admin/orders')
}

function goTo(target: number) {
  offset.value = Math.min(pages.value - 1, Math.max(0, target - 1)) * PageSize
  syncUrl()
  load()
}

/** 只看钱已到账、东西还没出去的单子。 */
function toggleAttention() {
  needAttention.value = !needAttention.value
  if (needAttention.value) statusFilter.value = ''
  applyFilters()
}

onMounted(() => {
  const { status, q, attention, user, page: asked } = route.query
  if (typeof status === 'string' && statuses.some((item) => item.value === status)) statusFilter.value = status
  if (typeof q === 'string') search.value = q
  if (attention === 'undelivered') {
    needAttention.value = true
    // 「需处理交付」本身就是一组状态，不再叠加状态筛选（下拉框这时也是锁住的）。
    statusFilter.value = ''
  }
  if (typeof user === 'string' && /^\d+$/.test(user)) {
    buyerId.value = Number(user)
    buyerName.value = `#${user}`
  }
  if (typeof asked === 'string' && /^\d+$/.test(asked)) offset.value = (Math.max(1, Number(asked)) - 1) * PageSize
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
        <select v-model="statusFilter" class="input w-32" aria-label="按订单状态筛选" :disabled="needAttention" @change="applyFilters">
          <option value="">全部状态</option>
          <option v-for="item in statuses" :key="item.value" :value="item.value">{{ item.label }}</option>
        </select>
        <button class="btn btn-secondary btn-sm" @click="applyFilters">筛选</button>
        <button
          class="chip"
          :class="{ 'chip-active': needAttention }"
          :aria-pressed="needAttention"
          title="已收款但还没发货/待人工/待补货的订单"
          @click="toggleAttention"
        >
          需处理交付
        </button>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <!-- Reconciling writes: it settles payment and hands out cards, so it is
             orders:manage, not the read grant this screen opens with. -->
        <button
          v-if="canManage"
          class="btn btn-secondary btn-sm"
          :disabled="reconciling"
          @click="reconcilePending"
        >
          <span v-if="reconciling" class="spinner" />
          {{ reconciling ? '正在向 NodeLoc 核实…' : '批量查单对账' }}
        </button>
        <!-- 导出只是读，所以跟着这一页本身走：能打开订单页就导得动。 -->
        <button
          class="btn btn-quiet btn-sm"
          :disabled="exporting || loading"
          :title="filtered ? '按当前筛选导出' : '导出全部订单'"
          @click="downloadOrders"
        >
          <span v-if="exporting" class="spinner" />
          {{ exporting ? '正在生成…' : filtered ? '导出当前筛选' : '导出订单 CSV' }}
        </button>
        <RouterLink to="/orders?status=pending" class="quiet text-xs">只看待支付 →</RouterLink>
      </div>
    </div>

    <p v-if="needAttention" class="alert alert-info" role="status">
      正在只看「已收款但未交付」的订单（含人工发货与等待补货），状态筛选暂不生效。商店每几分钟会自动重试交付，这里列出仍需要人推一把的。
      <button class="btn btn-quiet btn-sm ml-2" @click="toggleAttention">查看全部订单</button>
    </p>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="reconcileIssues.length" class="card space-y-2">
      <p class="text-sm font-semibold">以下 {{ reconcileIssues.length }} 笔仍未到账</p>
      <p class="hint">
        商店每 3 分钟会自动向 NodeLoc 核实一次超过 10 分钟的待支付订单，无需守着点。
        <RouterLink to="/orders?attention=undelivered" class="accent-text underline-offset-2 hover:underline">
          查看已收款未交付 →
        </RouterLink>
      </p>
      <p v-if="reconcileViaCheckout" class="hint">
        本轮有几笔是用「下单」核实的：NodeLoc 的查单接口不接受服务器端调用，商店自己换了这条路，到账不受影响。
      </p>
      <ul class="space-y-1.5">
        <li v-for="item in reconcileIssues" :key="item.order_no" class="flex flex-wrap items-center gap-2 text-sm">
          <RouterLink :to="`/orders/${item.order_no}`" class="mono accent-text underline-offset-2 hover:underline">
            {{ item.order_no }}
          </RouterLink>
          <span v-if="item.provider_status" class="badge badge-neutral">{{ providerStatus(item.provider_status) }}</span>
          <span v-if="item.provider_via === 'reprocess'" class="badge badge-neutral">经由下单核实</span>
          <span v-if="item.detail || item.message" class="quiet text-xs">{{ item.detail || item.message }}</span>
        </li>
      </ul>
    </div>

    <div v-if="buyerId" class="flex flex-wrap items-center gap-2">
      <span class="chip chip-active">
        只看用户
        <RouterLink :to="`/users/${buyerId}`" class="underline-offset-2 hover:underline">
          {{ buyerName || `#${buyerId}` }}
        </RouterLink>
      </span>
      <button class="btn btn-quiet btn-sm" @click="clearBuyer">取消筛选</button>
    </div>

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
                <p class="empty-title">{{ needAttention ? '没有待交付的订单' : filtered ? '没有符合条件的订单' : '还没有订单' }}</p>
                <p class="empty-hint">
                  {{ needAttention ? '已收款的订单都已交付，商店还会每几分钟自动重试失败的那几笔。' : filtered ? '换个关键词或选择全部状态。' : '买家在前台下单后会出现在这里。' }}
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
            <td class="text-sm">
              <div class="flex items-center gap-2">
                <RouterLink
                  v-if="order.user_id"
                  :to="`/users/${order.user_id}`"
                  class="accent-text underline-offset-2 hover:underline"
                >
                  {{ order.user?.username || `#${order.user_id}` }}
                </RouterLink>
                <span v-else class="quiet">已删除用户</span>
                <button
                  v-if="order.user_id"
                  class="quiet text-xs transition-colors hover:accent-text"
                  :title="`只看 ${order.user?.username || '#' + order.user_id} 的订单`"
                  @click="pickBuyer(order.user_id, order.user?.username || '')"
                >
                  筛选
                </button>
              </div>
              <p v-if="order.user?.email" class="hint mono mt-0.5 truncate">{{ order.user.email }}</p>
            </td>
            <td class="max-w-[220px] truncate text-sm">{{ order.product?.name || `#${order.product_id}` }}</td>
            <td class="nums text-sm">
              {{ money(order.total_amount) }} <span class="quiet">×{{ order.quantity }}</span>
              <!-- The code and its cut belong on the list: when a buyer says
                   「优惠码没减」, the owner should see at a glance whether this
                   order carried one at all. -->
              <p v-if="order.discount_amount" class="mt-0.5 text-xs text-[var(--success)]">
                <span class="mono">{{ order.coupon_code || '优惠码' }}</span>
                -{{ money(order.discount_amount) }}
              </p>
            </td>
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
      :summary="summary"
      @change="goTo"
    />
  </section>
</template>
