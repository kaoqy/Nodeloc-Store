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
  <div class="home-page">
    <div class="mx-auto w-full max-w-6xl px-4 sm:px-6">
      <p v-if="announcement" class="announcement-bar rise-in" role="status">
        <span class="accent-text mt-px shrink-0" aria-hidden="true">公告</span>
        <span class="min-w-0 flex-1 break-words whitespace-pre-line">{{ announcement }}</span>
        <button class="hint shrink-0 transition-colors hover:text-[var(--text)]" aria-label="关闭公告" @click="hideAnnouncement">
          知道了
        </button>
      </p>
    </div>

    <section class="home-hero">
      <div class="home-hero-inner">
        <div class="rise-in max-w-2xl">
          <p class="eyebrow">数字商品商店</p>
          <h1 class="home-title mt-2 text-3xl font-bold sm:text-4xl">
            选好商品，<span class="accent-text">支付后自动交付</span>
          </h1>
          <p class="mt-3 text-sm leading-relaxed text-[var(--text-dim)] sm:text-[15px]">
            使用 NodeLoc 账号登录即可购买。卡密在付款确认后自动发放，人工交付商品会进入发货队列，进度同步到订单详情。
          </p>
          <div class="hero-actions">
            <a href="#catalog" class="btn btn-primary">浏览商品</a>
          </div>
          <ul class="hero-points">
            <li>NodeLoc 授权登录</li>
            <li>支付结果服务端核实</li>
            <li>订单与交付记录可查</li>
          </ul>
        </div>

        <dl class="home-metrics rise-in">
          <div class="home-metric">
            <dt>在售商品</dt>
            <dd>{{ stats ? stats.products : '—' }}</dd>
          </div>
          <div class="home-metric">
            <dt>现货可购</dt>
            <dd>{{ stats ? stats.stock : '—' }}</dd>
          </div>
          <div class="home-metric">
            <dt>累计成交</dt>
            <dd>{{ stats ? stats.sales : '—' }}</dd>
          </div>
          <div class="home-metric">
            <dt>商品分类</dt>
            <dd>{{ stats ? stats.categories : '—' }}</dd>
          </div>
        </dl>
      </div>
    </section>

    <div class="mx-auto w-full max-w-6xl px-4 sm:px-6">
      <PromoStrip />

      <section id="catalog" class="catalog-layout">
        <aside class="catalog-aside card">
          <div class="catalog-aside-head">
            <div>
              <p class="eyebrow">筛选与分类</p>
              <h2 class="mt-1.5 text-lg font-bold">按分类浏览</h2>
            </div>
            <button v-if="narrowed || sortBy !== 'default'" class="hint" type="button" @click="resetFilters">重置</button>
          </div>
          <div class="catalog-categories">
            <button class="catalog-category" :class="{ 'catalog-category-active': activeCategory === '' }" @click="activeCategory = ''">
              <span>全部商品</span>
              <span class="nums">{{ stats ? stats.products : '—' }}</span>
            </button>
            <button
              v-for="item in categories"
              :key="item.id"
              class="catalog-category"
              :class="{ 'catalog-category-active': activeCategory === item.id }"
              @click="activeCategory = item.id"
            >
              <span class="truncate">{{ item.name }}</span>
              <span class="nums">{{ item.product_count ?? 0 }}</span>
            </button>
          </div>
          <div class="catalog-quick-grid">
            <button class="catalog-quick" :class="{ 'catalog-quick-active': featuredOnly }" @click="featuredOnly = !featuredOnly">
              店长推荐
            </button>
            <button class="catalog-quick" :class="{ 'catalog-quick-active': inStockOnly }" @click="inStockOnly = !inStockOnly">
              仅看现货
            </button>
          </div>
        </aside>

        <div class="min-w-0">
          <div class="catalog-topbar">
            <div class="relative min-w-0 flex-1">
              <input v-model="keyword" type="search" class="input !pl-9" placeholder="搜索商品…" aria-label="搜索商品" />
              <span class="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-[var(--text-quiet)]">⌕</span>
            </div>
            <div class="flex items-center gap-2">
              <label class="hint hidden whitespace-nowrap sm:inline" for="sort">排序</label>
              <select id="sort" v-model="sortBy" class="input !w-auto !py-2 text-[13px]" aria-label="排序">
                <option v-for="item in SORTS" :key="item.key" :value="item.key">{{ item.label }}</option>
              </select>
            </div>
            <button v-if="narrowed || sortBy !== 'default'" class="btn btn-quiet btn-sm" type="button" @click="resetFilters">清除筛选</button>
          </div>

          <div class="catalog-heading">
            <div>
              <p class="eyebrow">{{ narrowed ? '筛选结果' : '全部商品' }}</p>
              <h2 class="mt-1 text-xl font-bold">
                {{ activeCategory === '' ? '在售商品' : categories.find((item) => item.id === activeCategory)?.name || '在售商品' }}
              </h2>
            </div>
            <p v-if="!loading && !error" class="hint nums" role="status">
              共 <span class="text-[var(--text-dim)]">{{ total }}</span> 件 · 第 {{ page }} / {{ pageCount }} 页
            </p>
          </div>

          <div v-if="error" class="card text-center">
            <p class="alert alert-danger text-left" role="alert">{{ error }}</p>
            <button class="btn btn-secondary mt-5" :disabled="loading" @click="load">
              <span v-if="loading" class="spinner" />
              {{ loading ? '重新加载中…' : '重新加载商品' }}
            </button>
          </div>

          <div v-else-if="loading" class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
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
            <div class="stagger grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
              <ProductCard v-for="product in products" :key="product.id" :product="product" />
            </div>

            <nav v-if="pageCount > 1" class="mt-10 flex flex-wrap items-center justify-center gap-2" aria-label="分页">
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

          <div v-else class="card py-20 text-center">
            <p class="text-[var(--text-quiet)]" aria-hidden="true">◍</p>
            <p class="mt-3 font-semibold">{{ narrowed ? '没有匹配的商品' : '店铺还没有上架商品' }}</p>
            <p class="mt-1.5 text-sm text-[var(--text-quiet)]">
              {{ narrowed ? '换个关键词或分类试试' : '管理员在后台上架商品后即可在此购买' }}
            </p>
            <button v-if="narrowed" class="btn btn-secondary btn-sm mt-6" @click="resetFilters">清除筛选</button>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.home-hero {
  margin-top: 18px;
  border-block: 1px solid var(--stroke);
  background:
    linear-gradient(135deg, color-mix(in srgb, var(--accent-soft) 72%, transparent), transparent 42%),
    linear-gradient(180deg, var(--surface-hi), var(--surface-sunken));
}

