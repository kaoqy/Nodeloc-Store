<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import SearchBar from '../components/SearchBar.vue'
import { deleteProduct, listProducts } from '../api/products'
import type { Product } from '../types'

const products = ref<Product[]>([])
const search = ref('')
const page = ref(1)
const pageSize = 10
const loading = ref(false)

const filtered = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword) return products.value
  return products.value.filter(product =>
    product.name.toLowerCase().includes(keyword) ||
    product.description?.toLowerCase().includes(keyword),
  )
})
const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / pageSize)))
const visibleProducts = computed(() => filtered.value.slice((page.value - 1) * pageSize, page.value * pageSize))

async function load() {
  loading.value = true
  try {
    products.value = (await listProducts({ page: 1, per_page: 100 })).data
  } finally {
    loading.value = false
  }
}

async function remove(product: Product) {
  if (!window.confirm(`确定删除商品“${product.name}”吗？`)) return
  await deleteProduct(product.id)
  await load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-6">
    <div class="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
      <div>
        <h2 class="text-2xl font-bold">商品管理</h2>
        <p class="mt-1 text-sm text-slate-400">管理数字商品、库存卡密和销售状态</p>
      </div>
      <RouterLink to="/products/new" class="btn-primary text-center">新增商品</RouterLink>
    </div>

    <div class="panel">
      <SearchBar v-model="search" class="mb-5 max-w-md" placeholder="搜索商品名称或描述" @update:model-value="page = 1" />
      <div v-if="loading" class="py-16 text-center text-slate-500">商品加载中…</div>
      <div v-else-if="!visibleProducts.length" class="py-16 text-center text-slate-500">暂无匹配商品</div>
      <div v-else class="overflow-x-auto">
        <table class="w-full min-w-[760px] text-left text-sm">
          <thead class="text-slate-500">
            <tr><th class="pb-3">商品</th><th>价格</th><th>库存</th><th>状态</th><th>分类</th><th class="text-right">操作</th></tr>
          </thead>
          <tbody class="divide-y divide-white/5">
            <tr v-for="product in visibleProducts" :key="product.id" class="hover:bg-white/[0.03]">
              <td class="py-4"><p class="font-medium">{{ product.name }}</p><p class="mt-1 max-w-xs truncate text-xs text-slate-500">{{ product.description || '暂无描述' }}</p></td>
              <td>¥{{ Number(product.price).toFixed(2) }}</td>
              <td>{{ product.stock ?? 0 }}</td>
              <td><span class="rounded-full bg-indigo-500/15 px-2.5 py-1 text-xs text-indigo-300">{{ product.status || '正常' }}</span></td>
              <td>{{ product.category?.name || '-' }}</td>
              <td><div class="flex justify-end gap-2"><RouterLink :to="`/products/${product.id}/cards`" class="btn-muted">卡密</RouterLink><RouterLink :to="`/products/${product.id}/edit`" class="btn-muted">编辑</RouterLink><button class="rounded-xl border border-rose-500/20 bg-rose-500/10 px-3 py-2 text-rose-300 hover:bg-rose-500/20" @click="remove(product)">删除</button></div></td>
            </tr>
          </tbody>
        </table>
      </div>
      <div class="mt-5 flex items-center justify-between border-t border-white/5 pt-4 text-sm text-slate-400">
        <span>共 {{ filtered.length }} 件商品</span>
        <div class="flex items-center gap-2"><button class="btn-muted" :disabled="page <= 1" @click="page--">上一页</button><span>{{ page }} / {{ pages }}</span><button class="btn-muted" :disabled="page >= pages" @click="page++">下一页</button></div>
      </div>
    </div>
  </section>
</template>
