<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getProduct } from '../api/products'
import { createPayment } from '../api/payment'
import { useAuthStore } from '../stores/auth'
import type { Product } from '../types'
const route = useRoute(), router = useRouter(), auth = useAuthStore()
const product = ref<Product | null>(null), orderNo = ref(''), description = ref(''), loading = ref(true), paying = ref(false), error = ref('')
const money = (value: number) => new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)
async function purchase() {
  if (!auth.isAuthenticated) { await router.push({ name: 'login', query: { redirect: route.fullPath } }); return }
  if (!orderNo.value.trim()) { error.value = '请输入订单号'; return }
  paying.value = true; error.value = ''
  try { const result = await createPayment({ order_no: orderNo.value.trim(), description: description.value || undefined }); window.location.href = result.payment_order.payment_url }
  catch { error.value = '创建支付订单失败，请稍后重试' } finally { paying.value = false }
}
onMounted(async () => { try { product.value = (await getProduct(String(route.params.slug))).data } catch { error.value = '商品信息加载失败' } finally { loading.value = false } })
</script>
<template><div class="mx-auto max-w-7xl px-4 py-12 sm:px-6"><div v-if="loading" class="py-28 text-center text-zinc-400">正在加载商品…</div><div v-else-if="product" class="grid gap-8 lg:grid-cols-[1.3fr_.7fr]"><section class="overflow-hidden rounded-3xl glass"><div class="aspect-video bg-gradient-to-br from-fuchsia-950 to-indigo-950"><img v-if="product.cover_image" :src="product.cover_image" :alt="product.name" class="h-full w-full object-cover" /></div><div class="p-7 sm:p-10"><span v-if="product.category" class="rounded-full bg-fuchsia-500/10 px-3 py-1 text-sm text-fuchsia-300">{{ product.category.name }}</span><h1 class="mt-4 text-3xl font-bold sm:text-4xl">{{ product.name }}</h1><p class="mt-5 whitespace-pre-line leading-8 text-zinc-400">{{ product.description }}</p></div></section><aside class="h-fit rounded-3xl p-7 glass lg:sticky lg:top-24"><p class="text-sm text-zinc-400">商品价格</p><p class="mt-2 text-4xl font-black text-fuchsia-300">{{ money(product.price) }}</p><p v-if="product.original_price" class="mt-1 text-zinc-600 line-through">{{ money(product.original_price) }}</p><div class="my-6 h-px bg-white/10"></div><form class="space-y-4" @submit.prevent="purchase"><label class="block text-sm">订单号<input v-model="orderNo" class="field mt-2" required placeholder="请输入待支付订单号" /></label><label class="block text-sm">备注（选填）<textarea v-model="description" class="field mt-2 min-h-24 resize-none" placeholder="补充订单说明"></textarea></label><p v-if="error" class="text-sm text-red-300">{{ error }}</p><button class="btn-primary w-full" :disabled="paying || product.stock === 0">{{ product.stock === 0 ? '暂时缺货' : paying ? '正在创建支付…' : '立即购买' }}</button></form><p class="mt-4 text-center text-xs text-zinc-500">安全支付 · 自动发货 · 订单可追踪</p></aside></div><p v-else class="rounded-2xl p-8 text-center text-red-300 glass">{{ error || '未找到该商品' }}</p></div></template>
