<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PaginationFooter from '../components/PaginationFooter.vue'
import {
  batchAddCards,
  deleteCard,
  deleteCardsBatch,
  exportCards,
  generateCards,
  listAllCards,
  listLowStock,
  listProducts,
  setCardStatusBatch,
  updateCard,
} from '../api/products'
import { cardStatus, errorMessage, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Card, Product } from '../types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const PageSize = 50

const products = ref<Product[]>([])
const cards = ref<Card[]>([])
const total = ref(0)
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const query = ref('')
const statusFilter = ref('all')
const page = ref(1)
const showImport = ref(false)
const importText = ref('')
const importing = ref(false)
const showGenerate = ref(false)
const generateCount = ref(20)
const generatePrefix = ref('')
const generating = ref(false)
const selected = ref<number[]>([])
const exporting = ref(false)
// Products about to run dry, so the picker can flag the one the operator is
// about to pour new codes into. /admin/low-stock is priced on products:view,
// which a cards-only role does not hold, so it is only asked for when readable.
const lowStock = ref<Set<number>>(new Set())

const productId = computed(() => Number(route.params.id || 0))
const product = computed(() => products.value.find((item) => item.id === productId.value) || null)
const cardProducts = computed(() => products.value.filter((item) => item.product_type === 'card'))
const canManage = computed(() => auth.allows('cards', 'manage'))

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))
const from = computed(() => (cards.value.length ? (page.value - 1) * PageSize + 1 : 0))
const to = computed(() => (page.value - 1) * PageSize + cards.value.length)
// 这一页一张卡密都没有时只报总数，不报「第 0–40 张」那种范围。
const summary = computed(() =>
  cards.value.length ? `第 ${from.value}–${to.value} 张 · 共 ${total.value} 张` : `共 ${total.value} 张`,
)

// The batch bar needs one product: the endpoints move stock for a single
// catalogue item, and a mixed selection cannot be given one stock recount.
const batchable = computed(() => canManage.value && productId.value > 0 && selected.value.length > 0)

const selectionState = computed(() => {
  const visible = cards.value.map((card) => card.id)
  if (!visible.length) return 'none'
  const picked = visible.filter((id) => selected.value.includes(id)).length
  if (!picked) return 'none'
  return picked === visible.length ? 'all' : 'some'
})

async function loadProducts() {
  try {
    products.value = await listProducts()
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  }
}

async function loadLowStock() {
  if (!auth.allows('products', 'view')) return
  try {
    const { data } = await listLowStock()
    lowStock.value = new Set(data.map((item) => item.id))
  } catch {
    lowStock.value = new Set()
  }
}

async function loadCards() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAllCards({
      productId: productId.value || undefined,
      status: statusFilter.value,
      q: query.value,
      limit: PageSize,
      offset: (page.value - 1) * PageSize,
    })
    cards.value = result.data
    total.value = result.total
    // Ids from a previous page are outside what is on screen now; keeping them
    // would let a batch action reach cards the operator cannot see.
    const visible = new Set(result.data.map((card) => card.id))
    selected.value = selected.value.filter((id) => visible.has(id))
  } catch (err) {
    cards.value = []
    total.value = 0
    error.value = errorMessage(err, '加载卡密失败')
  } finally {
    loading.value = false
  }
}

// Card endpoints recompute stock_count, so the product row must be reloaded with the list.
async function reload() {
  await Promise.all([loadCards(), loadProducts(), loadLowStock()])
}

function open(id: number) {
  router.push(`/cards/${id}`)
}

function chooseProduct(event: Event) {
  const value = Number((event.target as HTMLSelectElement).value || 0)
  router.push(value ? `/cards/${value}` : '/cards')
}

function toggleCard(cardId: number) {
  selected.value = selected.value.includes(cardId)
    ? selected.value.filter((id) => id !== cardId)
    : [...selected.value, cardId]
}

function toggleAll() {
  const visible = cards.value.map((card) => card.id)
  selected.value = selectionState.value === 'all' ? [] : visible
}

