<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { listOrders, reconcileOrder, reconcilePendingOrders, exportOrders, type ReconcileReport } from '../api/orders'
import { money, when, errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Order } from '../types'

/**
 * 订单管理：筛选条件全部放在地址栏，刷新与分享都能还原同一个列表。
 * 批量查单会真的向 NodeLoc 询问每一笔待支付订单，结果在抽屉里逐条列出。
 */

const PAGE_SIZE = 20

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const canManage = computed(() => auth.allows('orders', 'manage'))

const COLUMNS: Column[] = [
  { label: '订单' },
  { label: '商品', hideOnMobile: true },
  { label: '买家', hideOnMobile: true },
  { label: '金额', numeric: true },
  { label: '状态' },
  { label: '发货' },
  { label: '', actions: true, width: '180px' },
]

const loading = ref(true)
const error = ref('')
const notice = ref('')
const orders = ref<Order[]>([])
const total = ref(0)
const page = ref(1)
const status = ref('')
const attention = ref('')
const search = ref('')
const buyerId = ref(0)
const buyerName = ref('')

const reconciling = ref(false)
const reconcilingOrder = ref('')
const report = ref<ReconcileReport | null>(null)
const showReport = ref(false)
const exporting = ref(false)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(
  () => Boolean(status.value || attention.value || search.value.trim() || buyerId.value),
)

const STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'pending', label: '待支付' },
  { value: 'paid', label: '已支付' },
  { value: 'completed', label: '已完成' },
  { value: 'cancelled', label: '已取消' },
  { value: 'refunded', label: '已退款' },
  { value: 'failed', label: '支付失败' },
]

const FULFILLMENT_LABEL: Record<string, string> = {
  pending: '待发货',
  delivered: '已发货',
  completed: '已完成',
  manual_pending: '人工待发',
  waiting_stock: '等待补货',
  plugin_pending: '交付中',
  failed: '发货异常',
}

/** 把当前筛选写回地址栏，让「已支付 + 某买家 + 第 3 页」成为可分享的链接。 */
function syncUrl() {
  const query: Record<string, string> = {}
  if (status.value) query.status = status.value
  if (attention.value) query.attention = attention.value
  if (search.value.trim()) query.q = search.value.trim()
  if (buyerId.value) query.user = String(buyerId.value)
  if (page.value > 1) query.page = String(page.value)
  void router.replace({ path: '/orders', query })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listOrders({
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE,
      status: status.value || undefined,
      q: search.value.trim() || undefined,
      attention: attention.value || undefined,
      user_id: buyerId.value || undefined,
    })
    orders.value = result.data
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载订单失败')
  } finally {
    loading.value = false
  }
}

function apply() {
  page.value = 1
  syncUrl()
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  syncUrl()
  void load()
}

function clearFilters() {
  status.value = ''
  attention.value = ''
  search.value = ''
  buyerId.value = 0
  buyerName.value = ''
  apply()
}

function toggleAttention() {
  attention.value = attention.value === 'undelivered' ? '' : 'undelivered'
  apply()
}

function pickBuyer(id: number, name: string) {
  buyerId.value = id
  buyerName.value = name
  apply()
}

/** 批量查单：让服务端把这批待支付订单拿去问 NodeLoc，结果逐条展示。 */
async function reconcile() {
  if (reconciling.value) return
  reconciling.value = true
  error.value = ''
  try {
    report.value = await reconcilePendingOrders()
    showReport.value = true
    await load()
  } catch (err) {
    error.value = errorMessage(err, '批量查单失败')
  } finally {
    reconciling.value = false
  }
}

async function reconcileOne(order: Order) {
  if (reconcilingOrder.value) return
  reconcilingOrder.value = order.order_no
  error.value = ''
  notice.value = ''
  try {
    const result = await reconcileOrder(order.order_no)
    notice.value = result.settled
      ? `订单 ${order.order_no} 已确认到账并进入交付流程。`
      : `订单 ${order.order_no} 尚未到账，NodeLoc 状态：${result.provider_status || '待支付'}。`
    await load()
  } catch (err) {
    error.value = errorMessage(err, '查单失败')
  } finally {
    reconcilingOrder.value = ''
  }
}

async function download() {
  if (exporting.value) return
  exporting.value = true
  try {
    await exportOrders({
      status: status.value || undefined,
      q: search.value.trim() || undefined,
      attention: attention.value || undefined,
      user_id: buyerId.value || undefined,
    })
  } catch (err) {
    error.value = errorMessage(err, '导出失败')
  } finally {
    exporting.value = false
  }
}

onMounted(() => {
  const query = route.query
  if (typeof query.status === 'string') status.value = query.status
  if (typeof query.attention === 'string') attention.value = query.attention
  if (typeof query.q === 'string') search.value = query.q
  if (typeof query.user === 'string') buyerId.value = Number(query.user) || 0
  if (typeof query.page === 'string') page.value = Math.max(1, Number(query.page) || 1)
  void load()
})
</script>

