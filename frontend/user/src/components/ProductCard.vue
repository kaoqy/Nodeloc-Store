<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Product } from '../types'
import { money } from '../utils/format'
import { useSiteStore } from '../stores/site'

const props = defineProps<{ product: Product }>()
const site = useSiteStore()
const imageFailed = ref(false)

// New-API 是额度型商品：没有商品价格，卡片以充值额度范围代替价格，
// 不能显示 0 元或任何虚构金额。
const isTopup = computed(() => props.product.delivery_channel === 'new_api')
const topupRange = computed(() => {
  const min = Number(props.product.min_topup_amount || 0)
  const max = Number(props.product.max_topup_amount || 0)
  return min > 0 && max > 0 ? `${min} – ${max}` : ''
})

/**
 * 价格区只有三件事，必须一眼分清：
 *   pay    —— 现在真正要付的钱（有活动就是活动价）
 *   was    —— 划线价：活动前的价格，或店家标记的原价
 *   saving —— 比划线价省下的金额
 * 活动价优先于原价：活动的计算基础就是当前售价，两者同时存在时
 * 只讲这次活动，避免同屏出现两个「原价」互相打架。
 */
const onSale = computed(() => Number(props.product.activity_saving ?? 0) > 0 && Number(props.product.activity_price ?? 0) > 0)
const pay = computed(() => (onSale.value ? Number(props.product.activity_price) : props.product.price))
const was = computed(() => {
  if (onSale.value) return props.product.price
  const original = Number(props.product.original_price ?? 0)
  return original > props.product.price ? original : 0
})
const saving = computed(() => (onSale.value ? Number(props.product.activity_saving) : was.value ? was.value - props.product.price : 0))
const offPercent = computed(() => (was.value > 0 ? Math.round((saving.value / was.value) * 100) : 0))

const soldOut = computed(
  () =>
    !isTopup.value &&
    props.product.stock_visible &&
    props.product.product_type === 'card' &&
    props.product.auto_deliver &&
    props.product.stock_count <= 0,
)
</script>

<template>
  <RouterLink
    :to="`/products/${product.slug}`"
    class="product-card group"
    :class="{ 'product-card-out': soldOut }"
  >
    <div class="product-media">
      <img
        v-if="product.image_path && !imageFailed"
        :src="product.image_path"
        :alt="product.name"
        class="product-media-img"
        loading="lazy"
        @error="imageFailed = true"
      />
      <div v-else class="product-media-fallback" aria-hidden="true">
        <span>{{ product.name.slice(0, 2).toUpperCase() }}</span>
      </div>

      <div class="pointer-events-none absolute left-3 top-3 flex flex-col items-start gap-1.5">
        <span v-if="soldOut" class="badge badge-neutral">暂时缺货</span>
        <span v-else-if="onSale" class="badge badge-danger">省 {{ offPercent }}%</span>
        <span v-if="product.is_featured" class="badge badge-accent">推荐</span>
      </div>

      <span class="product-delivery">
        {{ isTopup ? '额度充值' : product.product_type === 'card' ? '自动发货' : '人工交付' }}
      </span>
    </div>

    <div class="product-body">
      <div class="product-heading">
        <div class="min-w-0">
          <p class="product-kicker">{{ product.category?.name || '数字商品' }}</p>
          <h3 class="product-name">{{ product.name }}</h3>
        </div>
        <span v-if="product.is_featured" class="product-bookmark" aria-label="店长推荐">推荐</span>
      </div>

      <p class="product-summary">{{ product.summary || product.description || '暂无简介' }}</p>

      <div class="product-facts">
        <span v-if="site.showSoldCount" class="product-fact">
          已售 <strong class="nums">{{ product.sold_count ?? 0 }}</strong>
        </span>
        <span v-if="product.stock_visible && product.stock_count > 0" class="product-fact">
          现货 <strong class="nums">{{ product.stock_count }}</strong>
        </span>
        <span v-else-if="product.stock_visible && product.product_type === 'card'" class="product-fact product-fact-warn">
          库存紧张
        </span>
        <span v-if="product.activity_name" class="product-fact product-fact-accent">{{ product.activity_name }}</span>
      </div>

      <div v-if="isTopup" class="product-price product-price-topup">
        <div class="min-w-0">
          <span class="product-price-label">单次充值额度</span>
          <span class="product-topup-range nums">
            {{ topupRange || '以商品页为准' }}
          </span>
        </div>
        <span class="badge-accent">额度充值</span>
      </div>
      <div v-else class="product-price">
        <div class="min-w-0">
          <span class="product-price-label">{{ onSale ? '活动价' : '售价' }}</span>
          <span class="product-price-pay">{{ money(pay) }}</span>
        </div>
        <div v-if="was > 0" class="product-price-was">
          <span class="nums line-through">{{ money(was) }}</span>
          <span class="product-price-save">省 {{ money(saving) }}</span>
        </div>
      </div>

      <div class="product-action">
        <span>{{ isTopup ? '立即充值' : soldOut ? '查看补货状态' : '查看商品' }}</span>
        <span aria-hidden="true">→</span>
      </div>
    </div>
  </RouterLink>
</template>
