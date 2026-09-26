<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import PaginationFooter from '../components/PaginationFooter.vue'
import { batchAddCards, deleteCard, listCards, listProducts, updateCard } from '../api/products'
import { cardStatus, errorMessage, when } from '../utils/format'
import type { Card, Product } from '../types'

const route = useRoute()
const router = useRouter()

const products = ref<Product[]>([])
const cards = ref<Card[]>([])
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

const PageSize = 25

const productId = computed(() => Number(route.params.id || 0))
const product = computed(() => products.value.find((item) => item.id === productId.value) || null)
const cardProducts = computed(() => products.value.filter((item) => item.product_type === 'card'))

const counts = computed(() => ({
  available: cards.value.filter((card) => card.status === 'available').length,
  sold: cards.value.filter((card) => card.status === 'sold').length,
  disabled: cards.value.filter((card) => card.status === 'disabled').length,
}))

const filtered = computed(() => {
  const needle = query.value.trim().toLowerCase()
  return cards.value.filter((card) => {
    if (statusFilter.value !== 'all' && card.status !== statusFilter.value) return false
    if (!needle) return true
    return (card.content || '').toLowerCase().includes(needle) || String(card.id) === needle
  })
})

const paged = computed(() => {
  const start = (page.value - 1) * PageSize
  return filtered.value.slice(start, start + PageSize)
})

const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / PageSize)))
const pageSummary = computed(() => {
  const first = filtered.value.length ? (page.value - 1) * PageSize + 1 : 0
  return `第 ${first}–${first - 1 + paged.value.length} 张 · 共 ${filtered.value.length} 张`
})

async function loadProducts() {
  try {
    products.value = await listProducts()
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  }
}

async function loadCards() {
  if (!productId.value) {
    loading.value = false
    return
  }
  loading.value = true
  error.value = ''
  try {
    cards.value = await listCards(productId.value)
  } catch (err) {
    cards.value = []
    error.value = errorMessage(err, '加载卡密失败')
  } finally {
    loading.value = false
  }
}

// Card endpoints recompute stock_count, so the product row must be reloaded with the list.
async function reload() {
  await Promise.all([loadCards(), loadProducts()])
}

