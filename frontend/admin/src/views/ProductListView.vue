<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import { deleteProduct, listProducts, updateProduct } from '../api/products'
import { errorMessage, money, when } from '../utils/format'
import type { Product } from '../types'

const PageSize = 12

const router = useRouter()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const products = ref<Product[]>([])
const search = ref('')
const typeFilter = ref('all')
const page = ref(1)

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return products.value.filter((item) => {
    if (typeFilter.value !== 'all' && item.product_type !== typeFilter.value) return false
    if (!needle) return true
    return (
      item.name.toLowerCase().includes(needle) ||
      item.slug.toLowerCase().includes(needle) ||
      (item.category?.name || '').toLowerCase().includes(needle)
    )
  })
})

const totalPages = computed(() => Math.max(1, Math.ceil(filtered.value.length / PageSize)))
const paged = computed(() => {
  const start = (Math.min(page.value, totalPages.value) - 1) * PageSize
  return filtered.value.slice(start, start + PageSize)
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    products.value = await listProducts()
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  } finally {
    loading.value = false
  }
}

async function togglePublished(product: Product) {
  busy.value = true
  error.value = ''
  try {
    const next = await updateProduct(product.id, { ...product, is_published: !product.is_published })
    products.value = products.value.map((item) => (item.id === next.id ? next : item))
  } catch (err) {
    error.value = errorMessage(err, '更新商品状态失败')
  } finally {
    busy.value = false
  }
}

function applyFilters() {
  page.value = 1
}

async function removeProduct(product: Product) {
  if (!confirm(`删除商品「${product.name}」？其卡密会一并失效。`)) return
  busy.value = true
  error.value = ''
  try {
    await deleteProduct(product.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除商品失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="search"
          class="input w-64"
          placeholder="名称 / slug / 分类"
          @input="applyFilters"
        />
        <button
          v-for="option in [
            { key: 'all', label: '全部' },
            { key: 'card', label: '卡密' },
            { key: 'manual', label: '人工交付' },
          ]"
          :key="option.key"
          class="chip"
          :class="typeFilter === option.key ? 'chip-active' : ''"
          @click="typeFilter = option.key; applyFilters()"
        >
          {{ option.label }}
        </button>
      </div>
      <RouterLink to="/products/new" class="btn btn-primary btn-sm">+ 新建商品</RouterLink>
    </div>

    <div v-if="error" class="alert alert-danger">{{ error }}</div>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>商品</th>
            <th>分类</th>
            <th>类型</th>
            <th>价格</th>
            <th>库存</th>
            <th>状态</th>
            <th>创建时间</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="8"><div class="skeleton h-9" /></td>
          </tr>
          <tr v-else-if="!paged.length">
            <td colspan="8" class="py-12 text-center text-sm quiet">还没有商品，右上角创建一个。</td>
          </tr>
          <tr v-for="product in paged" :key="product.id">
            <td>
              <div class="flex min-w-0 items-center gap-3">
                <img
                  v-if="product.image_path"
                  :src="product.image_path"
                  :alt="product.name"
                  class="h-10 w-10 shrink-0 rounded-lg border border-[var(--stroke)] object-cover"
                />
                <div
                  v-else
                  class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[var(--accent-soft)] text-sm font-bold accent-text"
                >
                  {{ product.name.slice(0, 1) }}
                </div>
                <div class="min-w-0">
                  <p class="truncate text-sm font-medium">{{ product.name }}</p>
                  <p class="quiet truncate text-xs mono">{{ product.slug }}</p>
                </div>
              </div>
            </td>
            <td class="text-sm muted">{{ product.category?.name || '—' }}</td>
            <td>
              <span class="badge" :class="product.product_type === 'card' ? 'badge-info' : 'badge-neutral'">
                {{ product.product_type === 'card' ? '卡密' : '人工交付' }}
              </span>
            </td>
            <td class="nums text-sm">{{ money(product.price) }}</td>
            <td class="nums text-sm">
              <span v-if="product.product_type === 'manual'" class="quiet">—</span>
              <span v-else :class="product.stock_count > 0 ? '' : 'text-[var(--danger)]'">{{ product.stock_count }}</span>
            </td>
            <td>
              <span class="badge" :class="product.is_published ? 'badge-success' : 'badge-neutral'">
                {{ product.is_published ? '已上架' : '已下架' }}
              </span>
            </td>
            <td class="whitespace-nowrap text-sm quiet">{{ when(product.created_at) }}</td>
            <td class="whitespace-nowrap text-right">
              <RouterLink v-if="product.product_type === 'card'" :to="`/cards/${product.id}`" class="btn btn-ghost btn-sm">
                卡密
              </RouterLink>
              <button class="btn btn-ghost btn-sm" :disabled="busy" @click="togglePublished(product)">
                {{ product.is_published ? '下架' : '上架' }}
              </button>
              <RouterLink :to="`/products/${product.id}/edit`" class="btn btn-ghost btn-sm">编辑</RouterLink>
              <button
                class="btn btn-ghost btn-sm text-[var(--danger)]"
                :disabled="busy"
                @click="removeProduct(product)"
              >
                删除
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <p class="quiet text-xs mono">{{ filtered.length }} 个商品</p>
      <div class="flex items-center gap-2">
        <button class="btn btn-secondary btn-sm" :disabled="page <= 1" @click="page--">上一页</button>
        <span class="text-sm muted mono">{{ page }} / {{ totalPages }}</span>
        <button class="btn btn-secondary btn-sm" :disabled="page >= totalPages" @click="page++">下一页</button>
      </div>
    </div>
  </section>
</template>
