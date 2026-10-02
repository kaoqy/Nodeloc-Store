<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ProductCard from '../components/ProductCard.vue'
import PromoStrip from '../components/PromoStrip.vue'
import { listCategories, listProducts, storeStats } from '../api/products'
import { errorMessage } from '../api/client'
import { useSiteStore } from '../stores/site'
import type { Category, Product, StoreStats } from '../types'

const PAGE_SIZE = 12

const site = useSiteStore()
const route = useRoute()
const router = useRouter()

const products = ref<Product[]>([])
const categories = ref<Category[]>([])
const stats = ref<StoreStats | null>(null)
const total = ref(0)
const keyword = ref('')
const activeCategory = ref<number | ''>('')
const sortBy = ref('default')
const featuredOnly = ref(false)
const inStockOnly = ref(false)
const page = ref(1)
const loading = ref(true)
const error = ref('')
const hidden = ref('')

const SORTS: { key: string; label: string }[] = [
  { key: 'default', label: '默认排序' },
  { key: 'sales', label: '销量优先' },
  { key: 'price_asc', label: '价格从低到高' },
  { key: 'price_desc', label: '价格从高到低' },
  { key: 'newest', label: '最新上架' },
]

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

/**
 * A dismissal belongs to one announcement, not to the page: the shop owner may
 * post a new notice at any time and buyers should see it.
 */
const announcement = computed(() =>
  site.announcement && hidden.value !== dismissKey(site.announcement) ? site.announcement : '',
)

function dismissKey(value: string) {
  return `announcement:${value}`
}

// The site identity arrives after this page mounts, so the dismissal has to be
// re-read when it does — checking it once at mount looks in an empty store and
// means a buyer who closed the notice sees it again on every refresh.
watch(
  () => site.announcement,
  (text) => {
    hidden.value = text && sessionStorage.getItem(dismissKey(text)) ? dismissKey(text) : ''
  },
  { immediate: true },
)

function hideAnnouncement() {
  if (!site.announcement) return
  hidden.value = dismissKey(site.announcement)
  sessionStorage.setItem(hidden.value, '1')
}

// Two watches can fire a reload back to back (a sort chip click plus the keyword
// debounce), so the responses arrive in any order. Only the newest one may write.
let requestId = 0

async function load() {
  const request = ++requestId
  loading.value = true
  error.value = ''
  try {
    const list = await listProducts({
      q: keyword.value,
      sort: sortBy.value,
      category: activeCategory.value === '' ? undefined : activeCategory.value,
      featured: featuredOnly.value,
      inStock: inStockOnly.value,
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE,
    })
    if (request !== requestId) return
    const pages = Math.max(1, Math.ceil(list.total / PAGE_SIZE))
    // A narrower filter can leave the current page past the last one; step back
    // and let the page watcher fetch the window that does exist.
    if (page.value > pages) {
      total.value = list.total
      products.value = []
      loading.value = false
      page.value = pages
      return
    }
    products.value = list.data
    total.value = list.total
  } catch (e) {
    if (request !== requestId) return
    error.value = errorMessage(e, '商品加载失败，请稍后重试')
  } finally {
    if (request === requestId) loading.value = false
  }
}

/**
 * Filters live in the address, so "所有 VPN 分类，按销量" is a link a buyer can
 * send to someone instead of a state only their browser knew.
 */
function syncUrl() {
  const query: Record<string, string> = {}
  if (keyword.value.trim()) query.q = keyword.value.trim()
  if (activeCategory.value !== '') query.category = String(activeCategory.value)
  if (sortBy.value !== 'default') query.sort = sortBy.value
  if (featuredOnly.value) query.featured = '1'
  if (inStockOnly.value) query.in_stock = '1'
  if (page.value > 1) query.page = String(page.value)
  void router.replace({ path: '/', query })
}

function resetFilters() {
  keyword.value = ''
  activeCategory.value = ''
  sortBy.value = 'default'
  featuredOnly.value = false
  inStockOnly.value = false
  page.value = 1
}

