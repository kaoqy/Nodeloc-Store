<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import ProductCard from '../components/ProductCard.vue'
import { couponQuoteMessage, getProduct, listProducts, listStoreCoupons, quoteCoupon } from '../api/products'
import { checkoutAdvice, createOrder, createPayment, paymentSettled } from '../api/payment'
import { errorMessage, errorStatus } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import { setPageTitle } from '../utils/identity'
import { money, when } from '../utils/format'
import type { CouponQuote, Order, Product, StorefrontCoupon } from '../types'

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
// Set once an order exists but its 下单 request was refused, so the page can
// point at that order instead of leaving the buyer to start a second one.
const unpaidOrderNo = ref('')
const payAdvice = ref('')
const related = ref<Product[]>([])

const couponCode = ref('')
const quote = ref<CouponQuote | null>(null)
const couponError = ref('')
const quoting = ref(false)
const promos = ref<StorefrontCoupon[]>([])

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
const buyLabel = computed(() => {
  if (!site.paymentsEnabled) return '本店暂停收款'
  if (soldOut.value) return '暂时缺货'
  return submitting.value ? '正在跳转支付…' : '立即购买'
})
const gross = computed(() => (product.value?.price ?? 0) * quantity.value)
/** What NodeLoc is asked to collect: the quoted discount is already off it. */
const payable = computed(() => (quote.value?.accepted ? quote.value.payable : gross.value))
const discount = computed(() => (quote.value?.accepted ? quote.value.discount : 0))

/**
 * The shelf only shows the promotions this item can actually be bought with: a
 * code written for another product or another category would be refused the
 * moment the buyer pressed it, and a chip that cannot work is worse than none.
 */
const applicablePromos = computed(() => {
  const item = product.value
  if (!item) return []
  return promos.value.filter((promo) => {
    if (promo.scope === 'product') return Number(promo.product_id) === item.id
    if (promo.scope === 'category') return Boolean(item.category_id) && Number(promo.category_id) === Number(item.category_id)
    return true
  })
})

function promoWorth(promo: StorefrontCoupon): string {
  return promo.discount_type === 'percent' ? `立减 ${promo.discount_value}%` : `立减 ${money(promo.discount_value)}`
}

/** Read the code into the field and price it against this order right away. */
function usePromo(promo: StorefrontCoupon) {
  couponCode.value = promo.code
  void applyQuote()
}

async function loadPromos() {
  if (!site.couponsEnabled) {
    promos.value = []
    return
  }
  try {
    promos.value = await listStoreCoupons()
  } catch {
    // A shelf that failed to load must not take the purchase down with it: the
    // 优惠码 field still works with a code the buyer already has.
    promos.value = []
  }
}

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

/**
 * Ask NodeLoc for the payment page of one order. A refused 下单 leaves the order
 * itself in place, so the failure copy has to say where that order went rather
 * than send the buyer back to 立即购买 for a second one.
 */
async function openPayment(orderNo: string, goods: string) {
  submitting.value = true
  error.value = ''
  payAdvice.value = ''
  try {
    const payment = await createPayment(orderNo, goods)
    if (paymentSettled(payment)) {
      unpaidOrderNo.value = ''
      await router.replace({ name: 'order-detail', params: { orderNo }, query: { pay: 'ok' } })
      return
    }
    if (!payment.payment_url) throw new Error('支付通道未返回付款地址，请稍后在订单页重试')
    window.location.href = payment.payment_url
  } catch (e) {
    error.value = errorMessage(e, '发起支付失败')
    payAdvice.value = checkoutAdvice(e)
    submitting.value = false
  }
}

async function purchase() {
  const item = product.value
  if (!item || submitting.value) return
  if (!site.paymentsEnabled) return
  if (!auth.isAuthenticated) {
    await router.push({ name: 'login', query: { redirect: route.fullPath } })
    return
  }
  submitting.value = true
  error.value = ''
  payAdvice.value = ''
  unpaidOrderNo.value = ''
  const code = couponCode.value.trim()
  let couponForOrder: string | undefined
  if (code) {
    // A code the buyer typed has to either be priced into this order or stop the
    // click in front of them. Sending it only when an earlier quote happened to
    // land charges full price for a discount the buyer can still see on screen.
    if (quote.value?.accepted && quote.value.code === code) {
      couponForOrder = code
    } else {
      await applyQuote()
      if (!quote.value?.accepted || quote.value.code !== code) {
        submitting.value = false
        error.value = couponError.value || '优惠码无法使用，请修正后再下单。'
        return
      }
      couponForOrder = quote.value.code
    }
  }
  let order: Order
  try {
    order = await createOrder({
      slug: item.slug,
      quantity: quantity.value,
      contact: contact.value.trim() || undefined,
      note: note.value.trim() || undefined,
      coupon_code: couponForOrder,
    })
  } catch (e) {
    error.value = errorMessage(e, '下单失败，请稍后重试')
    submitting.value = false
    return
  }
  unpaidOrderNo.value = order.order_no
  await openPayment(order.order_no, item.name)
}

