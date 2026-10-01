<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { listCategories } from '../api/categories'
import { createCoupon, deleteCoupon, listCoupons, updateCoupon } from '../api/coupons'
import { listProducts } from '../api/products'
import PaginationFooter from '../components/PaginationFooter.vue'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { closeOnEscape } from '../utils/dialog'
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
closeOnEscape(editing, null)
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
// timezone offset. The picker gives a day, the server stores an instant, and
// the two halves of the window are different instants of that day — see
// fromDateInput.
function toDateInput(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

// 生效日期 is the first moment of the chosen day; 最后可用日期 is the last one,
// otherwise a code advertised as running through the 5th dies at midnight
// going into the 5th and every buyer that morning sees it refused.
function fromDateInput(value: string, endOfDay = false): string | null {
  if (!value) return null
  return new Date(`${value}T${endOfDay ? '23:59:59.999' : '00:00:00'}`).toISOString()
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

// 启用/停用 alone is not the answer the owner is asking for when a buyer says
// 「码用不了」: a switched-on code still cannot be spent before its window, after
// it, or once its 限量 is committed. This reads the same rules the storefront
// quote runs on, so the row and the refusal agree.
function couponState(coupon: Coupon): { label: string; tone: string; note: string } {
  const now = Date.now()
  if (!coupon.is_active) return { label: '已停用', tone: 'badge-neutral', note: '买家输入会被告知「优惠码已停用」。' }
  if (coupon.valid_from && new Date(coupon.valid_from).getTime() > now) {
    return { label: '未开始', tone: 'badge-info', note: `${toDateInput(coupon.valid_from)} 那天零点才生效。` }
  }
  if (coupon.valid_until && new Date(coupon.valid_until).getTime() < now) {
    return { label: '已过期', tone: 'badge-danger', note: `${toDateInput(coupon.valid_until)} 那天结束就失效了，重开要把日期改到今后。` }
  }
  if (coupon.max_uses && (coupon.remaining ?? 0) <= 0) {
    return { label: '已抢完', tone: 'badge-warning', note: `${coupon.max_uses} 个额度都排在单上（含刚下单未付款的），付完或过期后会腾出来。` }
  }
  return { label: '可用', tone: 'badge-success', note: '买家现在就能用上。' }
}

// A shop that runs promotions keeps every code it ever made, and the owner
// arriving here is looking for one: the code a buyer just typed, or the ones
// that are refusing people. Search and filter narrow the table, and the page
// size keeps a long history from turning into an endless scroll.
const STATES = ['可用', '未开始', '已抢完', '已过期', '已停用']
const search = ref('')
const stateFilter = ref('all')
const page = ref(1)
const PAGE_SIZE = 20

const filtered = computed(() => {
  const needle = search.value.trim().toLowerCase()
  return coupons.value.filter((coupon) => {
    if (stateFilter.value !== 'all' && couponState(coupon).label !== stateFilter.value) return false
    if (!needle) return true
    return (
      coupon.code.toLowerCase().includes(needle) ||
      (coupon.description || '').toLowerCase().includes(needle)
    )
  })
})

const pages = computed(() => Math.max(1, Math.ceil(filtered.value.length / PAGE_SIZE)))
const current = computed(() => Math.min(page.value, pages.value))
const shown = computed(() => {
  const start = (current.value - 1) * PAGE_SIZE
  return filtered.value.slice(start, start + PAGE_SIZE)
})
const summary = computed(() => {
  if (!filtered.value.length) return ''
  const start = (current.value - 1) * PAGE_SIZE + 1
  const end = Math.min(current.value * PAGE_SIZE, filtered.value.length)
  return `第 ${start}–${end} 条 · 共 ${filtered.value.length} 条`
})

watch([search, stateFilter], () => {
  page.value = 1
})

function clearFilters() {
  search.value = ''
  stateFilter.value = 'all'
  page.value = 1
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

    <div v-if="coupons.length" class="flex flex-wrap items-center gap-2">
      <input
        v-model="search"
        class="input max-w-[240px]"
        type="search"
        aria-label="搜索优惠码"
        placeholder="搜码或活动说明…"
      />
      <select v-model="stateFilter" class="input w-auto" aria-label="按状态筛选优惠码">
        <option value="all">全部状态</option>
        <option v-for="label in STATES" :key="label" :value="label">{{ label }}</option>
      </select>
      <p class="quiet text-xs">筛出 {{ filtered.length }} / {{ coupons.length }} 个码</p>
    </div>

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
          <tr v-else-if="!shown.length">
            <td colspan="9">
              <div class="empty-state">
                <p class="empty-title">没有符合条件的优惠码</p>
                <p class="empty-hint">
                  {{ coupons.length }} 个码里没有匹配的，换个关键字或把状态筛选放开。
                </p>
                <button class="btn btn-secondary btn-sm mt-3" type="button" @click="clearFilters">
                  清空筛选
                </button>
              </div>
            </td>
          </tr>
          <tr v-for="coupon in shown" :key="coupon.id">
            <td>
              <code class="mono text-sm">{{ coupon.code }}</code>
              <p v-if="coupon.description" class="quiet max-w-[220px] truncate text-xs">{{ coupon.description }}</p>
            </td>
            <td class="nums text-sm">{{ discountLabel(coupon) }}</td>
            <td class="text-sm muted">{{ scopeLabel(coupon) }}</td>
            <td class="nums text-sm muted">{{ coupon.min_order_amount ? money(coupon.min_order_amount) : '不限' }}</td>
            <td class="nums text-sm">
              <p>{{ coupon.used_count }} / {{ coupon.max_uses || '∞' }}</p>
              <p v-if="coupon.max_uses" class="quiet text-xs">在单 {{ coupon.held ?? 0 }} · 剩 {{ coupon.remaining ?? coupon.max_uses }}</p>
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
              <p class="mt-1">
                <span class="badge" :class="couponState(coupon).tone" :title="couponState(coupon).note">
                  {{ couponState(coupon).label }}
                </span>
              </p>
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

    <PaginationFooter
      v-if="filtered.length > PAGE_SIZE"
      :page="current"
      :pages="pages"
      :loading="loading"
      :summary="summary"
      @change="page = $event"
    />

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
          <div class="grid gap-3 sm:grid-cols-2">
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
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="label" for="k-min">最低消费</label>
              <input id="k-min" v-model.number="editing.min_order_amount" type="number" min="0" class="input nums" />
            </div>
            <div>
              <label class="label" for="k-max">总次数上限（0=不限）</label>
              <input id="k-max" v-model.number="editing.max_uses" type="number" min="0" class="input nums" />
            </div>
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
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
          <div class="grid gap-3 sm:grid-cols-2">
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
              <label class="label" for="k-until">最后可用日期</label>
              <input
                id="k-until"
                :value="toDateInput(editing.valid_until)"
                type="date"
                class="input"
                @input="editing.valid_until = fromDateInput(($event.target as HTMLInputElement).value, true)"
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