let typing: ReturnType<typeof setTimeout> | undefined
watch(keyword, () => {
  clearTimeout(typing)
  typing = setTimeout(run, 280)
})

// The debounce outlives the page it was set on: a buyer who types and then
// clicks a product would be pulled back to 首页 by their own keystroke, because
// run() rewrites the address of wherever they ended up.
onBeforeUnmount(() => clearTimeout(typing))

/**
 * Whether the buyer narrowed the shelf themselves. The server counts only what
 * the filters leave, so an empty `total` says nothing about whether the shop has
 * goods at all — and 「店铺还没有上架商品」 aimed at someone who just mistyped a
 * search is the one empty state that is never true.
 */
const narrowed = computed(
  () => keyword.value.trim() !== '' || activeCategory.value !== '' || featuredOnly.value || inStockOnly.value,
)

watch([activeCategory, sortBy, featuredOnly, inStockOnly], () => {
  if (page.value === 1) run()
  else page.value = 1 // the page watcher runs the reload
})

watch(page, run)

function run() {
  syncUrl()
  void load()
}

onMounted(async () => {
  const query = route.query
  keyword.value = typeof query.q === 'string' ? query.q : ''
  const category = Number(query.category)
  activeCategory.value = Number.isInteger(category) && category > 0 ? category : ''
  const sort = typeof query.sort === 'string' ? query.sort : 'default'
  sortBy.value = SORTS.some((item) => item.key === sort) ? sort : 'default'
  featuredOnly.value = query.featured === '1'
  inStockOnly.value = query.in_stock === '1'
  const parsed = Number(query.page)
  page.value = Number.isInteger(parsed) && parsed > 0 ? parsed : 1

  const [categoriesResult] = await Promise.all([
    listCategories().catch(() => [] as Category[]),
    load(),
    storeStats()
      .then((result) => {
        stats.value = result.stats
      })
      .catch(() => undefined),
  ])
  categories.value = categoriesResult
})
</script>

