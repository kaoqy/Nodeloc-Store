<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import PaginationFooter from '../components/PaginationFooter.vue'
import { deleteProduct, listAdminProducts, updateProduct } from '../api/products'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Product } from '../types'

// 商品列表走服务端分页与筛选：店铺商品上千件时，把全部记录拉到浏览器里
// 再本地过滤既慢又会在 100 条上限处静默漏数据。
const PageSize = 20

const router = useRouter()
const auth = useAuthStore()
const canManage = computed(() => auth.allows('products', 'manage'))
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const products = ref<Product[]>([])
const total = ref(0)
const page = ref(1)
const search = ref('')
const typeFilter = ref('all')
const statusFilter = ref('all')
const sort = ref('newest')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))

const statusOptions = [
  { key: 'all', label: '全部状态' },
  { key: 'published', label: '已上架' },
  { key: 'hidden', label: '已下架' },
  { key: 'low_stock', label: '库存告急' },
  { key: 'archived', label: '已归档' },
]

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAdminProducts({
      q: search.value,
      type: typeFilter.value === 'all' ? undefined : typeFilter.value,
      status: statusFilter.value === 'all' ? undefined : statusFilter.value,
      sort: sort.value,
      limit: PageSize,
      offset: (page.value - 1) * PageSize,
    })
    products.value = result.data
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

// 上下架与推荐都用同一个更新接口：后端会忽略未改动的字段，
// 这里只发送需要翻转的那一个布尔值，避免把整个商品对象回写回去。
async function togglePublished(product: Product) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const next = await updateProduct(product.id, { is_published: !product.is_published })
    products.value = products.value.map((item) => (item.id === next.id ? next : item))
  } catch (err) {
    error.value = errorMessage(err, '更新商品状态失败')
  } finally {
    busy.value = false
  }
}

async function toggleFeatured(product: Product) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const next = await updateProduct(product.id, { is_featured: !product.is_featured })
    products.value = products.value.map((item) => (item.id === next.id ? next : item))
  } catch (err) {
    error.value = errorMessage(err, '更新推荐状态失败')
  } finally {
    busy.value = false
  }
}

async function removeProduct(product: Product) {
  if (!window.confirm('删除商品「' + product.name + '」？该商品下的卡密会一并失效。')) return
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
    <div class="flex flex-wrap items-center gap-2">
      <input
        v-model="search"
        class="input w-56"
        type="search"
        placeholder="名称 / slug / 简介"
        aria-label="搜索商品"
        @keyup.enter="applyFilters"
      />
      <select v-model="typeFilter" class="input !w-auto" aria-label="商品类型" @change="applyFilters">
        <option value="all">全部类型</option>
        <option value="card">卡密自动发货</option>
        <option value="manual">人工交付</option>
      </select>
      <select v-model="statusFilter" class="input !w-auto" aria-label="商品状态" @change="applyFilters">
        <option v-for="option in statusOptions" :key="option.key" :value="option.key">{{ option.label }}</option>
      </select>
      <select v-model="sort" class="input !w-auto" aria-label="排序" @change="applyFilters">
        <option value="newest">最新创建</option>
        <option value="sales">销量优先</option>
        <option value="price_desc">价格从高到低</option>
        <option value="price_asc">价格从低到高</option>
        <option value="default">按排序值</option>
      </select>
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="applyFilters">查询</button>
      <RouterLink v-if="canManage" to="/products/new" class="btn btn-primary btn-sm ml-auto">+ 新建商品</RouterLink>
      <p v-else class="quiet ml-auto text-xs">当前角色只能查看商品，编辑需要「商品管理」权限。</p>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div v-if="loading" class="table-container">
      <div class="card space-y-3">
        <div v-for="i in 6" :key="i" class="skeleton h-10 w-full" />
      </div>
    </div>

    <div v-else-if="!products.length" class="card py-16 text-center">
      <p class="font-semibold">没有符合条件商品</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">换个关键词、类型或状态再查一次。</p>
    </div>

    <div v-else class="table-container">
      <table class="table">
        <thead>
          <tr>
            <th>商品</th>
            <th>类型</th>
            <th class="nums">价格</th>
            <th class="nums">库存</th>
            <th class="nums">已售</th>
            <th>状态</th>
            <th>更新</th>
            <th class="text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="product in products" :key="product.id">
            <td>
              <div class="flex items-center gap-3">
                <img
                  v-if="product.image_path"
                  :src="product.image_path"
                  :alt="product.name"
                  class="size-9 rounded-md border border-[var(--stroke)] object-cover"
                />
                <div class="min-w-0">
                  <p class="truncate font-semibold">{{ product.name }}</p>
                  <p class="quiet mono text-[11px]">{{ product.slug }}</p>
                </div>
              </div>
            </td>
            <td class="text-xs">{{ product.product_type === 'card' ? '卡密' : '人工交付' }}</td>
            <td class="nums">{{ money(product.price) }}</td>
            <td class="nums">
              {{ product.product_type === 'card' ? product.stock_count : '—' }}
              <span v-if="product.product_type === 'card' && product.stock_count <= 3" class="badge-warning ml-1">紧张</span>
            </td>
            <td class="nums">{{ product.sold_count ?? 0 }}</td>
            <td>
              <span :class="product.is_published ? 'badge-success' : 'badge'">
                {{ product.is_published ? '已上架' : '已下架' }}
              </span>
              <span v-if="product.is_featured" class="badge-accent ml-1">推荐</span>
            </td>
            <td class="quiet text-xs">{{ when(product.updated_at) }}</td>
            <td class="text-right">
              <div class="flex flex-wrap justify-end gap-1.5">
                <RouterLink v-if="canManage" :to="'/products/' + product.id + '/edit'" class="btn btn-quiet btn-sm">编辑</RouterLink>
                <RouterLink :to="'/cards/' + product.id" class="btn btn-quiet btn-sm">卡密</RouterLink>
                <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="togglePublished(product)">
                  {{ product.is_published ? '下架' : '上架' }}
                </button>
                <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggleFeatured(product)">
                  {{ product.is_featured ? '取消推荐' : '设为推荐' }}
                </button>
                <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="removeProduct(product)">删除</button>
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
      :summary="'共 ' + total + ' 个商品'"
      @change="goPage"
    />
  </section>
</template>
