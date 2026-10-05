<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import ManagementPage from '../components/ManagementPage.vue'
import StatusBadge from '../components/StatusBadge.vue'
import {
  alertLowStock,
  batchAddCards,
  deleteCardsBatch,
  exportCards,
  generateCards,
  listAllCards,
  listLowStock,
  listProducts,
  setCardStatusBatch,
  updateCard,
  deleteCard,
  type LowStockRow,
} from '../api/products'
import { errorMessage, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Card, Product } from '../types'

/**
 * 卡密库存。三件事在这里完成：导入/生成卡密、按状态筛选与批量操作、
 * 看哪些商品欠买家货。
 */

const PAGE_SIZE = 20

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const canManage = computed(() => auth.allows('cards', 'manage'))

const COLUMNS: Column[] = [
  { label: '', width: '40px' },
  { label: '卡密内容' },
  { label: '所属商品', hideOnMobile: true },
  { label: '状态' },
  { label: '售出时间', hideOnMobile: true },
  { label: '', actions: true, width: '160px' },
]

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const cards = ref<Card[]>([])
const total = ref(0)
const page = ref(1)
const products = ref<Product[]>([])
const productId = ref(0)
const statusFilter = ref('all')
const search = ref('')
const selected = ref<number[]>([])

const lowStock = ref<LowStockRow[]>([])
const alerting = ref(false)
const alertNotice = ref('')

const showImport = ref(false)
const showGenerate = ref(false)
const importText = ref('')
const importProduct = ref(0)
const generateProduct = ref(0)
const generateCount = ref(10)
const generatePrefix = ref('')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(() => Boolean(search.value.trim() || statusFilter.value !== 'all' || productId.value))
const allSelected = computed(() => cards.value.length > 0 && selected.value.length === cards.value.length)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, stock] = await Promise.all([
      listAllCards({
        productId: productId.value || undefined,
        status: statusFilter.value,
        q: search.value,
        limit: PAGE_SIZE,
        offset: (page.value - 1) * PAGE_SIZE,
      }),
      listLowStock().then((r) => r.data).catch(() => [] as LowStockRow[]),
    ])
    cards.value = list.data
    total.value = list.total
    lowStock.value = stock
    selected.value = selected.value.filter((id) => list.data.some((c) => c.id === id))
  } catch (err) {
    error.value = errorMessage(err, '加载卡密失败')
  } finally {
    loading.value = false
  }
}

async function loadProducts() {
  products.value = await listProducts().catch(() => [] as Product[])
}

function apply() {
  page.value = 1
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

function clearFilters() {
  search.value = ''
  statusFilter.value = 'all'
  productId.value = 0
  apply()
}

function toggleAll() {
  selected.value = allSelected.value ? [] : cards.value.map((c) => c.id)
}

function toggleOne(id: number) {
  selected.value = selected.value.includes(id) ? selected.value.filter((x) => x !== id) : [...selected.value, id]
}

/** 批量操作只对当前商品生效；跨商品的选中先按商品分组再逐组提交。 */
async function batchStatus(status: string) {
  const ids = selected.value
  if (!ids.length) return
  busy.value = true
  notice.value = ''
  try {
    const groups = new Map<number, number[]>()
    for (const id of ids) {
      const card = cards.value.find((c) => c.id === id)
      if (!card) continue
      const list = groups.get(card.product_id) ?? []
      list.push(id)
      groups.set(card.product_id, list)
    }
    let released = 0
    for (const [pid, idsInGroup] of groups) {
      const result = await setCardStatusBatch(pid, idsInGroup, status)
      released += result.released ?? 0
    }
    notice.value = '已更新 ' + ids.length + ' 张卡密' + (released ? '，并自动发出 ' + released + ' 笔等待中的订单' : '')
    selected.value = []
    await load()
  } catch (err) {
    error.value = errorMessage(err, '批量更新失败')
  } finally {
    busy.value = false
  }
}

async function batchDelete() {
  if (!selected.value.length) return
  if (!window.confirm('删除选中的 ' + selected.value.length + ' 张卡密？已售出的卡密不会被删除。')) return
  busy.value = true
  try {
    const groups = new Map<number, number[]>()
    for (const id of selected.value) {
      const card = cards.value.find((c) => c.id === id)
      if (!card) continue
      const list = groups.get(card.product_id) ?? []
      list.push(id)
      groups.set(card.product_id, list)
    }
    for (const [pid, idsInGroup] of groups) {
      await deleteCardsBatch(pid, idsInGroup)
    }
    notice.value = '已删除选中的卡密'
    selected.value = []
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除失败')
  } finally {
    busy.value = false
  }
}

async function toggleOneStatus(card: Card) {
  busy.value = true
  try {
    await updateCard(card.product_id, card.id, { status: card.status === 'available' ? 'disabled' : 'available' } as Partial<Card>)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '更新卡密状态失败')
  } finally {
    busy.value = false
  }
}

