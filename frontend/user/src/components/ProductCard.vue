<script setup lang="ts">
import type { Product } from '../types'

defineProps<{ product: Product }>()

const money = (value: number) =>
  new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)
</script>

<template>
  <RouterLink
    :to="`/products/${product.slug}`"
    class="group overflow-hidden rounded-2xl border border-white/[0.08] bg-[#111114] transition-all duration-300 hover:-translate-y-1 hover:border-purple-400/30 hover:shadow-xl hover:shadow-purple-500/5"
  >
    <div class="aspect-[16/10] overflow-hidden bg-gradient-to-br from-purple-950/50 to-indigo-950/50">
      <img
        v-if="product.cover_image"
        :src="product.cover_image"
        :alt="product.name"
        class="h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
      />
      <div v-else class="grid h-full place-items-center text-5xl font-black text-white/[0.07]">N</div>
    </div>

    <div class="p-5">
      <div class="mb-2 flex items-start justify-between gap-3">
        <h3 class="font-semibold text-sm text-white truncate">{{ product.name }}</h3>
        <span
          v-if="product.category"
          class="shrink-0 rounded-full bg-purple-500/10 px-2 py-0.5 text-[11px] text-purple-300"
        >{{ product.category.name }}</span>
      </div>

      <p class="line-clamp-2 min-h-[2.5rem] text-[13px] text-[#a1a1aa] leading-relaxed">{{ product.description }}</p>

      <div class="mt-4 flex items-end justify-between">
        <div>
          <span class="text-lg font-bold gradient-text">{{ money(product.price) }}</span>
          <span v-if="product.original_price" class="ml-2 text-xs text-[#71717a] line-through">{{ money(product.original_price) }}</span>
        </div>
        <span class="text-xs text-[#71717a] transition group-hover:text-white">查看 →</span>
      </div>
    </div>
  </RouterLink>
</template>
