<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listProducts } from '../api/products'
import { useRouter } from 'vue-router'
import type { Product } from '../types'

const router = useRouter()
const loading = ref(true)
const products = ref<Product[]>([])
const search = ref('')
const page = ref(1)
const perPage = ref(10)

async function load() {
  loading.value = true
  try {
    const result = await listProducts({ page: page.value, per_page: perPage.value, q: search.value || undefined })
    products.value = result.data
  } finally {
    loading.value = false
  }
}

function editProduct(id: number) {
  router.push(`/products/${id}`)
}

function deleteProduct(id: number) {
  if (confirm('确定要删除此商品吗？')) {
    // TODO: implement delete
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <!-- Actions bar -->
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <input v-model="search" class="input w-64" placeholder="搜索商品名称..." @keyup.enter="load" />
        <button class="btn-secondary" @click="load">筛选</button>
      </div>
      <button class="btn-primary" @click="router.push('/products/new')">+ 新建商品</button>
    </div>

    <!-- Table -->
    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>商品名称</th>
            <th>分类</th>
            <th>价格</th>
            <th>库存</th>
            <th>状态</th>
            <th>创建时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="8">
              <div class="space-y-2">
                <div v-for="i in 5" :key="i" class="skeleton h-8" />
              </div>
            </td>
          </tr>
          <tr v-else-if="!products.length">
            <td colspan="8" class="py-12 text-center text-[#6b6b80]">暂无商品</td>
          </tr>
          <tr v-for="product in products" :key="product.id">
            <td>{{ product.id }}</td>
            <td>
              <div class="flex items-center gap-3">
                <div v-if="product.image_path" class="h-10 w-10 rounded-lg bg-cover bg-center" :style="{ backgroundImage: `url(${product.image_path})` }" />
                <div v-else class="flex h-10 w-10 items-center justify-center rounded-lg bg-[#1a1a25] text-sm text-[#6b6b80]">无图</div>
                <span class="font-medium">{{ product.name }}</span>
              </div>
            </td>
            <td>{{ product.category?.name || '-' }}</td>
            <td>¥{{ Number(product.price).toFixed(2) }}</td>
            <td>{{ product.stock_count }}</td>
            <td>
              <span
                :class="[
                  'badge',
                  product.is_published ? 'badge-success' : 'badge-neutral'
                ]"
              >
                {{ product.is_published ? '上架' : '下架' }}
              </span>
            </td>
            <td class="text-[#a1a1b5]">{{ product.created_at }}</td>
            <td>
              <div class="flex items-center gap-2">
                <button class="btn-ghost text-xs" @click="editProduct(product.id)">编辑</button>
                <button class="btn-ghost text-xs text-[#ef4444]" @click="deleteProduct(product.id)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Pagination -->
    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">显示 {{ products.length }} 条</p>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-sm text-[#a1a1b5]">第 {{ page }} 页</span>
        <button class="btn-secondary" @click="page++; load()">下一页</button>
      </div>
    </div>
  </section>
</template>