.home-hero-inner {
  display: grid;
  width: 100%;
  max-width: 72rem;
  margin-inline: auto;
  gap: 28px;
  padding: 36px 16px;
}

.announcement-bar {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  margin-top: 18px;
  border: 1px solid var(--accent-line);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--accent-soft) 72%, var(--glass));
  padding: 12px 14px;
  color: var(--text-dim);
  font-size: 13px;
  box-shadow: var(--shadow-xs);
  backdrop-filter: blur(8px);
}

.hero-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 22px;
}

.hero-points {
  display: flex;
  flex-wrap: wrap;
  gap: 8px 18px;
  margin-top: 18px;
  color: var(--text-quiet);
  font-size: 12px;
}

.hero-points li {
  display: flex;
  align-items: center;
  gap: 7px;
}

.hero-points li::before {
  content: '';
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  border-radius: 50%;
  background: var(--accent);
  box-shadow: 0 0 0 3px var(--accent-soft);
}

.home-metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  overflow: hidden;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--surface) 76%, transparent);
  box-shadow: var(--shadow-sm);
  backdrop-filter: blur(10px);
}

.home-metrics .home-metric {
  min-width: 0;
  padding: 13px 16px;
  border-right: 1px solid var(--stroke-quiet);
  border-bottom: 1px solid var(--stroke-quiet);
}

.home-metrics dt {
  color: var(--text-quiet);
  font-size: 11.5px;
}

.home-metrics dd {
  margin-top: 2px;
  color: var(--text);
  font-family: var(--font-mono);
  font-size: 20px;
  font-weight: 700;
  line-height: 1.25;
}

.catalog-layout {
  display: grid;
  gap: 30px;
  margin-top: 42px;
}

.catalog-aside {
  min-width: 0;
  padding: 16px;
}

.catalog-aside-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.catalog-categories {
  display: flex;
  gap: 6px;
  margin-top: 12px;
  overflow-x: auto;
  padding-bottom: 2px;
  scrollbar-width: none;
}

.catalog-categories::-webkit-scrollbar {
  display: none;
}

.catalog-category,
.catalog-quick {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 38px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-sm);
  background: var(--surface);
  color: var(--text-dim);
  font-size: 13px;
  text-align: left;
  transition: border-color var(--fast), background var(--fast), color var(--fast);
}

.catalog-category {
  flex: 0 0 auto;
  padding: 8px 12px;
  transition: border-color var(--fast), background var(--fast), color var(--fast), transform 180ms var(--spring);
}

.catalog-quick {
  justify-content: center;
  padding: 8px 12px;
}

.catalog-quick-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  margin-top: 12px;
}

@media (hover: hover) {
  .catalog-category:hover,
  .catalog-quick:hover {
    transform: translateY(-1px);
  }
}

.catalog-category:hover,
.catalog-quick:hover {
  border-color: var(--stroke-hi);
  color: var(--text);
}

.catalog-category-active,
.catalog-quick-active {
  border-color: var(--accent-line);
  background: var(--accent-soft);
  color: var(--accent);
}

.catalog-topbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-md);
  background: color-mix(in srgb, var(--surface) 84%, transparent);
  box-shadow: var(--shadow-xs);
  padding: 12px;
  backdrop-filter: blur(8px);
}

.catalog-heading {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: 10px;
  margin: 22px 0 16px;
}

@media (min-width: 640px) {
  .home-metrics {
    grid-template-columns: repeat(4, minmax(0, 1fr));
  }
}

@media (min-width: 1024px) {
  .home-hero-inner {
    grid-template-columns: minmax(0, 1.2fr) minmax(320px, 0.8fr);
    align-items: end;
    gap: 42px;
    padding-block: 48px;
  }

  .catalog-layout {
    grid-template-columns: 214px minmax(0, 1fr);
    gap: 34px;
  }

  .catalog-aside {
    position: sticky;
    top: 82px;
    align-self: start;
  }

  .catalog-categories {
    display: grid;
    overflow: visible;
    margin-top: 12px;
  }

  .catalog-category {
    width: 100%;
    padding-inline: 11px;
  }

  .catalog-quick-grid {
    grid-template-columns: 1fr;
  }
}

@media (min-width: 640px) {
  .home-hero-inner {
    padding-inline: 24px;
  }
}

@media (max-width: 639px) {
  .home-title {
    font-size: clamp(2rem, 9.5vw, 2.55rem);
  }

  .hero-actions .btn {
    flex: 1;
    min-width: 9rem;
  }

  .catalog-topbar select {
    max-width: 100%;
  }
}
</style>