async function removeOne(card: Card) {
  if (!window.confirm('删除这张卡密？')) return
  busy.value = true
  try {
    await deleteCard(card.product_id, card.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除卡密失败')
  } finally {
    busy.value = false
  }
}

async function submitImport() {
  const lines = importText.value.split('\n').map((l) => l.trim()).filter(Boolean)
  if (!importProduct.value || !lines.length) {
    error.value = '请选择商品并粘贴至少一行卡密。'
    return
  }
  busy.value = true
  try {
    const result = await batchAddCards(importProduct.value, lines)
    notice.value =
      '已导入 ' + result.created.length + ' 张卡密' +
      (result.duplicates ? '（其中 ' + result.duplicates + ' 张是重复键，已按现有规则照常入库）' : '') +
      (result.blank ? '，忽略空白行 ' + result.blank + ' 行' : '') +
      (result.released ? '，并自动发出 ' + result.released + ' 笔等待中的订单' : '')
    importText.value = ''
    showImport.value = false
    await load()
  } catch (err) {
    error.value = errorMessage(err, '导入失败')
  } finally {
    busy.value = false
  }
}

async function submitGenerate() {
  if (!generateProduct.value || generateCount.value < 1) {
    error.value = '请选择商品并填写生成数量。'
    return
  }
  busy.value = true
  try {
    const result = await generateCards(generateProduct.value, generateCount.value, generatePrefix.value)
    notice.value =
      '已生成 ' + result.created.length + ' 张卡密' +
      (result.released ? '，并自动发出 ' + result.released + ' 笔等待中的订单' : '')
    showGenerate.value = false
    await load()
  } catch (err) {
    error.value = errorMessage(err, '生成失败')
  } finally {
    busy.value = false
  }
}

async function download() {
  try {
    await exportCards({ productId: productId.value || undefined, status: statusFilter.value, q: search.value })
  } catch (err) {
    error.value = errorMessage(err, '导出失败')
  }
}

async function warnRestock() {
  if (alerting.value) return
  alerting.value = true
  try {
    const result = await alertLowStock()
    alertNotice.value = !result.checked
      ? '巡检没有需要提醒的商品。'
      : result.sent
        ? '已发出 ' + result.sent + ' 条提醒。'
        : '今天已经提醒过 ' + result.checked + ' 件缺货商品了。'
  } catch (err) {
    alertNotice.value = errorMessage(err, '提醒失败')
  } finally {
    alerting.value = false
  }
}

onMounted(async () => {
  if (typeof route.query.product === 'string') productId.value = Number(route.query.product) || 0
  if (typeof route.query.status === 'string') statusFilter.value = route.query.status
  await Promise.all([loadProducts(), load()])
})
</script>

<template>
  <section class="space-y-4">
    <ManagementPage
      title="卡密库存"
      description="导入或生成卡密、按状态筛选与批量操作。补货后等待中的订单会自动发货。"
      eyebrow="库存与交付"
      :metrics="[
        { label: '当前结果', value: total, hint: '筛选后的卡密总数' },
        { label: '库存告急', value: lowStock.length, hint: '需要补货的商品', tone: 'warning' },
        { label: '已选', value: selected.length, hint: '可批量处理的卡密' },
      ]"
    >
      <template #actions>
        <button v-if="canManage" class="btn btn-primary btn-sm" @click="showImport = true; importProduct = productId">
          <AdminIcon name="plus" :size="14" />
          导入卡密
        </button>
        <button v-if="canManage" class="btn btn-secondary btn-sm" @click="showGenerate = true; generateProduct = productId">
          批量生成
        </button>
        <button class="btn btn-quiet btn-sm" @click="download">
          <AdminIcon name="download" :size="14" />
          导出
        </button>
      </template>
    </ManagementPage>

    <!-- 欠货提示：只有真的欠着买家才出现 -->
    <div v-if="lowStock.length" class="card space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <AdminIcon name="alert" :size="16" class="text-[var(--warning)]" />
          <p class="text-sm font-semibold">{{ lowStock.length }} 件商品库存告急</p>
        </div>
        <button class="btn btn-quiet btn-sm" :disabled="alerting" @click="warnRestock">
          {{ alerting ? '发送中…' : '提醒补货' }}
        </button>
      </div>
      <p v-if="alertNotice" class="quiet text-xs">{{ alertNotice }}</p>
      <ul class="grid gap-2 sm:grid-cols-2">
        <li v-for="row in lowStock.slice(0, 6)" :key="row.id" class="card-quiet flex items-center gap-2">
          <span class="min-w-0 flex-1">
            <span class="block truncate text-[13px] font-semibold">{{ row.name }}</span>
            <span class="quiet text-[11px]">
              剩余 <span class="nums">{{ row.stock_count }}</span>
              <span v-if="row.waiting_orders"> · 已付待发 <span class="nums text-[var(--warning)]">{{ row.waiting_orders }}</span></span>
            </span>
          </span>
          <RouterLink :to="'/cards/' + row.id" class="btn btn-secondary btn-sm">补货</RouterLink>
        </li>
      </ul>
    </div>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <FilterBar :count="total ? '共 ' + total + ' 张卡密' : ''">
      <input v-model="search" class="input w-52" type="search" placeholder="搜索卡密内容" aria-label="搜索卡密" @keyup.enter="apply" />
      <select v-model.number="productId" class="input !w-auto" aria-label="所属商品" @change="apply">
        <option :value="0">全部商品</option>
        <option v-for="p in products" :key="p.id" :value="p.id">{{ p.name }}</option>
      </select>
      <select v-model="statusFilter" class="input !w-auto" aria-label="卡密状态" @change="apply">
        <option value="all">全部状态</option>
        <option value="available">可用</option>
        <option value="sold">已售出</option>
        <option value="disabled">已停用</option>
      </select>
      <button class="btn btn-secondary btn-sm" @click="apply">查询</button>
      <template #actions>
        <template v-if="selected.length && canManage">
          <span class="nums badge-info self-center">已选 {{ selected.length }}</span>
          <button class="btn btn-quiet btn-sm" :disabled="busy" @click="batchStatus('available')">批量启用</button>
          <button class="btn btn-quiet btn-sm" :disabled="busy" @click="batchStatus('disabled')">批量停用</button>
          <button class="btn btn-danger btn-sm" :disabled="busy" @click="batchDelete">批量删除</button>
        </template>
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
      :summary="'共 ' + total + ' 张卡密'"
      empty-title="还没有卡密"
      empty-hint="点右上角导入或批量生成，卡密入库后等待中的订单会自动发货。"
      @retry="load"
      @clear-filters="clearFilters"
      @change="goPage"
    >
      <tr v-for="card in cards" :key="card.id">
        <td>
          <input
            type="checkbox"
            :checked="selected.includes(card.id)"
            :aria-label="'选择卡密 ' + card.id"
            @change="toggleOne(card.id)"
          />
        </td>
        <td class="mono max-w-[320px] truncate text-[12.5px]">{{ card.content }}</td>
        <td class="hide-on-mobile max-w-[200px] truncate">
          {{ products.find((p) => p.id === card.product_id)?.name || '#' + card.product_id }}
        </td>
        <td><StatusBadge :value="card.status || 'available'" :label="card.status === 'sold' ? '已售出' : card.status === 'disabled' ? '已停用' : '可用'" /></td>
        <td class="quiet hide-on-mobile text-xs">{{ card.sold_at ? when(card.sold_at) : '—' }}</td>
        <td class="text-right">
          <div v-if="card.status !== 'sold'" class="flex flex-wrap justify-end gap-1.5">
            <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggleOneStatus(card)">
              {{ card.status === 'disabled' ? '启用' : '停用' }}
            </button>
            <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="removeOne(card)">删除</button>
          </div>
          <span v-else class="quiet text-xs">已交付</span>
        </td>
      </tr>
      <tr v-if="cards.length">
        <td colspan="6">
          <label class="flex items-center gap-2 text-xs">
            <input type="checkbox" :checked="allSelected" @change="toggleAll" />
            选中本页全部
          </label>
        </td>
      </tr>
    </DataTable>

    <!-- 导入 -->
    <AppDrawer :open="showImport" title="导入卡密" @close="showImport = false">
      <template #feedback>
        <p v-if="error" class="alert alert-danger mb-3" role="alert">{{ error }}</p>
      </template>
      <div class="space-y-3">
        <div>
          <label class="label" for="imp-product">选择商品</label>
          <select id="imp-product" v-model.number="importProduct" class="input">
            <option :value="0">请选择商品</option>
            <option v-for="p in products" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <div>
          <label class="label" for="imp-text">卡密内容（每行一张）</label>
          <textarea id="imp-text" v-model="importText" class="input mono min-h-[220px] text-xs" placeholder="CARD-XXXX-YYYY&#10;CARD-ZZZZ-WWWW" />
        </div>
        <p class="quiet text-xs">每行一张。重复键也会照常入库，并按现有库存规则逐张交付；空白行会被忽略。</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showImport = false">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="submitImport">
            {{ busy ? '导入中…' : '导入' }}
          </button>
        </div>
      </template>
    </AppDrawer>

    <!-- 生成 -->
    <AppDrawer :open="showGenerate" title="批量生成卡密" width="sm" @close="showGenerate = false">
      <template #feedback>
        <p v-if="error" class="alert alert-danger mb-3" role="alert">{{ error }}</p>
      </template>
      <div class="space-y-3">
        <div>
          <label class="label" for="gen-product">选择商品</label>
          <select id="gen-product" v-model.number="generateProduct" class="input">
            <option :value="0">请选择商品</option>
            <option v-for="p in products" :key="p.id" :value="p.id">{{ p.name }}</option>
          </select>
        </div>
        <div>
          <label class="label" for="gen-count">生成数量</label>
          <input id="gen-count" v-model.number="generateCount" class="input nums" type="number" min="1" max="500" />
        </div>
        <div>
          <label class="label" for="gen-prefix">前缀（可选）</label>
          <input id="gen-prefix" v-model="generatePrefix" class="input mono text-xs" placeholder="NL-" maxlength="16" />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showGenerate = false">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="submitGenerate">
            {{ busy ? '生成中…' : '生成' }}
          </button>
        </div>
      </template>
    </AppDrawer>
  </section>
</template>
