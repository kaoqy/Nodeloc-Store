<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import ManagementPage from '../components/ManagementPage.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { deleteProduct, listAdminProducts, updateProduct } from '../api/products'
import { money, when, errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Product } from '../types'

/**
 * 商品管理。列表走服务端分页与筛选：商品上千件时把全部记录拉到浏览器里
 * 再本地过滤既慢又会在 fetch 上限处静默漏数据。
 */

const PAGE_SIZE = 20

const router = useRouter()
const auth = useAuthStore()
const canManage = computed(() => auth.allows('products', 'manage'))

const COLUMNS: Column[] = [
  { label: '商品' },
  { label: '类型', hideOnMobile: true },
  { label: '价格 / 额度', numeric: true },
  { label: '库存', numeric: true, hideOnMobile: true },
  { label: '已售', numeric: true, hideOnMobile: true },
  { label: '状态' },
  { label: '', actions: true, width: '260px' },
]

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

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(
  () => Boolean(search.value.trim() || typeFilter.value !== 'all' || statusFilter.value !== 'all'),
)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAdminProducts({
      q: search.value,
      type: typeFilter.value,
      status: statusFilter.value,
      sort: sort.value,
      limit: PAGE_SIZE,
      offset: (page.value - 1) * PAGE_SIZE,
    })
    products.value = result.data
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  } finally {
    loading.value = false
  }
}

function apply() {
  page.value = 1
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

function clearFilters() {
  search.value = ''
  typeFilter.value = 'all'
  statusFilter.value = 'all'
  sort.value = 'newest'
  apply()
}

/** 只发送要翻转的那个布尔值；后端按补丁更新，不会覆盖其他字段。 */
async function toggle(product: Product, field: 'is_published' | 'is_featured') {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const next = await updateProduct(product.id, { [field]: !product[field] })
    products.value = products.value.map((item) => (item.id === next.id ? next : item))
  } catch (err) {
    error.value = errorMessage(err, '更新商品失败')
  } finally {
    busy.value = false
  }
}

