<script setup lang="ts">
import { onMounted, ref } from 'vue'
import ProductCard from '../components/ProductCard.vue'
import { listCategories, listProducts } from '../api/products'
import type { Category, Product } from '../types'

const products = ref<Product[]>([])
const categories = ref<Category[]>([])
const q = ref('')
const category = ref('')
const page = ref(1)
const lastPage = ref(1)
const loading = ref(false)
const error = ref('')

async function load(reset = false) {
  if (reset) page.value = 1
  loading.value = true
  error.value = ''
  try {
    const result = await listProducts({
      q: q.value || undefined,
      category: category.value || undefined,
      page: page.value,
    })
    products.value = result.data
    lastPage.value = result.last_page || 1
  } catch {
    error.value = '商品加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

async function changePage(next: number) {
  page.value = next
  await load()
  window.scrollTo({ top: 0, behavior: 'smooth' })
}

onMounted(async () => {
  await Promise.all([
    load(),
    listCategories().then(r => categories.value = r.data).catch(() => undefined),
  ])
})
</script>

<template>
  <div class="mx-auto max-w-7xl px-4 py-10 sm:px-6">
    <!-- Hero -->
    <section class="rise-in relative mb-10 overflow-hidden rounded-[30px] border border-white/[0.14] bg-gradient-to-br from-purple-500/12 via-white/[0.03] to-indigo-500/12 p-8 shadow-[inset_0_1px_0_rgba(255,255,255,0.16),0_24px_70px_-14px_rgba(0,0,0,0.6)] backdrop-blur-2xl sm:p-14">
      <div class="pointer-events-none absolute -top-24 -left-24 size-72 rounded-full bg-purple-500/25 blur-3xl" />
      <div class="pointer-events-none absolute -bottom-28 -right-20 size-80 rounded-full bg-indigo-500/20 blur-3xl" />
      <div class="relative">
        <span class="badge badge-accent mb-5 tracking-[0.28em] uppercase">Digital Marketplace</span>
        <h1 class="max-w-3xl text-4xl font-black tracking-tight sm:text-6xl sm:leading-[1.08]">
          发现优质<span class="gradient-text">数字商品</span>
        </h1>
        <p class="mt-5 max-w-2xl text-base leading-relaxed text-[#b4b2c3] sm:text-lg">
          安全支付，即时交付。精选数字资源，让创意与效率触手可及。
        </p>
        <div class="mt-8 flex flex-wrap items-center gap-6 text-sm text-[#7b7990]">
          <span class="flex items-center gap-2"><span class="size-1.5 rounded-full bg-emerald-400 shadow-[0_0_12px_rgba(52,211,153,0.9)]" />Nodeloc Payments 担保支付</span>
          <span class="flex items-center gap-2"><span class="size-1.5 rounded-full bg-purple-400 shadow-[0_0_12px_rgba(168,85,247,0.9)]" />卡密自动交付</span>
          <span class="flex items-center gap-2"><span class="size-1.5 rounded-full bg-sky-400 shadow-[0_0_12px_rgba(56,189,248,0.9)]" />NodeLoc 一键登录</span>
        </div>
      </div>
    </section>

    <!-- Search & Filter -->
    <form
      class="glass mb-10 grid gap-3 rounded-full p-3 sm:grid-cols-[1fr_220px_auto]"
      @submit.prevent="load(true)"
    >
      <input v-model="q" class="input !border-transparent !bg-white/[0.05] !shadow-none" placeholder="搜索商品…" />
      <select v-model="category" class="input !border-transparent !bg-white/[0.05] !shadow-none" @change="load(true)">
        <option value="">全部分类</option>
        <option v-for="item in categories" :key="item.id" :value="item.slug">{{ item.name }}</option>
      </select>
      <button class="btn btn-primary !px-8" :disabled="loading">搜索</button>
    </form>

    <!-- Error -->
    <p v-if="error" class="glass fade-in rounded-2xl border-red-500/25 !bg-rose-500/[0.07] p-5 text-rose-300">{{ error }}</p>

    <!-- Loading -->
    <div v-else-if="loading" class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="i in 6" :key="i" class="overflow-hidden rounded-[22px] border border-white/[0.1] bg-white/[0.03] backdrop-blur-xl">
        <div class="skeleton aspect-[16/10] !rounded-none !border-0" />
        <div class="space-y-3 p-5">
          <div class="skeleton h-4 w-3/4" />
          <div class="skeleton h-3 w-full" />
          <div class="skeleton h-3 w-1/2" />
        </div>
      </div>
    </div>

    <!-- Products Grid -->
    <div v-else-if="products.length" class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3 fade-in">
      <ProductCard v-for="product in products" :key="product.id" :product="product" />
    </div>

    <!-- Empty -->
    <div v-else class="glass rise-in py-24 text-center">
      <div class="mx-auto mb-6 grid size-16 place-items-center rounded-full border border-white/15 bg-white/[0.06] text-2xl backdrop-blur-md">🔮</div>
      <p class="text-xl font-semibold">没有找到商品</p>
      <p class="mt-2 text-[#7b7990]">换个关键词或分类试试吧</p>
    </div>

    <!-- Pagination -->
    <div v-if="lastPage > 1" class="mt-12 flex items-center justify-center gap-4">
      <button class="btn btn-secondary" :disabled="page <= 1" @click="changePage(page - 1)">← 上一页</button>
      <span class="glass rounded-full px-5 py-2 text-sm text-[#b4b2c3]">第 {{ page }} / {{ lastPage }} 页</span>
      <button class="btn btn-secondary" :disabled="page >= lastPage" @click="changePage(page + 1)">下一页 →</button>
    </div>
  </div>
</template>
