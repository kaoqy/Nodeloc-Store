<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { createCoupon, deleteCoupon, listCoupons, updateCoupon } from '../api/coupons'
import { listCategories } from '../api/categories'
import { listProducts } from '../api/products'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Category, Coupon, Product } from '../types'

/** 优惠券：优惠码、折扣方式、适用范围与限用规则。 */

const auth = useAuthStore()
const canManage = computed(() => auth.allows('coupons', 'manage'))

const COLUMNS: Column[] = [
  { label: '优惠码' },
  { label: '优惠', hideOnMobile: true },
  { label: '适用范围', hideOnMobile: true },
  { label: '用量', numeric: true },
  { label: '有效期', hideOnMobile: true },
  { label: '状态' },
  { label: '', actions: true, width: '190px' },
]

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const coupons = ref<Coupon[]>([])
const products = ref<Product[]>([])
const categories = ref<Category[]>([])
const search = ref('')
const scopeFilter = ref('all')
const editing = ref<Partial<Coupon> | null>(null)

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return coupons.value.filter((c) => {
    if (scopeFilter.value !== 'all' && (c.scope || 'all') !== scopeFilter.value) return false
    if (!needle) return true
    return c.code.toLowerCase().includes(needle) || (c.description || '').toLowerCase().includes(needle)
  })
})

const scopeLabel = (coupon: Coupon) => {
  if (coupon.scope === 'product') return '商品：' + (products.value.find((p) => p.id === coupon.product_id)?.name || '#' + coupon.product_id)
  if (coupon.scope === 'category') return '分类：' + (categories.value.find((c) => c.id === coupon.category_id)?.name || '#' + coupon.category_id)
  return '全场通用'
}

const discountLabel = (coupon: Coupon) =>
  coupon.discount_type === 'percent' ? coupon.discount_value + '%' : money(coupon.discount_value)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, productList, categoryList] = await Promise.all([
      listCoupons(),
      listProducts().catch(() => [] as Product[]),
      listCategories().catch(() => [] as Category[]),
    ])
    coupons.value = list
    products.value = productList
    categories.value = categoryList
  } catch (err) {
    error.value = errorMessage(err, '加载优惠券失败')
  } finally {
    loading.value = false
  }
}

function startCreate() {
  editing.value = {
    code: '', discount_type: 'percent', discount_value: 10, min_order_amount: 0,
    max_uses: 0, per_user_limit: 0, is_active: true, advertised: false, scope: 'all',
    description: '',
  }
}

async function save() {
  const draft = editing.value
  if (!draft) return
  if (!draft.code?.trim()) {
    error.value = '请填写优惠码。'
    return
  }
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    if (draft.id) await updateCoupon(draft.id, draft)
    else await createCoupon(draft)
    notice.value = draft.id ? '优惠码已更新。' : '优惠码已创建。'
    editing.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存失败')
  } finally {
    busy.value = false
  }
}

async function toggleAdvertise(coupon: Coupon) {
  busy.value = true
  try {
    const next = await updateCoupon(coupon.id, { ...coupon, advertised: !coupon.advertised })
    coupons.value = coupons.value.map((c) => (c.id === next.id ? next : c))
    notice.value = next.advertised ? '已在前台展示这个优惠码。' : '已从前台撤下这个优惠码。'
  } catch (err) {
    error.value = errorMessage(err, '更新失败')
  } finally {
    busy.value = false
  }
}

