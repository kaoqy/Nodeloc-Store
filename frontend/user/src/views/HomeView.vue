<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import ProductCard from '../components/ProductCard.vue'
import { listCategories, listProducts } from '../api/products'
import { errorMessage } from '../api/client'
import type { Category, Product } from '../types'

const products = ref<Product[]>([])
const categories = ref<Category[]>([])
const keyword = ref('')
const activeCategory = ref('')
const sortBy = ref('default')
const loading = ref(true)
const error = ref('')

const SORTS: { key: string; label: string }[] = [
  { key: 'default', label: '默认排序' },
  { key: 'price-asc', label: '价格从低到高' },
  { key: 'price-desc', label: '价格从高到低' },
  { key: 'newest', label: '最新上架' },
]

const visible = computed(() => {
  const needle = keyword.value.trim().toLowerCase()
  const list = products.value.filter((product) => {
    if (activeCategory.value && product.category?.slug !== activeCategory.value) return false
    if (!needle) return true
    return [product.name, product.summary, product.description]
      .filter(Boolean)
      .some((field) => String(field).toLowerCase().includes(needle))
  })
  if (sortBy.value === 'price-asc' || sortBy.value === 'price-desc') {
    const sign = sortBy.value === 'price-asc' ? 1 : -1
    return [...list].sort((a, b) => (a.price - b.price) * sign)
  }
  if (sortBy.value === 'newest') {
    return [...list].sort((a, b) => String(b.created_at || '').localeCompare(String(a.created_at || '')))
  }
  return list
})

const purchasable = computed(() => products.value.filter((item) => item.stock_count > 0).length)

const categoryName = computed(
  () => categories.value.find((item) => item.slug === activeCategory.value)?.name || activeCategory.value,
)

function resetFilters() {
  keyword.value = ''
  activeCategory.value = ''
  sortBy.value = 'default'
}

onMounted(async () => {
  try {
    const [items, groups] = await Promise.all([listProducts(), listCategories()])
    products.value = items
    categories.value = groups
  } catch (e) {
    error.value = errorMessage(e, '商品加载失败，请稍后重试')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mx-auto w-full max-w-6xl px-4 sm:px-6">
    <!-- Editorial intro: what this store ships, stated plainly. -->
    <section class="rise-in mt-12 max-w-3xl">
      <p class="eyebrow">Digital goods store</p>
      <h1 class="mt-3 text-4xl font-bold sm:text-5xl">
        下单、支付、<span class="accent-text">即时到货</span>
      </h1>
      <p class="mt-4 text-[15px] leading-relaxed text-[var(--text-dim)]">
        使用 NodeLoc 账号登录即可购买。卡密类商品在付款完成的瞬间自动交付，人工交付的商品会进入商家的发货队列并同步到你的订单。
      </p>
      <dl class="mt-7 grid grid-cols-2 gap-x-8 gap-y-3 text-sm sm:max-w-md">
        <div>
          <dt class="text-[var(--text-quiet)]">在售商品</dt>
          <dd class="nums text-lg font-semibold">{{ loading ? '—' : products.length }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">现货可购</dt>
          <dd class="nums text-lg font-semibold">{{ loading ? '—' : purchasable }}</dd>
        </div>
      </dl>
    </section>

    <!-- Search + category filter -->
    <section class="mt-12 flex flex-col gap-3 lg:flex-row lg:items-center">
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
          :class="{ 'chip-active': activeCategory === item.slug }"
          @click="activeCategory = item.slug"
        >
          {{ item.name }}
        </button>
      </div>
      <div class="flex items-center gap-2 lg:ml-auto">
        <label class="hint whitespace-nowrap" for="sort">排序</label>
        <select id="sort" v-model="sortBy" class="input !w-auto !py-1.5 text-[13px]">
          <option v-for="item in SORTS" :key="item.key" :value="item.key">{{ item.label }}</option>
        </select>
      </div>
    </section>

    <p v-if="!loading && !error && products.length" class="hint mt-5 nums" role="status">
      找到 <span class="text-[var(--text-dim)]">{{ visible.length }}</span> 件商品
      <template v-if="activeCategory"> · {{ categoryName }}</template>
    </p>

    <p v-if="error" class="alert alert-danger mt-8" role="alert">{{ error }}</p>

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

    <div v-else-if="visible.length" class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      <ProductCard v-for="product in visible" :key="product.id" :product="product" />
    </div>

    <div v-else class="card mt-8 py-20 text-center">
      <p class="text-[var(--text-quiet)]" aria-hidden="true">◍</p>
      <p class="mt-3 font-semibold">{{ products.length ? '没有匹配的商品' : '店铺还没有上架商品' }}</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">
        {{ products.length ? '换个关键词或分类试试' : '管理员在后台上架商品后即可在此购买' }}
      </p>
      <button v-if="products.length" class="btn btn-secondary btn-sm mt-6" @click="resetFilters">清除筛选</button>
    </div>
  </div>
</template>
