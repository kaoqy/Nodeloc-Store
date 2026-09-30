<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { listCategories } from '../api/categories'
import { createCoupon, deleteCoupon, listCoupons, updateCoupon } from '../api/coupons'
import { listProducts } from '../api/products'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Category, Coupon, Product } from '../types'

const auth = useAuthStore()
const canManage = computed(() => auth.allows('coupons', 'manage'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const coupons = ref<Coupon[]>([])
const categories = ref<Category[]>([])
const products = ref<Product[]>([])
const editing = ref<Partial<Coupon> | null>(null)
const toggling = ref<number | null>(null)

const SCOPE_LABEL: Record<string, string> = {
  all: '全场可用',
  category: '限定分类',
  product: '限定商品',
}

const scopeProducts = computed(() => {
  if (!editing.value) return []
  if (editing.value.scope !== 'category') return products.value
  const categoryID = Number(editing.value.category_id) || 0
  if (!categoryID) return products.value
  return products.value.filter((item) => Number(item.category_id) === categoryID)
})

function scopeLabel(coupon: Coupon): string {
  const scope = coupon.scope && SCOPE_LABEL[coupon.scope] ? coupon.scope : 'all'
  if (scope === 'product') {
    const product = products.value.find((item) => item.id === Number(coupon.product_id))
    return `限 ${product?.name || `商品 #${coupon.product_id ?? '—'}`}`
  }
  if (scope === 'category') {
    const category = categories.value.find((item) => item.id === Number(coupon.category_id))
    return `限 ${category?.name || `分类 #${coupon.category_id ?? '—'}`}`
  }
  return SCOPE_LABEL.all
}

const canSave = computed(() => {
  const item = editing.value
  if (!item) return false
  const value = Number(item.discount_value)
  if (!item.code?.trim() || !value || value <= 0) return false
  if (item.discount_type === 'percent' && value > 100) return false
  if (item.scope === 'product' && !Number(item.product_id)) return false
  if (item.scope === 'category' && !Number(item.category_id)) return false
  return true
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [rows, categoryRows, productRows] = await Promise.all([listCoupons(), listCategories(), listProducts()])
    coupons.value = rows
    categories.value = categoryRows
    products.value = productRows
  } catch (err) {
    error.value = errorMessage(err, '加载优惠券失败')
  } finally {
    loading.value = false
  }
}

function startCreate() {
  editing.value = {
    code: '',
    discount_type: 'fixed',
    discount_value: 0,
    min_order_amount: 0,
    max_uses: 0,
    used_count: 0,
    is_active: true,
    valid_from: null,
    valid_until: null,
    description: '',
    advertised: false,
    scope: 'all',
    category_id: null,
    product_id: null,
    per_user_limit: 0,
  }
}

function changeScope(value: string) {
  if (!editing.value) return
  editing.value.scope = value
  // The server clears the unused half of the scope, but the form should not
  // carry a stale id into the next save either.
  if (value !== 'category') editing.value.category_id = null
  if (value !== 'product') editing.value.product_id = null
}

// Dates are calendar days chosen in the admin's timezone: convert with local
// parts, never with toISOString(), whose UTC rendering shifts the day by the
// timezone offset.
function toDateInput(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function fromDateInput(value: string): string | null {
  return value ? new Date(`${value}T00:00:00`).toISOString() : null
}

async function save() {
  if (!editing.value || !canSave.value) return
  const item = editing.value
  busy.value = true
  error.value = ''
  try {
    const payload: Partial<Coupon> = {
      code: item.code?.trim().toUpperCase(),
      discount_type: item.discount_type,
      discount_value: Number(item.discount_value),
      min_order_amount: Number(item.min_order_amount) || 0,
      max_uses: Number(item.max_uses) || 0,
      used_count: item.used_count ?? 0,
      is_active: item.is_active ?? true,
      valid_from: item.valid_from || null,
      valid_until: item.valid_until || null,
      description: item.description?.trim() || null,
      // The server saves the whole row, so a PUT that left this out would switch
      // an advertised code back to private without anyone touching the box.
      advertised: item.advertised ?? false,
      scope: item.scope || 'all',
      category_id: item.scope === 'category' ? Number(item.category_id) || null : null,
      product_id: item.scope === 'product' ? Number(item.product_id) || null : null,
      per_user_limit: Number(item.per_user_limit) || 0,
    }
    if (item.id) {
      await updateCoupon(item.id, payload)
    } else {
      await createCoupon(payload)
    }
    editing.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存优惠券失败')
  } finally {
    busy.value = false
  }
}

async function remove(coupon: Coupon) {
  if (!confirm(`删除优惠码「${coupon.code}」？`)) return
  error.value = ''
  try {
    await deleteCoupon(coupon.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除优惠券失败')
  }
}

// Putting a code on the storefront shelf is the one thing worth flipping after a
// promotion is written: the discount stays the same, only its visibility moves.
async function toggleAdvertise(coupon: Coupon) {
  toggling.value = coupon.id
  error.value = ''
  try {
    await updateCoupon(coupon.id, { ...coupon, advertised: !coupon.advertised })
    await load()
  } catch (err) {
    error.value = errorMessage(err, '更新前台展示失败')
  } finally {
    toggling.value = null
  }
}

function discountLabel(coupon: Coupon): string {
  return coupon.discount_type === 'percent' ? `立减 ${coupon.discount_value}%` : `立减 ${money(coupon.discount_value)}`
}

function expiry(coupon: Coupon): string {
  if (!coupon.valid_from && !coupon.valid_until) return '长期有效'
  return `${toDateInput(coupon.valid_from) || '立即'} → ${toDateInput(coupon.valid_until) || '长期'}`
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="quiet text-sm">
        优惠码在下单时参与计价：买家在商品页输入后，折扣会直接从应付金额里扣掉。
        勾了「在前台展示」的码还会列在商品页的促销位上，买家点一下就填好，不用记字母。
      </p>
      <button v-if="canManage" class="btn btn-primary btn-sm" @click="startCreate">+ 新建优惠券</button>
    </div>

    <p v-if="!canManage" class="quiet text-xs">当前角色只能查看优惠码，新建与修改需要「优惠码管理」权限。</p>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>优惠码</th>
            <th>折扣</th>
            <th>适用范围</th>
            <th>最低消费</th>
            <th>使用情况</th>
            <th>有效期</th>
            <th>展示</th>
            <th>状态</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 5" :key="`skeleton-${i}`">
              <td colspan="9"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!coupons.length">
            <td colspan="9">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">还没有优惠码</p>
                <p v-if="canManage" class="empty-hint">新建一个码，买家在商品详情页就能用它试算折扣。</p>
              </div>
            </td>
          </tr>
          <tr v-for="coupon in coupons" :key="coupon.id">
            <td>
              <code class="mono text-sm">{{ coupon.code }}</code>
              <p v-if="coupon.description" class="quiet max-w-[220px] truncate text-xs">{{ coupon.description }}</p>
            </td>
            <td class="nums text-sm">{{ discountLabel(coupon) }}</td>
            <td class="text-sm muted">{{ scopeLabel(coupon) }}</td>
            <td class="nums text-sm muted">{{ coupon.min_order_amount ? money(coupon.min_order_amount) : '不限' }}</td>
            <td class="nums text-sm">
              <p>{{ coupon.used_count }} / {{ coupon.max_uses || '∞' }}</p>
              <p v-if="coupon.per_user_limit" class="quiet text-xs">每人 {{ coupon.per_user_limit }} 次</p>
            </td>
            <td class="text-xs quiet">{{ expiry(coupon) }}</td>
            <td>
              <button
                v-if="canManage"
                class="badge cursor-pointer transition-opacity hover:opacity-80"
                :class="coupon.advertised ? 'badge-teal' : 'badge-neutral'"
                :title="coupon.advertised ? '买家在商品页能看到这个码，点击收起' : '只有拿到码的人知道，点击放到前台'"
                :disabled="toggling === coupon.id"
                @click="toggleAdvertise(coupon)"
              >
                {{ toggling === coupon.id ? '处理中…' : coupon.advertised ? '前台展示' : '仅私下' }}
              </button>
              <span v-else class="badge" :class="coupon.advertised ? 'badge-teal' : 'badge-neutral'">
                {{ coupon.advertised ? '前台展示' : '仅私下' }}
              </span>
            </td>
            <td>
              <span class="badge" :class="coupon.is_active ? 'badge-success' : 'badge-neutral'">
                {{ coupon.is_active ? '启用' : '停用' }}
              </span>
            </td>
            <td v-if="canManage" class="whitespace-nowrap text-right">
              <button class="btn btn-ghost btn-sm" @click="editing = { ...coupon }">编辑</button>
              <button class="btn btn-ghost btn-sm text-[var(--danger)]" @click="remove(coupon)">删除</button>
            </td>
            <td v-else class="text-right"><span class="quiet text-xs">只读</span></td>
          </tr>
        </tbody>
      </table>
    </div>

    <div
      v-if="editing"
      class="overlay" role="dialog" aria-modal="true" aria-label="优惠券表单"
      @click.self="editing = null"
    >
      <div class="card w-full max-w-lg !p-5">
        <h3 class="text-base font-semibold">{{ editing.id ? '编辑优惠券' : '新建优惠券' }}</h3>
        <div class="mt-4 space-y-3">
          <div>
            <label class="label" for="k-code">优惠码 *</label>
            <input id="k-code" v-model="editing.code" class="input mono uppercase" placeholder="SUMMER10" />
          </div>
          <div>
            <label class="label" for="k-desc">说明（买家可见）</label>
            <input id="k-desc" v-model="editing.description" class="input" placeholder="新人首单立减 10 NL" />
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-type">类型</label>
              <select id="k-type" v-model="editing.discount_type" class="input">
                <option value="fixed">固定金额</option>
                <option value="percent">百分比</option>
              </select>
            </div>
            <div>
              <label class="label" for="k-value">{{ editing.discount_type === 'percent' ? '折扣（%）' : '立减（NL）' }} *</label>
              <input id="k-value" v-model.number="editing.discount_value" type="number" min="1" class="input nums" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-min">最低消费</label>
              <input id="k-min" v-model.number="editing.min_order_amount" type="number" min="0" class="input nums" />
            </div>
            <div>
              <label class="label" for="k-max">总次数上限（0=不限）</label>
              <input id="k-max" v-model.number="editing.max_uses" type="number" min="0" class="input nums" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-per">每人可用次数（0=不限）</label>
              <input id="k-per" v-model.number="editing.per_user_limit" type="number" min="0" class="input nums" />
            </div>
            <div>
              <label class="label" for="k-scope">适用范围</label>
              <select
                id="k-scope"
                class="input"
                :value="editing.scope || 'all'"
                @change="changeScope(($event.target as HTMLSelectElement).value)"
              >
                <option value="all">全场</option>
                <option value="category">指定分类</option>
                <option value="product">指定商品</option>
              </select>
            </div>
          </div>
          <div v-if="editing.scope === 'category'">
            <label class="label" for="k-category">分类 *</label>
            <select id="k-category" v-model.number="editing.category_id" class="input">
              <option :value="null" disabled>请选择分类</option>
              <option v-for="category in categories" :key="category.id" :value="category.id">
                {{ category.name }}
              </option>
            </select>
          </div>
          <div v-if="editing.scope === 'product'">
            <label class="label" for="k-product">商品 *</label>
            <select id="k-product" v-model.number="editing.product_id" class="input">
              <option :value="null" disabled>请选择商品</option>
              <option v-for="product in scopeProducts" :key="product.id" :value="product.id">
                {{ product.name }}
              </option>
            </select>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-from">生效日期</label>
              <input
                id="k-from"
                :value="toDateInput(editing.valid_from)"
                type="date"
                class="input"
                @input="editing.valid_from = fromDateInput(($event.target as HTMLInputElement).value)"
              />
            </div>
            <div>
              <label class="label" for="k-until">失效日期</label>
              <input
                id="k-until"
                :value="toDateInput(editing.valid_until)"
                type="date"
                class="input"
                @input="editing.valid_until = fromDateInput(($event.target as HTMLInputElement).value)"
              />
            </div>
          </div>
          <div v-if="editing.id" class="hint">已使用 {{ editing.used_count }} 次 · 创建于 {{ when(editing.created_at) }}</div>
          <label class="flex items-center gap-2.5 text-sm">
            <input v-model="editing.is_active" type="checkbox" class="accent-[var(--accent)]" />
            启用
          </label>
          <div>
            <label class="flex items-center gap-2.5 text-sm">
              <input v-model="editing.advertised" type="checkbox" class="accent-[var(--accent)]" />
              在前台展示
            </label>
            <p class="hint mt-1">
              勾上后，这个码会出现在商品详情页的促销位上，任何访客都看得到（过期或额度用完会自动收起）。
              不勾就是私下发的码，只有拿到字的人知道。
            </p>
          </div>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="editing = null">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy || !canSave" @click="save">
            {{ busy ? '保存中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