async function submitImport() {
  const lines = importText.value
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
  if (!lines.length || !productId.value) return
  importing.value = true
  error.value = ''
  try {
    const result = await batchAddCards(productId.value, lines)
    const created = result.created?.length ?? 0
    const skipped = result.skipped ?? 0
    notice.value = `已导入 ${created} 条卡密${skipped ? `，跳过重复 ${skipped} 条` : ''}`
    importText.value = ''
    showImport.value = false
    await reload()
  } catch (err) {
    error.value = errorMessage(err, '导入卡密失败')
  } finally {
    importing.value = false
  }
}

async function submitGenerate() {
  if (!productId.value) return
  generating.value = true
  error.value = ''
  try {
    const result = await generateCards(productId.value, Number(generateCount.value) || 0, generatePrefix.value.trim())
    notice.value = `已生成 ${result.created?.length ?? 0} 条卡密${result.skipped ? `，${result.skipped} 条因重复被跳过` : ''}`
    showGenerate.value = false
    await reload()
  } catch (err) {
    error.value = errorMessage(err, '生成卡密失败')
  } finally {
    generating.value = false
  }
}

async function doExport() {
  exporting.value = true
  error.value = ''
  try {
    await exportCards({ productId: productId.value || undefined, status: statusFilter.value, q: query.value })
    notice.value = '已导出当前筛选结果的 CSV。'
  } catch (err) {
    error.value = errorMessage(err, '导出卡密失败')
  } finally {
    exporting.value = false
  }
}

async function toggleStatus(card: Card, status: string) {
  if (!productId.value) return
  busy.value = true
  error.value = ''
  try {
    await updateCard(productId.value, card.id, { content: card.content, status })
    await reload()
  } catch (err) {
    error.value = errorMessage(err, '更新卡密状态失败')
  } finally {
    busy.value = false
  }
}

async function removeCard(card: Card) {
  if (!productId.value) return
  if (!confirm(`删除卡密 #${card.id}？此操作不可撤销。`)) return
  busy.value = true
  try {
    await deleteCard(productId.value, card.id)
    await reload()
  } catch (err) {
    error.value = errorMessage(err, '删除卡密失败')
  } finally {
    busy.value = false
  }
}

async function batchStatus(status: string) {
  if (!batchable.value) return
  const label = status === 'disabled' ? '停用' : '启用'
  if (!confirm(`${label}选中的 ${selected.value.length} 张卡密？`)) return
  busy.value = true
  error.value = ''
  try {
    const result = await setCardStatusBatch(productId.value, selected.value, status)
    notice.value = `已${label} ${result.updated ?? 0} 张卡密`
    selected.value = []
    await reload()
  } catch (err) {
    error.value = errorMessage(err, `批量${label}失败`)
  } finally {
    busy.value = false
  }
}

async function batchDelete() {
  if (!batchable.value) return
  if (!confirm(`删除选中的 ${selected.value.length} 张卡密？此操作不可撤销。`)) return
  busy.value = true
  error.value = ''
  try {
    const result = await deleteCardsBatch(productId.value, selected.value)
    notice.value = `已删除 ${result.deleted ?? 0} 张卡密`
    selected.value = []
    await reload()
  } catch (err) {
    error.value = errorMessage(err, '批量删除失败')
  } finally {
    busy.value = false
  }
}

watch([statusFilter, query], () => {
  page.value = 1
  loadCards()
})

watch(page, loadCards)

watch(productId, () => {
  page.value = 1
  selected.value = []
  notice.value = ''
  loadCards()
})

