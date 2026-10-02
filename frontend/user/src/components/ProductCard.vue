<script setup lang="ts">
import { ref } from 'vue'
import type { Product } from '../types'
import { money } from '../utils/format'
import { useSiteStore } from '../stores/site'

defineProps<{ product: Product }>()
// 店家可以在配置中心关掉前台销量展示，卡片要跟着走。
const site = useSiteStore()
const imageFailed = ref(false)
</script>

<template>
  <RouterLink
    :to="`/products/${product.slug}`"
    class="card-hover group flex flex-col overflow-hidden !p-0"
  >
    <div class="relative aspect-[16/9] overflow-hidden border-b border-[var(--stroke-quiet)] bg-[var(--surface-sunken)]">
      <img
        v-if="product.image_path && !imageFailed"
        :src="product.image_path"
        :alt="product.name"
        class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.03]"
        @error="imageFailed = true"
      />
      <div v-if="!product.image_path" class="grid h-full place-items-center">
        <span class="mono text-2xl font-bold tracking-[0.2em] text-[var(--text-quiet)]/50">
          {{ product.name.slice(0, 2).toUpperCase() }}
        </span>
      </div>
      <span
        v-if="product.stock_visible && product.product_type === 'card' && product.auto_deliver && product.stock_count <= 0"
        class="badge badge-neutral absolute left-3 top-3"
      >暂时缺货</span>
      <span v-if="product.is_featured" class="badge badge-accent absolute right-3 top-3">推荐</span>
    </div>

    <div class="flex flex-1 flex-col p-5">
      <div class="flex items-start justify-between gap-3">
        <h3 class="line-clamp-2 min-w-0 break-words text-[15px] font-semibold">{{ product.name }}</h3>
        <span v-if="product.category" class="badge badge-neutral shrink-0">{{ product.category.name }}</span>
      </div>

      <p class="mt-2 line-clamp-2 min-h-[2.6em] text-[13px] leading-relaxed text-[var(--text-dim)]">
        {{ product.summary || product.description || '暂无简介' }}
      </p>

      <div class="mt-auto flex items-end justify-between gap-3 pt-4">
        <div class="flex items-baseline gap-2">
          <span class="nums text-lg font-bold accent-text">{{ money(product.price) }}</span>
          <span v-if="product.original_price && product.original_price > product.price" class="nums text-xs text-[var(--text-quiet)] line-through">
            {{ money(product.original_price) }}
          </span>
        </div>
        <span class="badge" :class="product.product_type === 'card' ? 'badge-teal' : 'badge-accent'">
          {{ product.product_type === 'card' ? '自动发货' : '人工交付' }}
        </span>
      </div>

      <!-- 销量 is delivered volume from the server, so it counts goods that
           actually left the shop rather than orders that were abandoned. -->
      <p class="hint mt-2 flex flex-wrap items-center gap-x-3 gap-y-1">
        <span v-if="site.showSoldCount">
          已售 <span class="nums">{{ product.sold_count ?? 0 }}</span> 件
        </span>
        <span v-if="product.stock_visible && product.stock_count > 0">
          现货 <span class="nums">{{ product.stock_count }}</span> 件
        </span>
        <span v-else-if="product.stock_visible && product.product_type === 'card'" class="text-[var(--warning)]">库存紧张或暂时缺货</span>
      </p>
    </div>
  </RouterLink>
</template>