async function remove(product: Product) {
  if (!window.confirm('删除商品「' + product.name + '」？该商品下的卡密会一并失效。')) return
  busy.value = true
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
    <ManagementPage
      title="商品管理"
      description="上架、定价、库存可见性与推荐位都在这里维护。"
      eyebrow="商品目录"
      :metrics="[
        { label: '商品总数', value: total, hint: '当前筛选结果' },
        { label: '已上架', value: products.filter((item) => item.is_published).length, hint: '本页在售商品', tone: 'success' },
        { label: '低库存', value: products.filter((item) => item.product_type === 'card' && item.stock_count <= 3).length, hint: '本页库存告急', tone: 'warning' },
      ]"
    >
      <template #actions>
        <RouterLink v-if="canManage" to="/products/new" class="btn btn-primary btn-sm">
          <AdminIcon name="plus" :size="14" />
          新建商品
        </RouterLink>
      </template>
    </ManagementPage>

    <p v-if="!canManage" class="alert" role="status">当前角色只能查看商品，编辑需要「商品管理」权限。</p>

    <FilterBar :count="total ? '共 ' + total + ' 个商品' : ''">
      <input v-model="search" class="input w-52" type="search" placeholder="名称 / slug / 简介" aria-label="搜索商品" @keyup.enter="apply" />
      <select v-model="typeFilter" class="input !w-auto" aria-label="商品类型" @change="apply">
        <option value="all">全部类型</option>
        <option value="card">卡密自动发货</option>
        <option value="manual">人工交付</option>
        <option value="new_api">New-API 兑换码</option>
      </select>
      <select v-model="statusFilter" class="input !w-auto" aria-label="商品状态" @change="apply">
        <option value="all">全部状态</option>
        <option value="published">已上架</option>
        <option value="hidden">已下架</option>
        <option value="low_stock">库存告急</option>
        <option value="archived">已归档</option>
      </select>
      <select v-model="sort" class="input !w-auto" aria-label="排序" @change="apply">
        <option value="newest">最新创建</option>
        <option value="sales">销量优先</option>
        <option value="price_desc">价格从高到低</option>
        <option value="price_asc">价格从低到高</option>
        <option value="default">按排序值</option>
      </select>
      <button class="btn btn-secondary btn-sm" @click="apply">查询</button>
      <template #actions>
        <button v-if="filtered" class="btn btn-quiet btn-sm" @click="clearFilters">清除筛选</button>
      </template>
    </FilterBar>

    <DataTable
      :columns="COLUMNS"
      :loading="loading"
      :error="error"
      :filtered="filtered"
      :page="page"
      :pages="pageCount"
      :total="total"
      :summary="'共 ' + total + ' 个商品'"
      empty-title="还没有商品"
      empty-hint="创建第一个商品后，买家就能在店铺首页看到它。"
      @retry="load"
      @clear-filters="clearFilters"
      @change="goPage"
    >
      <tr v-for="product in products" :key="product.id">
        <td>
          <div class="flex items-center gap-2.5">
            <img
              v-if="product.image_path"
              :src="product.image_path"
              :alt="product.name"
              class="size-9 shrink-0 rounded-md border border-[var(--stroke)] object-cover"
            />
            <span v-else class="grid size-9 shrink-0 place-items-center rounded-md border border-[var(--stroke)] bg-[var(--surface-sunken)] text-[11px] text-[var(--text-quiet)]">
              {{ product.name.slice(0, 1) }}
            </span>
            <span class="min-w-0">
              <RouterLink v-if="canManage" :to="'/products/' + product.id + '/edit'" class="block truncate font-semibold hover:accent-text">
                {{ product.name }}
              </RouterLink>
              <span v-else class="block truncate font-semibold">{{ product.name }}</span>
              <span class="quiet mono block truncate text-[11px]">{{ product.slug }}</span>
            </span>
          </div>
        </td>
        <td class="hide-on-mobile">
          <span v-if="product.delivery_channel === 'new_api'" class="badge-accent">New-API 兑换码</span>
          <span v-else-if="product.product_type === 'card'">卡密</span>
          <span v-else>人工交付</span>
        </td>
        <td class="nums">
          <template v-if="product.delivery_channel === 'new_api'">
            <span class="badge-accent">额度充值</span>
            <div class="quiet mt-1 text-[11px]">
              {{ product.min_topup_amount }}–{{ product.max_topup_amount }}
            </div>
          </template>
          <template v-else>{{ money(product.price) }}</template>
        </td>
        <td class="nums hide-on-mobile">
          {{ product.product_type === 'card' ? product.stock_count : '—' }}
          <span v-if="product.product_type === 'card' && product.stock_count <= 3" class="badge-warning ml-1">紧张</span>
        </td>
        <td class="nums hide-on-mobile">{{ product.sold_count ?? 0 }}</td>
        <td>
          <div class="flex flex-wrap gap-1">
            <StatusBadge :value="product.is_published ? 'published' : 'draft'" :label="product.is_published ? '已上架' : '已下架'" />
            <span v-if="product.is_featured" class="badge-accent">推荐</span>
          </div>
        </td>
        <td class="text-right">
          <div class="flex flex-wrap justify-end gap-1.5">
            <RouterLink v-if="canManage" :to="'/products/' + product.id + '/edit'" class="btn btn-quiet btn-sm">编辑</RouterLink>
            <RouterLink v-if="product.product_type === 'card'" :to="'/cards/' + product.id" class="btn btn-quiet btn-sm">卡密</RouterLink>
            <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggle(product, 'is_published')">
              {{ product.is_published ? '下架' : '上架' }}
            </button>
            <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggle(product, 'is_featured')">
              {{ product.is_featured ? '取消推荐' : '推荐' }}
            </button>
            <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="remove(product)">删除</button>
          </div>
        </td>
      </tr>
    </DataTable>
  </section>
</template>
