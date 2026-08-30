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
    <section class="mb-10 rounded-3xl border border-white/[0.08] bg-gradient-to-br from-purple-900/20 via-[#09090b] to-indigo-900/20 p-8 sm:p-12">
      <p class="mb-3 text-xs font-semibold uppercase tracking-[0.3em] text-purple-300">Digital Marketplace</p>
      <h1 class="max-w-3xl text-4xl font-black tracking-tight sm:text-5xl">
        发现优质<span class="gradient-text">数字商品</span>
      </h1>
      <p class="mt-4 max-w-2xl text-[#a1a1aa] text-base leading-relaxed">安全支付，即时交付。精选数字资源，让创意与效率触手可及。</p>
    </section>

    <!-- Search & Filter -->
    <form
      class="mb-8 grid gap-3 rounded-2xl border border-white/[0.08] bg-[#111114] p-4 sm:grid-cols-[1fr_220px_auto]"
      @submit.prevent="load(true)"
    >
      <input v-model="q" class="input" placeholder="搜索商品…" />
      <select v-model="category" class="input" @change="load(true)">
        <option value="">全部分类</option>
        <option v-for="item in categories" :key="item.id" :value="item.slug">{{ item.name }}</option>
      </select>
      <button class="btn-primary" :disabled="loading">搜索</button>
    </form>

    <!-- Error -->
    <p v-if="error" class="rounded-2xl border border-red-500/20 bg-red-500/10 p-4 text-red-300">{{ error }}</p>

    <!-- Loading -->
    <div v-else-if="loading" class="grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="i in 6" :key="i" class="overflow-hidden rounded-2xl border border-white/[0.08] bg-[#111114]">
        <div class="skeleton aspect-[16/10]" />
        <div class="p-5 space-y-3">
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
    <div v-else class="rounded-3xl border border-white/[0.08] bg-[#111114] py-24 text-center">
      <p class="text-xl font-semibold">没有找到商品</p>
      <p class="mt-2 text-[#71717a]">换个关键词或分类试试吧</p>
    </div>

    <!-- Pagination -->
    <div v-if="lastPage > 1" class="mt-10 flex items-center justify-center gap-3">
      <button
        class="btn-secondary"
        :disabled="page <= 1"
        @click="changePage(page - 1)"
      >上一页</button>
      <span class="text-sm text-[#71717a]">第 {{ page }} / {{ lastPage }} 页</span>
      <button
        class="btn-secondary"
        :disabled="page >= lastPage"
        @click="changePage(page + 1)"
      >下一页</button>
    </div>
  </div>
</template>