<template>
  <div class="mx-auto w-full max-w-6xl px-4 sm:px-6">
    <p
      v-if="announcement"
      class="rise-in mt-8 flex items-start gap-3 rounded-[var(--radius-sm)] border border-[var(--accent-line)] bg-[var(--accent-soft)] px-4 py-3 text-[13px] text-[var(--text-dim)]"
      role="status"
    >
      <span aria-hidden="true" class="accent-text mt-px">📣</span>
      <span class="min-w-0 flex-1 break-words whitespace-pre-line">{{ announcement }}</span>
      <button class="hint shrink-0 transition-colors hover:text-[var(--text)]" aria-label="关闭公告" @click="hideAnnouncement">
        知道了
      </button>
    </p>

    <!-- Editorial intro: what this store ships, stated plainly. -->
    <section class="rise-in mt-12 max-w-3xl">
      <p class="eyebrow">数字商品商店</p>
      <h1 class="home-title mt-3 text-4xl font-bold sm:text-5xl">
        下单、支付、<span class="accent-text">即时到货</span>
      </h1>
      <p class="mt-4 text-[15px] leading-relaxed text-[var(--text-dim)]">
        使用 NodeLoc 账号登录即可购买。卡密类商品在付款完成的瞬间自动交付，人工交付的商品会进入商家的发货队列并同步到你的订单。
      </p>
      <dl class="mt-7 grid grid-cols-2 gap-x-8 gap-y-3 text-sm sm:grid-cols-4">
        <div>
          <dt class="text-[var(--text-quiet)]">在售商品</dt>
          <dd class="nums text-lg font-semibold">{{ stats ? stats.products : '—' }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">现货可购</dt>
          <dd class="nums text-lg font-semibold">{{ stats ? stats.stock : '—' }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">累计成交</dt>
          <dd class="nums text-lg font-semibold">{{ stats ? stats.sales : '—' }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">分类</dt>
          <dd class="nums text-lg font-semibold">{{ stats ? stats.categories : '—' }}</dd>
        </div>
      </dl>
    </section>

    <PromoStrip />

    <!-- Search + category filter -->
    <section class="catalog-toolbar mt-12 flex flex-col gap-3 lg:flex-row lg:items-center">
      <div class="relative sm:max-w-xs sm:flex-1">
        <input v-model="keyword" type="search" class="input !pl-9" placeholder="搜索商品…" aria-label="搜索商品" />
        <span class="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-[var(--text-quiet)]">⌕</span>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <button class="chip" :class="{ 'chip-active': activeCategory === '' }" @click="activeCategory = ''">
          全部
        </button>
        <button
          v-for="item in categories"
          :key="item.id"
          class="chip"
          :class="{ 'chip-active': activeCategory === item.id }"
          @click="activeCategory = item.id"
        >
          {{ item.name }}
          <span v-if="item.product_count" class="nums opacity-60">{{ item.product_count }}</span>
        </button>
      </div>
      <div class="flex flex-wrap items-center gap-3 lg:ml-auto">
        <button class="chip" :class="{ 'chip-active': featuredOnly }" @click="featuredOnly = !featuredOnly">
          店长推荐
        </button>
        <button class="chip" :class="{ 'chip-active': inStockOnly }" @click="inStockOnly = !inStockOnly">
          仅看现货
        </button>
        <div class="flex items-center gap-2">
          <label class="hint whitespace-nowrap" for="sort">排序</label>
          <select id="sort" v-model="sortBy" class="input !w-auto !py-1.5 text-[13px]">
            <option v-for="item in SORTS" :key="item.key" :value="item.key">{{ item.label }}</option>
          </select>
        </div>
      </div>
    </section>

    <p v-if="!loading && !error && products.length" class="hint mt-5 nums" role="status">
      共 <span class="text-[var(--text-dim)]">{{ total }}</span> 件商品 · 第 {{ page }} / {{ pageCount }} 页
    </p>

    <div v-if="error" class="card mt-8 text-center">
      <p class="alert alert-danger text-left" role="alert">{{ error }}</p>
      <button class="btn btn-secondary mt-5" :disabled="loading" @click="load">
        <span v-if="loading" class="spinner" />
        {{ loading ? '重新加载中…' : '重新加载商品' }}
      </button>
    </div>

    <!-- Loading -->
    <div v-else-if="loading" class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="i in 6" :key="i" class="card overflow-hidden !p-0">
        <div class="skeleton aspect-[16/9] !rounded-none" />
        <div class="space-y-3 p-5">
          <div class="skeleton h-4 w-2/3" />
          <div class="skeleton h-3 w-full" />
          <div class="skeleton h-3 w-1/2" />
        </div>
      </div>
    </div>

    <template v-else-if="products.length">
      <div class="stagger mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
        <ProductCard v-for="product in products" :key="product.id" :product="product" />
      </div>

      <nav v-if="pageCount > 1" class="mt-10 flex items-center justify-center gap-2" aria-label="分页">
        <button class="btn btn-quiet btn-sm" :disabled="page <= 1" @click="page -= 1">← 上一页</button>
        <template v-for="item in pageCount" :key="item">
          <button
            v-if="item === 1 || item === pageCount || Math.abs(item - page) <= 1"
            class="btn btn-sm"
            :class="item === page ? 'btn-primary' : 'btn-quiet'"
            @click="page = item"
          >
            {{ item }}
          </button>
          <span v-else-if="item === 2 || item === pageCount - 1" class="hint">…</span>
        </template>
        <button class="btn btn-quiet btn-sm" :disabled="page >= pageCount" @click="page += 1">下一页 →</button>
      </nav>
    </template>

    <div v-else class="card mt-8 py-20 text-center">
      <p class="text-[var(--text-quiet)]" aria-hidden="true">◍</p>
      <p class="mt-3 font-semibold">{{ narrowed ? '没有匹配的商品' : '店铺还没有上架商品' }}</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">
        {{ narrowed ? '换个关键词或分类试试' : '管理员在后台上架商品后即可在此购买' }}
      </p>
      <button v-if="narrowed" class="btn btn-secondary btn-sm mt-6" @click="resetFilters">清除筛选</button>
    </div>
  </div>
</template>
