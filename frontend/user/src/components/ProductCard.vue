<script setup lang="ts">
import type { Product } from '../types'
import { money } from '../utils/format'

defineProps<{ product: Product }>()
</script>

<template>
  <RouterLink
    :to="`/products/${product.slug}`"
    class="card-hover group flex flex-col overflow-hidden !p-0"
  >
    <div class="relative aspect-[16/9] overflow-hidden border-b border-[var(--stroke-quiet)] bg-[var(--surface-sunken)]">
      <img
        v-if="product.image_path"
        :src="product.image_path"
        :alt="product.name"
        class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-[1.03]"
      />
      <div v-else class="grid h-full place-items-center">
        <span class="mono text-2xl font-bold tracking-[0.2em] text-[var(--text-quiet)]/50">
          {{ product.name.slice(0, 2).toUpperCase() }}
        </span>
      </div>
      <span
        v-if="product.stock_visible && product.stock_count <= 0"
        class="badge badge-neutral absolute left-3 top-3"
      >暂时缺货</span>
    </div>

    <div class="flex flex-1 flex-col p-5">
      <div class="flex items-start justify-between gap-3">
        <h3 class="text-[15px] font-semibold">{{ product.name }}</h3>
        <span v-if="product.category" class="badge badge-neutral shrink-0">{{ product.category.name }}</span>
      </div>

      <p class="mt-2 line-clamp-2 min-h-[2.6em] text-[13px] leading-relaxed text-[var(--text-dim)]">
        {{ product.summary || product.description || '暂无简介' }}
      </p>

      <div class="mt-auto flex items-end justify-between gap-3 pt-4">
        <div class="flex items-baseline gap-2">
          <span class="nums text-lg font-bold accent-text">{{ money(product.price) }}</span>
          <span v-if="product.original_price" class="nums text-xs text-[var(--text-quiet)] line-through">
            {{ money(product.original_price) }}
          </span>
        </div>
        <span class="badge" :class="product.product_type === 'card' ? 'badge-teal' : 'badge-accent'">
          {{ product.product_type === 'card' ? '自动发货' : '人工交付' }}
        </span>
      </div>

      <p v-if="product.stock_visible && product.stock_count > 0" class="hint mt-2">
        现货 <span class="nums">{{ product.stock_count }}</span> 件
      </p>
    </div>
  </RouterLink>
</template>