<template>
  <section class="space-y-4">
    <PageHeader
      title="订单管理"
      description="查看支付与发货状态、重试自动发货、人工补发或退款。批量查单会向 NodeLoc 核对待支付订单。"
      bordered
    >
      <template #actions>
        <button class="btn btn-quiet btn-sm" :disabled="reconciling" @click="reconcile">
          <span v-if="reconciling" class="spinner !size-3.5" />
          {{ reconciling ? '核对中…' : '批量查单' }}
        </button>
        <button class="btn btn-quiet btn-sm" :disabled="exporting" @click="download">
          <AdminIcon name="download" :size="14" />
          {{ exporting ? '导出中…' : '导出 CSV' }}
        </button>
      </template>
    </PageHeader>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <FilterBar :count="total ? '共 ' + total + ' 笔订单' : ''">
      <input
        v-model="search"
        class="input w-52"
        type="search"
        placeholder="订单号 / 交易号 / 商品"
        aria-label="搜索订单"
        @keyup.enter="apply"
      />
      <select v-model="status" class="input !w-auto" aria-label="订单状态" @change="apply">
        <option v-for="option in STATUS_OPTIONS" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <button class="chip" :class="attention === 'undelivered' ? 'chip-active' : ''" @click="toggleAttention">
        仅看未发货
      </button>
      <button v-if="buyerId" class="chip chip-active" @click="pickBuyer(0, '')">
        买家：{{ buyerName || buyerId }} ✕
      </button>
      <button class="btn btn-secondary btn-sm" @click="apply">查询</button>
      <template #actions>
        <button v-if="filtered" class="btn btn-quiet btn-sm" @click="clearFilters">清除筛选</button>
      </template>
    </FilterBar>

    <DataTable
      :columns="COLUMNS"
      :loading="loading"
      :error="error"
      :filtered="filtered"
      :page="page"
      :pages="pageCount"
      :total="total"
      :summary="'共 ' + total + ' 笔订单'"
      empty-title="还没有订单"
      empty-hint="买家下单后会出现在这里。"
      @retry="load"
      @clear-filters="clearFilters"
      @change="goPage"
    >
      <tr v-for="order in orders" :key="order.id">
        <td>
          <RouterLink :to="'/orders/' + order.order_no" class="mono text-[12.5px] hover:accent-text">
            {{ order.order_no }}
          </RouterLink>
          <p v-if="order.transaction_id" class="quiet mono mt-0.5 text-[11px]">交易 {{ order.transaction_id }}</p>
        </td>
        <td class="hide-on-mobile max-w-[220px] truncate">
          {{ order.product?.name || '—' }}
          <span v-if="(order.quantity ?? 1) > 1" class="nums quiet">×{{ order.quantity }}</span>
        </td>
        <td class="hide-on-mobile">
          <RouterLink v-if="order.user_id" :to="'/users/' + order.user_id" class="hover:accent-text">
            {{ order.user?.username || '#' + order.user_id }}
          </RouterLink>
        </td>
        <td class="nums">{{ money(order.total_amount) }}</td>
        <td><StatusBadge :value="order.status" /></td>
        <td>
          <StatusBadge
            :value="order.fulfillment_status || 'pending'"
            :label="FULFILLMENT_LABEL[order.fulfillment_status || 'pending'] || order.fulfillment_status || '待发货'"
          />
        </td>
        <td class="text-right">
          <div class="flex flex-wrap justify-end gap-1.5">
            <RouterLink :to="'/orders/' + order.order_no" class="btn btn-quiet btn-sm">详情</RouterLink>
            <button
              v-if="canManage && order.status === 'pending'"
              class="btn btn-quiet btn-sm"
              :disabled="reconcilingOrder === order.order_no"
              @click="reconcileOne(order)"
            >
              {{ reconcilingOrder === order.order_no ? '查询中…' : '查单' }}
            </button>
          </div>
        </td>
      </tr>
    </DataTable>

    <!-- 批量查单结果 -->
    <AppDrawer :open="showReport" title="批量查单结果" @close="showReport = false">
      <div v-if="report" class="space-y-3">
        <div class="grid gap-3 sm:grid-cols-3">
          <div class="card-quiet">
            <p class="quiet text-xs">已核对</p>
            <p class="nums mt-1 text-lg font-bold">{{ report.checked }}</p>
          </div>
          <div class="card-quiet">
            <p class="quiet text-xs">本次入账</p>
            <p class="nums mt-1 text-lg font-bold accent-text">{{ report.settled }}</p>
          </div>
          <div class="card-quiet">
            <p class="quiet text-xs">核对时间</p>
            <p class="mt-1 text-[13px]">{{ when(report.checked_at) }}</p>
          </div>
        </div>

        <p v-if="!report.items.length" class="quiet py-8 text-center text-sm">没有需要核对的待支付订单</p>
        <ul v-else class="space-y-2">
          <li v-for="item in report.items" :key="item.order_no" class="card-quiet space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <span class="mono text-[12.5px]">{{ item.order_no }}</span>
              <StatusBadge :value="item.settled ? 'paid' : 'pending'" :label="item.settled ? '已入账' : '仍未支付'" />
              <span v-if="item.provider_via" class="badge-neutral">{{ item.provider_via }}</span>
            </div>
            <p v-if="item.message" class="text-xs text-[var(--text-dim)]">{{ item.message }}</p>
            <p v-if="item.provider_note" class="quiet text-[11px]">NodeLoc 原文：{{ item.provider_note }}</p>
          </li>
        </ul>
      </div>
    </AppDrawer>
  </section>
</template>
