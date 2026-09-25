<script setup lang="ts">
import type { Product } from '../types'

defineProps<{ product: Product }>()

const money = (value: number) =>
  new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)
</script>

<template>
  <RouterLink
    :to="`/products/${product.slug}`"
    class="sheen group flex flex-col overflow-hidden rounded-[22px] border border-white/[0.12] bg-gradient-to-b from-white/[0.09] to-white/[0.03] shadow-[inset_0_1px_0_rgba(255,255,255,0.14),0_10px_36px_-8px_rgba(0,0,0,0.45)] backdrop-blur-2xl transition-all duration-500 [transition-timing-function:cubic-bezier(0.34,1.56,0.64,1)] hover:-translate-y-1.5 hover:border-purple-400/40 hover:shadow-[inset_0_1px_0_rgba(255,255,255,0.2),0_24px_54px_-12px_rgba(0,0,0,0.55),0_0_44px_-10px_rgba(168,85,247,0.4)]"
  >
    <div class="relative aspect-[16/10] overflow-hidden bg-gradient-to-br from-purple-500/15 via-indigo-500/10 to-sky-500/15">
      <img
        v-if="product.cover_image"
        :src="product.cover_image"
        :alt="product.name"
        class="h-full w-full object-cover transition-transform duration-700 group-hover:scale-[1.07]"
      />
      <div v-else class="grid h-full place-items-center">
        <span class="grid size-16 place-items-center rounded-full border border-white/15 bg-white/[0.06] text-3xl font-black text-white/25 backdrop-blur-md">N</span>
      </div>
      <div class="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-[#0a0916]/70 to-transparent" />
    </div>

    <div class="flex flex-1 flex-col p-5">
      <div class="mb-2 flex items-start justify-between gap-3">
        <h3 class="truncate text-sm font-semibold text-white">{{ product.name }}</h3>
        <span
          v-if="product.category"
          class="shrink-0 rounded-full border border-purple-400/25 bg-purple-500/12 px-2.5 py-0.5 text-[11px] font-medium text-purple-200 backdrop-blur-sm"
        >{{ product.category.name }}</span>
      </div>

      <p class="line-clamp-2 min-h-[2.5rem] text-[13px] leading-relaxed text-[#b4b2c3]">{{ product.description }}</p>

      <div class="mt-auto flex items-end justify-between pt-4">
        <div>
          <span class="text-lg font-bold gradient-text">{{ money(product.price) }}</span>
          <span v-if="product.original_price" class="ml-2 text-xs text-[#7b7990] line-through">{{ money(product.original_price) }}</span>
        </div>
        <span class="rounded-full border border-white/10 bg-white/[0.05] px-3 py-1 text-xs text-[#b4b2c3] transition-all duration-300 group-hover:border-purple-400/40 group-hover:bg-purple-500/15 group-hover:text-white">
          查看 →
        </span>
      </div>
    </div>
  </RouterLink>
</template>