/** 再试一次支付：重发这一单的 下单请求，不再新建订单。 */
async function retryPayment() {
  const orderNo = unpaidOrderNo.value
  const item = product.value
  if (!orderNo || !item || submitting.value) return
  await openPayment(orderNo, item.name)
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

/** Read the goods this address names, starting every product-only panel fresh. */
async function load(slug: string) {
  loading.value = true
  error.value = ''
  product.value = null
  related.value = []
  quantity.value = 1
  couponCode.value = ''
  quote.value = null
  couponError.value = ''
  promos.value = []
  submitting.value = false
  try {
    product.value = await getProduct(slug)
    // The tab says which goods the visitor is reading about, not just which shop.
    setPageTitle(product.value?.name)
    if (product.value) {
      void loadRelated(product.value)
      void loadPromos()
    }
  } catch (e) {
    // An address for goods the shop does not carry is not a fault worth
    // reporting: the empty state below already says the item is gone.
    error.value = errorStatus(e) === 404 ? '' : errorMessage(e, '商品加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(() => void load(String(route.params.slug)))

// Related goods link straight to each other, and both addresses render this same
// component. Without reloading on the slug the page keeps the item the visitor
// came from while the address already names another one.
watch(
  () => route.params.slug,
  (next) => void load(String(next ?? '')),
)
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

    <div v-else-if="!product" class="card mx-auto max-w-xl py-14 text-center">
      <p class="eyebrow">{{ error ? '加载失败' : '不在架上' }}</p>
      <h1 class="mt-3 text-lg font-semibold">{{ error || '未找到该商品，它可能已经下架。' }}</h1>
      <p class="mt-2 text-sm leading-relaxed text-[var(--text-dim)]">
        {{ error ? '网络或服务暂时没应答，可以直接重试一次。' : '下架的商品不会再用旧链接打开，货架上还有其他可选。' }}
      </p>
      <div class="mt-7 flex flex-wrap justify-center gap-2">
        <button v-if="error" class="btn btn-primary btn-sm" @click="load(String(route.params.slug))">再试一次</button>
        <RouterLink v-else to="/" class="btn btn-primary btn-sm">去挑选商品</RouterLink>
        <RouterLink v-if="auth.isAuthenticated" to="/orders" class="btn btn-quiet btn-sm">我的订单</RouterLink>
      </div>
    </div>

    <div v-else class="fade-in grid items-start gap-8 lg:grid-cols-[1.5fr_1fr]">
      <section class="min-w-0">
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

            <h1 class="mt-4 break-words text-3xl font-bold">{{ product.name }}</h1>
            <p v-if="product.summary" class="mt-2 text-[15px] text-[var(--text-dim)]">{{ product.summary }}</p>
            <p class="hint mt-2 nums">已售 {{ product.sold_count ?? 0 }} 件</p>

            <div v-if="product.description" class="my-6 divider" />

            <p v-if="product.description" class="break-words whitespace-pre-line text-[15px] leading-7 text-[var(--text-dim)]">
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

          <ul v-if="applicablePromos.length" class="space-y-2">
            <li v-for="promo in applicablePromos" :key="promo.code" class="card-quiet px-3 py-2.5">
              <div class="flex items-start justify-between gap-3">
                <div class="min-w-0">
                  <p class="mono text-sm font-semibold">{{ promo.code }}</p>
                  <p class="hint mt-1 break-words">
                    {{ promoWorth(promo) }}
                    <span v-if="promo.min_order_amount"> · 满 {{ money(promo.min_order_amount) }} 可用</span>
                    <span v-if="promo.description"> · {{ promo.description }}</span>
                  </p>
                  <p v-if="promo.min_order_amount > gross" class="mt-1 text-xs text-[var(--warning)]">
                    这个单还差 {{ money(promo.min_order_amount - gross) }}，多加一件就能用。
                  </p>
                  <p class="hint mt-1">
                    <span v-if="promo.valid_until">有效期至 {{ when(promo.valid_until) }}</span>
                    <span v-if="promo.remaining !== undefined && promo.remaining !== null">
                      <span v-if="promo.valid_until"> ·</span> 仅剩 {{ promo.remaining }} 次
                    </span>
                    <span v-if="promo.per_user_limit"> · 每人 {{ promo.per_user_limit }} 次</span>
                  </p>
                </div>
                <button class="btn btn-quiet btn-sm shrink-0" type="button" :disabled="quoting" @click="usePromo(promo)">
                  用这个
                </button>
              </div>
            </li>
          </ul>

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
            <p v-if="quote?.accepted" class="alert alert-success mt-2 break-words">
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

          <div v-if="error" class="alert alert-danger" role="alert">
            <p>{{ error }}</p>
            <p v-if="payAdvice" class="mt-1 font-normal">{{ payAdvice }}</p>
            <div v-if="unpaidOrderNo" class="mt-2.5 flex flex-wrap items-center gap-2 font-normal">
              <button class="btn btn-secondary btn-sm" type="button" :disabled="submitting" @click="retryPayment">
                再试一次支付
              </button>
              <RouterLink :to="`/orders/${unpaidOrderNo}`" class="hint underline hover:text-[var(--text)]">
                或去订单 {{ unpaidOrderNo }} 继续支付
              </RouterLink>
            </div>
          </div>

          <button
            class="btn btn-primary btn-lg w-full"
            type="submit"
            :disabled="submitting || soldOut || !site.paymentsEnabled"
          >
            <span v-if="submitting" class="spinner spinner-light" />
            {{ buyLabel }}
          </button>

          <p class="hint text-center">
            {{
              !site.paymentsEnabled
                ? '店家已在后台关闭收款，重新打开后即可下单。'
                : auth.isAuthenticated
                  ? '点击后跳转至 Nodeloc Payments 完成扣款'
                  : '登录后即可下单，订单会保留你的选择'
            }}
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
