<script setup lang="ts">
import type { Product } from '../types'
defineProps<{ product: Product }>()
const money = (value: number) => new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)
</script>
<template>
  <RouterLink :to="`/products/${product.slug}`" class="group overflow-hidden rounded-2xl glass transition hover:-translate-y-1 hover:border-fuchsia-400/40">
    <div class="aspect-[16/10] overflow-hidden bg-gradient-to-br from-fuchsia-950 to-indigo-950">
      <img v-if="product.cover_image" :src="product.cover_image" :alt="product.name" class="h-full w-full object-cover transition duration-500 group-hover:scale-105" />
      <div v-else class="grid h-full place-items-center text-5xl font-black text-white/15">N</div>
    </div>
    <div class="p-5"><div class="mb-2 flex items-start justify-between gap-3"><h3 class="font-semibold text-white">{{ product.name }}</h3><span v-if="product.category" class="shrink-0 rounded-full bg-fuchsia-500/10 px-2 py-1 text-xs text-fuchsia-300">{{ product.category.name }}</span></div><p class="line-clamp-2 min-h-10 text-sm text-zinc-400">{{ product.description }}</p><div class="mt-5 flex items-end justify-between"><div><span class="text-xl font-bold text-fuchsia-300">{{ money(product.price) }}</span><span v-if="product.original_price" class="ml-2 text-xs text-zinc-600 line-through">{{ money(product.original_price) }}</span></div><span class="text-sm text-zinc-400 group-hover:text-white">查看详情 →</span></div></div>
  </RouterLink>
</template>