async function remove(coupon: Coupon) {
  if (!window.confirm('删除优惠码「' + coupon.code + '」？已使用它的订单不受影响。')) return
  busy.value = true
  try {
    await deleteCoupon(coupon.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除失败')
  } finally {
    busy.value = false
  }
}

function clearFilters() {
  search.value = ''
  scopeFilter.value = 'all'
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <PageHeader title="优惠券" description="优惠码的折扣方式、使用范围、限用次数与前台展示开关。" bordered>
      <template #actions>
        <button v-if="canManage" class="btn btn-primary btn-sm" @click="startCreate">
          <AdminIcon name="plus" :size="14" />
          新建优惠码
        </button>
      </template>
    </PageHeader>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <FilterBar :count="'共 ' + filtered.length + ' 个优惠码'">
      <input v-model="search" class="input w-52" type="search" placeholder="搜码或说明" aria-label="搜索优惠券" />
      <select v-model="scopeFilter" class="input !w-auto" aria-label="适用范围">
        <option value="all">全部范围</option>
        <option value="all_scope">全场</option>
        <option value="product">指定商品</option>
        <option value="category">指定分类</option>
      </select>
      <template #actions>
        <button v-if="search || scopeFilter !== 'all'" class="btn btn-quiet btn-sm" @click="clearFilters">清除筛选</button>
      </template>
    </FilterBar>

    <DataTable
      :columns="COLUMNS"
      :loading="loading"
      :error="error"
      :filtered="Boolean(search.trim() || scopeFilter !== 'all')"
      :total="filtered.length"
      :summary="'共 ' + filtered.length + ' 个优惠码'"
      empty-title="还没有优惠码"
      empty-hint="创建优惠码后可以在活动里展示，也可以在结算时填写。"
      @retry="load"
      @clear-filters="clearFilters"
    >
      <tr v-for="coupon in filtered" :key="coupon.id">
        <td>
          <p class="mono font-semibold">{{ coupon.code }}</p>
          <p v-if="coupon.description" class="quiet mt-0.5 truncate text-xs">{{ coupon.description }}</p>
        </td>
        <td class="hide-on-mobile">
          {{ discountLabel(coupon) }}
          <span v-if="coupon.min_order_amount" class="quiet block text-[11px]">满 {{ money(coupon.min_order_amount) }}</span>
        </td>
        <td class="hide-on-mobile text-xs">{{ scopeLabel(coupon) }}</td>
        <td class="nums">
          {{ coupon.used_count ?? 0 }}
          <span class="quiet">/ {{ coupon.max_uses || '∞' }}</span>
        </td>
        <td class="quiet hide-on-mobile text-xs">
          {{ coupon.valid_from ? when(coupon.valid_from) : '不限' }}<br />
          {{ coupon.valid_until ? when(coupon.valid_until) : '不限' }}
        </td>
        <td>
          <div class="flex flex-wrap gap-1">
            <StatusBadge :value="coupon.is_active ? 'ok' : 'disabled'" :label="coupon.is_active ? '启用' : '停用'" />
            <span v-if="coupon.advertised" class="badge-info">前台展示</span>
          </div>
        </td>
        <td class="text-right">
          <div class="flex flex-wrap justify-end gap-1.5">
            <button v-if="canManage" class="btn btn-quiet btn-sm" @click="editing = { ...coupon }">编辑</button>
            <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggleAdvertise(coupon)">
              {{ coupon.advertised ? '撤下' : '展示' }}
            </button>
            <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="remove(coupon)">删除</button>
          </div>
        </td>
      </tr>
    </DataTable>

    <AppDrawer :open="Boolean(editing)" :title="editing?.id ? '编辑优惠码' : '新建优惠码'" @close="editing = null">
      <div v-if="editing" class="space-y-3">
        <div class="grid gap-3 sm:grid-cols-2">
          <div>
            <label class="label" for="cp-code">优惠码</label>
            <input id="cp-code" v-model="editing.code" class="input mono uppercase" placeholder="SUMMER10" maxlength="64" />
          </div>
          <div>
            <label class="label" for="cp-desc">说明</label>
            <input id="cp-desc" v-model="editing.description" class="input" placeholder="新人首单立减" maxlength="255" />
          </div>
          <div>
            <label class="label" for="cp-type">折扣方式</label>
            <select id="cp-type" v-model="editing.discount_type" class="input">
              <option value="percent">百分比折扣</option>
              <option value="fixed">固定金额</option>
            </select>
          </div>
          <div>
            <label class="label" for="cp-value">折扣值</label>
            <input id="cp-value" v-model.number="editing.discount_value" class="input nums" type="number" min="1" />
          </div>
          <div>
            <label class="label" for="cp-min">最低消费（0 为不限）</label>
            <input id="cp-min" v-model.number="editing.min_order_amount" class="input nums" type="number" min="0" />
          </div>
          <div>
            <label class="label" for="cp-max">总量限用（0 为不限）</label>
            <input id="cp-max" v-model.number="editing.max_uses" class="input nums" type="number" min="0" />
          </div>
          <div>
            <label class="label" for="cp-per">每人限用（0 为不限）</label>
            <input id="cp-per" v-model.number="editing.per_user_limit" class="input nums" type="number" min="0" />
          </div>
          <div>
            <label class="label" for="cp-scope">适用范围</label>
            <select id="cp-scope" v-model="editing.scope" class="input">
              <option value="all">全场通用</option>
              <option value="product">指定商品</option>
              <option value="category">指定分类</option>
            </select>
          </div>
          <div v-if="editing.scope === 'product'">
            <label class="label" for="cp-product">商品</label>
            <select id="cp-product" v-model.number="editing.product_id" class="input">
              <option :value="undefined">请选择</option>
              <option v-for="p in products" :key="p.id" :value="p.id">{{ p.name }}</option>
            </select>
          </div>
          <div v-if="editing.scope === 'category'">
            <label class="label" for="cp-category">分类</label>
            <select id="cp-category" v-model.number="editing.category_id" class="input">
              <option :value="undefined">请选择</option>
              <option v-for="c in categories" :key="c.id" :value="c.id">{{ c.name }}</option>
            </select>
          </div>
        </div>
        <div class="flex flex-wrap items-center gap-4">
          <label class="flex items-center gap-2 text-sm"><input v-model="editing.is_active" type="checkbox" />启用</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="editing.advertised" type="checkbox" />在前台展示</label>
        </div>
        <p class="quiet text-xs">开启「在前台展示」后，这个优惠码会出现在商品详情页的促销区，任何买家都能看到。</p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="editing = null">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存' }}</button>
        </div>
      </template>
    </AppDrawer>
  </section>
</template>
