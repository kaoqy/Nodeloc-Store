<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ProductCard from '../components/ProductCard.vue'
import { couponQuoteMessage, getProduct, listProducts, quoteCoupon } from '../api/products'
import { createOrder, createPayment } from '../api/payment'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import { setPageTitle } from '../utils/identity'
import { money } from '../utils/format'
import type { CouponQuote, Product } from '../types'

const MaxQuantity = 20

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const site = useSiteStore()

const product = ref<Product | null>(null)
const quantity = ref(1)
const contact = ref('')
const note = ref('')
const loading = ref(true)
const submitting = ref(false)
const error = ref('')
const related = ref<Product[]>([])

const couponCode = ref('')
const quote = ref<CouponQuote | null>(null)
const couponError = ref('')
const quoting = ref(false)

const cardStock = computed(() => {
  const item = product.value
  if (!item || item.product_type !== 'card' || !item.auto_deliver) return null
  return item.stock_count
})

const limit = computed(() => {
  if (cardStock.value === null) return MaxQuantity
  return Math.min(MaxQuantity, Math.max(cardStock.value, 0))
})

const soldOut = computed(() => limit.value === 0)
const gross = computed(() => (product.value?.price ?? 0) * quantity.value)
/** What NodeLoc is asked to collect: the quoted discount is already off it. */
const payable = computed(() => (quote.value?.accepted ? quote.value.payable : gross.value))
const discount = computed(() => (quote.value?.accepted ? quote.value.discount : 0))

function step(delta: number) {
  quantity.value = Math.min(limit.value, Math.max(1, quantity.value + delta))
}

/**
 * The code is checked against the order the buyer is actually looking at, so a
 * 满 100 减 20 code says "还差多少" before checkout instead of failing at 支付.
 */
async function applyQuote() {
  const item = product.value
  couponError.value = ''
  quote.value = null
  if (!item) return
  const code = couponCode.value.trim()
  if (!code) return
  if (!auth.isAuthenticated) {
    couponError.value = '登录后才能使用优惠码。'
    return
  }
  quoting.value = true
  try {
    quote.value = await quoteCoupon({ code, slug: item.slug, quantity: quantity.value })
  } catch (e) {
    couponError.value = couponQuoteMessage(e)
  } finally {
    quoting.value = false
  }
}

watch(quantity, () => {
  if (couponCode.value.trim()) void applyQuote()
})