onMounted(async () => {
  await Promise.all([loadProducts(), loadCards(), loadLowStock()])
})
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <RouterLink v-if="productId" to="/cards" class="quiet text-xs hover:text-[var(--text)]">← 全部卡密</RouterLink>
        <div class="mt-1 flex flex-wrap items-center gap-2">
          <h2 class="truncate text-lg font-semibold">
            {{ product?.name || '全部卡密' }}
            <span v-if="product" class="quiet text-sm font-normal">· 卡密库存</span>
          </h2>
          <span v-if="product" class="badge badge-accent mono">库存 {{ product.stock_count }}</span>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <label class="sr-only" for="card-product">按商品筛选</label>
        <select id="card-product" class="input w-52 !py-1.5 text-sm" :value="productId" @change="chooseProduct">
          <option :value="0">全部商品</option>
          <option v-for="item in cardProducts" :key="item.id" :value="item.id">
            {{ item.name }}（可用 {{ item.stock_count }}{{ lowStock.has(item.id) ? ' · 需补货' : '' }}）
          </option>
        </select>
        <template v-if="canManage">
          <button class="btn btn-secondary btn-sm" :disabled="exporting" @click="doExport">
            {{ exporting ? '导出中…' : '导出 CSV' }}
          </button>
          <button v-if="productId" class="btn btn-secondary btn-sm" @click="showImport = true">导入卡密</button>
          <button v-if="productId" class="btn btn-primary btn-sm" @click="showGenerate = true">生成卡密</button>
        </template>
      </div>
    </div>

    <p v-if="!canManage" class="quiet text-xs">当前角色只能查看卡密库存，导入、生成与清理需要「卡密管理」权限。</p>

    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="query"
          class="input w-64"
          type="search"
          placeholder="搜索卡密内容或 ID…"
          aria-label="搜索卡密"
        />
        <button
          v-for="tab in [
            { key: 'available', label: '可用' },
            { key: 'sold', label: '已售出' },
            { key: 'disabled', label: '已停用' },
          ]"
          :key="tab.key"
          class="chip"
          :class="statusFilter === tab.key ? 'chip-active' : ''"
          @click="statusFilter = statusFilter === tab.key ? 'all' : tab.key"
        >
          {{ tab.label }}
        </button>
      </div>
      <p class="quiet text-xs mono">共 {{ total }} 张</p>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="canManage && productId && selected.length" class="card-quiet flex flex-wrap items-center gap-2 !p-3">
      <span class="text-sm muted">已选 {{ selected.length }} 张</span>
      <button class="btn btn-quiet btn-sm" :disabled="busy" @click="batchStatus('disabled')">批量停用</button>
      <button class="btn btn-quiet btn-sm" :disabled="busy" @click="batchStatus('available')">批量启用</button>
      <button class="btn btn-quiet btn-sm text-[var(--danger)]" :disabled="busy" @click="batchDelete">批量删除</button>
      <button class="btn btn-ghost btn-sm ml-auto" @click="selected = []">取消选择</button>
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th v-if="canManage && productId" class="w-10">
              <input
                type="checkbox"
                class="accent-[var(--accent)]"
                aria-label="全选本页"
                :checked="selectionState === 'all'"
                @change="toggleAll"
              />
            </th>
            <th>ID</th>
            <th>卡密内容</th>
            <th v-if="!productId">商品</th>
            <th>状态</th>
            <th>订单</th>
            <th>售出时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 6" :key="`skeleton-${i}`">
              <td :colspan="8"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!cards.length">
            <td colspan="8">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">{{ query.trim() || statusFilter !== 'all' ? '没有符合条件的卡密' : '还没有卡密' }}</p>
                <p class="empty-hint">
                  {{ productId || !query.trim() ? '选择商品后可以导入、生成或批量清理卡密。' : '换个关键词或清空筛选再试。' }}
                </p>
              </div>
            </td>
          </tr>
          <tr v-for="card in cards" :key="card.id">
            <td v-if="canManage && productId">
              <input
                type="checkbox"
                class="accent-[var(--accent)]"
                :aria-label="`选择卡密 ${card.id}`"
                :checked="selected.includes(card.id)"
                @change="toggleCard(card.id)"
              />
            </td>
            <td class="nums text-sm quiet">{{ card.id }}</td>
            <td>
              <code class="mono block max-w-[360px] truncate text-xs">{{ card.content || '（不可见）' }}</code>
            </td>
            <td v-if="!productId" class="text-sm">
              <RouterLink
                v-if="card.product_name"
                :to="`/cards/${card.product_id}`"
                class="truncate hover:text-[var(--accent)]"
              >
                {{ card.product_name }}
              </RouterLink>
              <span v-else class="quiet">—</span>
            </td>
            <td>
              <span class="badge" :class="cardStatus(card.status).badge">{{ cardStatus(card.status).label }}</span>
            </td>
            <td class="mono text-sm quiet">{{ card.order_no || card.order_id || '—' }}</td>
            <td class="text-sm quiet">{{ when(card.sold_at) }}</td>
            <td>
              <div class="flex items-center gap-1">
                <button
                  v-if="canManage && productId && card.status === 'available'"
                  class="btn btn-ghost btn-sm"
                  :disabled="busy"
                  @click="toggleStatus(card, 'disabled')"
                >
                  停用
                </button>
                <button
                  v-else-if="canManage && productId && card.status === 'disabled'"
                  class="btn btn-ghost btn-sm"
                  :disabled="busy"
                  @click="toggleStatus(card, 'available')"
                >
                  启用
                </button>
                <span v-else-if="!canManage || !productId" class="px-2 text-xs quiet">—</span>
                <span v-else class="px-2 text-xs quiet">已售出</span>
                <button
                  v-if="canManage && productId"
                  class="btn btn-ghost btn-sm text-[var(--danger)]"
                  :disabled="busy"
                  @click="removeCard(card)"
                >
                  删除
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <PaginationFooter
      :page="page"
      :pages="pageCount"
      :loading="loading"
      :summary="summary"
      @change="page = $event"
    />

    <!-- Import overlay -->
    <div
      v-if="showImport"
      class="overlay" role="dialog" aria-modal="true" aria-label="导入卡密"
      @click.self="showImport = false"
    >
      <div class="card w-full max-w-lg !p-5">
        <h3 class="text-base font-semibold">导入卡密 · {{ product?.name }}</h3>
        <p class="quiet mt-1 text-xs">每行一条，空行与重复内容会被跳过。导入后商品库存会自动重算。</p>
        <textarea
          v-model="importText"
          class="input mono mt-4 h-56 resize-none text-xs"
          placeholder="CARD-XXXX-XXXX&#10;CARD-YYYY-YYYY"
        />
        <div class="mt-4 flex items-center justify-between">
          <span class="quiet text-xs mono">{{ importText.split('\n').filter((line) => line.trim()).length }} 条待导入</span>
          <div class="flex gap-2">
            <button class="btn btn-secondary btn-sm" @click="showImport = false">取消</button>
            <button
              class="btn btn-primary btn-sm"
              :disabled="importing || !importText.trim()"
              @click="submitImport"
            >
              {{ importing ? '导入中…' : '确认导入' }}
            </button>
          </div>
        </div>
      </div>
    </div>

    <!-- Generate overlay -->
    <div
      v-if="showGenerate"
      class="overlay" role="dialog" aria-modal="true" aria-label="生成卡密"
      @click.self="showGenerate = false"
    >
      <div class="card w-full max-w-sm !p-5">
        <h3 class="text-base font-semibold">生成卡密 · {{ product?.name }}</h3>
        <p class="quiet mt-1 text-xs">一次最多 500 条。前缀只允许字母、数字和短横线，例如 NL-</p>
        <div class="mt-4 space-y-3">
          <div>
            <label class="label" for="g-count">数量</label>
            <input id="g-count" v-model.number="generateCount" type="number" min="1" max="500" class="input nums w-32" />
          </div>
          <div>
            <label class="label" for="g-prefix">前缀（可选）</label>
            <input id="g-prefix" v-model="generatePrefix" class="input mono text-xs" placeholder="NL-" />
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showGenerate = false">取消</button>
          <button
            class="btn btn-primary btn-sm"
            :disabled="generating || !generateCount || generateCount < 1 || generateCount > 500"
            @click="submitGenerate"
          >
            {{ generating ? '生成中…' : '确认生成' }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