function open(id: number) {
  router.push(`/cards/${id}`)
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
    notice.value = `已导入 ${result.count ?? lines.length} 条卡密`
    importText.value = ''
    showImport.value = false
    await reload()
  } catch (err) {
    error.value = errorMessage(err, '导入卡密失败')
  } finally {
    importing.value = false
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

watch(productId, () => {
  page.value = 1
  query.value = ''
  statusFilter.value = 'all'
  notice.value = ''
  loadCards()
})

onMounted(async () => {
  await loadProducts()
  await loadCards()
})
</script>

<template>
  <!-- Product picker -->
  <section v-if="!productId" class="space-y-4">
    <p class="quiet text-sm">选择一个卡密商品以导入、查看或清理卡密。人工交付商品无需卡密库存。</p>
    <div v-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <div v-for="i in 6" :key="i" class="skeleton h-28" />
    </div>
    <p v-else-if="!cardProducts.length" class="card py-12 text-center text-sm quiet">
      还没有卡密商品，先在<u class="mx-1 inline"><RouterLink to="/products/new" class="accent-text">商品管理</RouterLink></u>创建。
    </p>
    <div v-else class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
      <button
        v-for="item in cardProducts"
        :key="item.id"
        class="card card-hover !p-5 text-left"
        @click="open(item.id)"
      >
        <p class="eyebrow mono">#{{ item.id }}</p>
        <p class="mt-2 truncate font-semibold">{{ item.name }}</p>
        <p class="quiet mt-1 truncate text-xs">{{ item.slug }}</p>
        <div class="mt-4 flex items-center gap-2">
          <span class="badge" :class="item.stock_count > 0 ? 'badge-success' : 'badge-danger'">
            可用 {{ item.stock_count }}
          </span>
          <span class="chip">管理卡密 →</span>
        </div>
      </button>
    </div>
  </section>

  <!-- Card management -->
  <section v-else class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="min-w-0">
        <RouterLink to="/cards" class="quiet text-xs hover:text-[var(--text)]">← 全部卡密商品</RouterLink>
        <div class="mt-1 flex flex-wrap items-center gap-2">
          <h2 class="truncate text-lg font-semibold">{{ product?.name || `商品 #${productId}` }}</h2>
          <span class="badge badge-accent mono">库存 {{ product?.stock_count ?? 0 }}</span>
        </div>
      </div>
      <button class="btn btn-primary btn-sm" @click="showImport = true">导入卡密</button>
    </div>

    <div class="grid gap-3 sm:grid-cols-3">
      <button
        v-for="tab in [
          { key: 'available', label: '可用' },
          { key: 'sold', label: '已售出' },
          { key: 'disabled', label: '已停用' },
        ]"
        :key="tab.key"
        class="card-quiet flex items-center justify-between px-4 py-3 text-left transition-colors hover:border-[var(--stroke-hi)]"
        :class="statusFilter === tab.key ? 'border-[var(--accent-line)] bg-[var(--accent-soft)]' : 'border border-[var(--stroke)]'"
        @click="statusFilter = statusFilter === tab.key ? 'all' : tab.key; page = 1"
      >
        <span class="text-sm muted">{{ tab.label }}</span>
        <span class="nums text-lg font-semibold">{{ counts[tab.key as keyof typeof counts] }}</span>
      </button>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-3">
      <input
        v-model="query"
        class="input w-64"
        type="search"
        placeholder="搜索卡密内容或 ID…"
        aria-label="搜索卡密"
        @input="page = 1"
      />
      <p class="quiet text-xs mono">
        {{ filtered.length }} / {{ cards.length }} 条
      </p>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>卡密内容</th>
            <th>状态</th>
            <th>订单</th>
            <th>售出时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 5" :key="`skeleton-${i}`">
              <td colspan="6"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!paged.length">
            <td colspan="6">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">没有符合条件的卡密</p>
              </div>
            </td>
          </tr>
          <tr v-for="card in paged" :key="card.id">
            <td class="nums text-sm quiet">{{ card.id }}</td>
            <td>
              <code class="mono block max-w-[360px] truncate text-xs">{{ card.content || '（不可见）' }}</code>
            </td>
            <td>
              <span class="badge" :class="cardStatus(card.status).badge">{{ cardStatus(card.status).label }}</span>
            </td>
            <td class="nums text-sm quiet">{{ card.order_id || '—' }}</td>
            <td class="text-sm quiet">{{ when(card.sold_at) }}</td>
            <td>
              <div class="flex items-center gap-1">
                <button
                  v-if="card.status === 'available'"
                  class="btn btn-ghost btn-sm"
                  :disabled="busy"
                  @click="toggleStatus(card, 'disabled')"
                >
                  停用
                </button>
                <button
                  v-else-if="card.status === 'disabled'"
                  class="btn btn-ghost btn-sm"
                  :disabled="busy"
                  @click="toggleStatus(card, 'available')"
                >
                  启用
                </button>
                <span v-else class="px-2 text-xs quiet">—</span>
                <button class="btn btn-ghost btn-sm text-[var(--danger)]" :disabled="busy" @click="removeCard(card)">
                  删除
                </button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <PaginationFooter :page="page" :pages="totalPages" :summary="pageSummary" @change="page = $event" />

    <!-- Import overlay -->
    <div
      v-if="showImport"
      class="overlay" role="dialog" aria-modal="true" aria-label="导入卡密"
      @click.self="showImport = false"
    >
      <div class="card w-full max-w-lg !p-5">
        <h3 class="text-base font-semibold">导入卡密 · {{ product?.name }}</h3>
        <p class="quiet mt-1 text-xs">每行一条，空行自动忽略。导入后商品库存会自动重算。</p>
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
  </section>
</template>