async function purchase() {
  const item = product.value
  if (!item || submitting.value) return
  if (!auth.isAuthenticated) {
    await router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  submitting.value = true
  error.value = ''
  try {
    const order = await createOrder({
      slug: item.slug,
      quantity: quantity.value,
      contact: contact.value.trim() || undefined,
      note: note.value.trim() || undefined,
      coupon_code: quote.value?.accepted ? quote.value.code : undefined,
    })
    const payment = await createPayment(order.order_no, item.name)
    if (!payment.payment_url) throw new Error('支付通道未返回付款地址，请稍后在订单页重试')
    window.location.href = payment.payment_url
  } catch (e) {
    error.value = errorMessage(e, '下单失败，请稍后重试')
    submitting.value = false
  }
}

async function loadRelated(item: Product) {
  if (!item.category_id) return
  try {
    const list = await listProducts({ category: item.category_id, limit: 4 })
    related.value = list.data.filter((other) => other.id !== item.id).slice(0, 3)
  } catch {
    related.value = []
  }
}

onMounted(async () => {
  try {
    product.value = await getProduct(String(route.params.slug))
    // The tab says which goods the visitor is reading about, not just which shop.
    setPageTitle(product.value?.name)
    if (product.value) void loadRelated(product.value)
  } catch (e) {
    error.value = errorMessage(e, '商品加载失败')
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6">
    <div v-if="loading" class="grid gap-8 lg:grid-cols-[1.5fr_1fr]">
      <div class="space-y-4">
        <div class="skeleton aspect-[16/9] w-full !rounded-lg" />
        <div class="skeleton h-7 w-1/2" />
        <div class="skeleton h-4 w-full" />
        <div class="skeleton h-4 w-5/6" />
      </div>
      <div class="skeleton h-80 w-full !rounded-lg" />
    </div>

    <p v-else-if="!product" class="alert alert-danger max-w-xl" role="alert">
      {{ error || '未找到该商品，它可能已经下架。' }}
    </p>

    <div v-else class="fade-in grid items-start gap-8 lg:grid-cols-[1.5fr_1fr]">
      <section>
        <RouterLink to="/" class="hint inline-flex items-center gap-1.5 transition-colors hover:text-[var(--text)]">
          ← 全部商品
        </RouterLink>

        <div class="card mt-4 overflow-hidden !p-0">
          <div class="aspect-[16/9] w-full bg-[var(--surface-sunken)]">
            <img
              v-if="product.image_path"
              :src="product.image_path"
              :alt="product.name"
              class="h-full w-full object-cover"
            />
            <div v-else class="grid h-full place-items-center">
              <span class="mono text-3xl font-bold tracking-[0.24em] text-[var(--text-quiet)]/45">
                {{ product.name.slice(0, 2).toUpperCase() }}
              </span>
            </div>
          </div>

          <div class="p-6 sm:p-8">
            <div class="flex flex-wrap items-center gap-2">
              <span v-if="product.category" class="badge badge-neutral">{{ product.category.name }}</span>
              <span class="badge" :class="product.product_type === 'card' ? 'badge-teal' : 'badge-accent'">
                {{ product.product_type === 'card' ? '付款后自动交付' : '商家人工交付' }}
              </span>
              <span v-if="product.is_featured" class="badge badge-accent">店长推荐</span>
            </div>

            <h1 class="mt-4 text-3xl font-bold">{{ product.name }}</h1>
            <p v-if="product.summary" class="mt-2 text-[15px] text-[var(--text-dim)]">{{ product.summary }}</p>
            <p class="hint mt-2 nums">已售 {{ product.sold_count ?? 0 }} 件</p>

            <div v-if="product.description" class="my-6 divider" />

            <p v-if="product.description" class="whitespace-pre-line text-[15px] leading-7 text-[var(--text-dim)]">
              {{ product.description }}
            </p>
          </div>
        </div>

        <section v-if="related.length" class="mt-10">
          <div class="flex items-baseline justify-between gap-3">
            <h2 class="text-[15px] font-bold">同类推荐</h2>
            <RouterLink to="/" class="hint transition-colors hover:text-[var(--text)]">查看全部 →</RouterLink>
          </div>
          <div class="mt-4 grid gap-5 sm:grid-cols-2">
            <ProductCard v-for="item in related" :key="item.id" :product="item" />
          </div>
        </section>
      </section>

      <aside class="card lg:sticky lg:top-24">
        <div class="flex items-baseline justify-between gap-3">
          <span class="label !mb-0">单价</span>
          <span class="nums text-2xl font-bold">{{ money(product.price) }}</span>
        </div>
        <p v-if="product.original_price && product.original_price > product.price" class="mt-1 text-right">
          <span class="nums hint line-through">{{ money(product.original_price) }}</span>
        </p>

        <div class="my-5 divider" />

        <form class="space-y-4" @submit.prevent="purchase">
          <div class="flex items-center justify-between gap-3">
            <span class="label !mb-0">数量</span>
            <div class="stepper">
              <button type="button" :disabled="quantity <= 1 || soldOut" aria-label="减少数量" @click="step(-1)">−</button>
              <span class="stepper-value py-2">{{ soldOut ? 0 : quantity }}</span>
              <button type="button" :disabled="quantity >= limit" aria-label="增加数量" @click="step(1)">+</button>
            </div>
          </div>

          <div v-if="cardStock !== null" class="hint -mt-1">
            现货 <span class="nums">{{ cardStock }}</span> 件{{ soldOut ? '，暂时缺货' : '' }}
          </div>

          <div v-if="product.require_contact">
            <label class="label" for="contact">联系方式 <span class="accent-text">*</span></label>
            <input
              id="contact"
              v-model="contact"
              class="input"
              required
              maxlength="255"
              placeholder="商家交付时需要用到的账号或邮箱"
            />
            <p class="hint mt-1.5">仅商家可见，用于向你交付商品。</p>
          </div>

          <div>
            <label class="label" for="note">备注</label>
            <textarea id="note" v-model="note" class="input" maxlength="500" placeholder="选填，例如规格要求"></textarea>
          </div>

          <div v-if="site.couponsEnabled">
            <label class="label" for="coupon">优惠码</label>
            <div class="flex gap-2">
              <input
                id="coupon"
                v-model="couponCode"
                class="input flex-1"
                maxlength="64"
                placeholder="选填，例如 NEWBIE20"
                @keyup.enter="applyQuote"
              />
              <button class="btn btn-secondary btn-sm shrink-0" type="button" :disabled="quoting" @click="applyQuote">
                {{ quoting ? '核对中…' : '使用' }}
              </button>
            </div>
            <p v-if="quote?.accepted" class="alert alert-success mt-2">
              已优惠 {{ money(quote.discount) }}
              <span v-if="quote.description"> · {{ quote.description }}</span>
            </p>
            <p v-else-if="couponError" class="alert alert-warning mt-2" role="status">{{ couponError }}</p>
            <p v-else-if="!auth.isAuthenticated" class="hint mt-1.5">登录后即可核对优惠码。</p>
          </div>

          <div class="divider" />

          <div class="space-y-1.5 text-sm">
            <div class="flex items-baseline justify-between text-[var(--text-dim)]">
              <span>小计</span>
              <span class="nums">{{ money(gross) }}</span>
            </div>
            <div v-if="discount" class="flex items-baseline justify-between text-[var(--success)]">
              <span>优惠码减免</span>
              <span class="nums">-{{ money(discount) }}</span>
            </div>
            <div class="flex items-baseline justify-between">
              <span class="text-[var(--text-dim)]">应付合计</span>
              <span class="nums accent-text text-2xl font-bold">{{ money(payable) }}</span>
            </div>
          </div>

          <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

          <button class="btn btn-primary btn-lg w-full" type="submit" :disabled="submitting || soldOut">
            <span v-if="submitting" class="spinner spinner-light" />
            {{ soldOut ? '暂时缺货' : submitting ? '正在跳转支付…' : '立即购买' }}
          </button>

          <p class="hint text-center">
            {{ auth.isAuthenticated ? '点击后跳转至 Nodeloc Payments 完成扣款' : '登录后即可下单，订单会保留你的选择' }}
          </p>
        </form>

        <ul class="mt-5 space-y-1.5 text-xs text-[var(--text-quiet)]">
          <li class="flex items-center gap-2"><span class="size-1 rounded-full bg-[var(--teal)]" />支付成功即可在订单页查看交付结果</li>
          <li class="flex items-center gap-2"><span class="size-1 rounded-full bg-[var(--info)]" />未支付的订单可随时继续付款</li>
        </ul>
      </aside>
    </div>
  </div>
</template>
