<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getProduct } from '../api/products'
import { createPayment } from '../api/payment'
import { useAuthStore } from '../stores/auth'
import type { Product } from '../types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const product = ref<Product | null>(null)
const orderNo = ref('')
const description = ref('')
const loading = ref(true)
const paying = ref(false)
const error = ref('')

const money = (value: number) => new Intl.NumberFormat('zh-CN', { style: 'currency', currency: 'CNY' }).format(value)

async function purchase() {
  if (!auth.isAuthenticated) { await router.push({ name: 'login', query: { redirect: route.fullPath } }); return }
  if (!orderNo.value.trim()) { error.value = '请输入订单号'; return }
  paying.value = true
  error.value = ''
  try {
    const result = await createPayment({ order_no: orderNo.value.trim(), description: description.value || undefined })
    window.location.href = result.payment_order.payment_url
  } catch {
    error.value = '创建支付订单失败，请稍后重试'
  } finally {
    paying.value = false
  }
}

onMounted(async () => {
  try {
    product.value = (await getProduct(String(route.params.slug))).data
  } catch {
    error.value = '商品信息加载失败'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="mx-auto max-w-7xl px-4 py-10 sm:px-6">
    <!-- Loading -->
    <div v-if="loading" class="py-28 text-center text-[#b4b2c3]">
      <div class="skeleton mx-auto mb-4 h-8 w-48 !rounded-full" />
      <div class="skeleton mx-auto h-64 max-w-3xl" />
    </div>

    <!-- Content -->
    <div v-else-if="product" class="fade-in grid gap-8 lg:grid-cols-[1.4fr_0.6fr]">
      <!-- Product Info -->
      <section class="sheen self-start overflow-hidden rounded-[26px] border border-white/[0.13] bg-gradient-to-b from-white/[0.08] to-white/[0.025] shadow-[inset_0_1px_0_rgba(255,255,255,0.15),0_20px_60px_-14px_rgba(0,0,0,0.55)] backdrop-blur-2xl">
        <div class="relative aspect-video bg-gradient-to-br from-purple-500/15 via-indigo-500/8 to-sky-500/15">
          <img v-if="product.cover_image" :src="product.cover_image" :alt="product.name" class="h-full w-full object-cover" />
          <div v-else class="grid h-full place-items-center text-7xl font-black text-white/[0.06]">N</div>
          <div class="pointer-events-none absolute inset-x-0 bottom-0 h-24 bg-gradient-to-t from-[#0a0916]/75 to-transparent" />
        </div>
        <div class="p-7 sm:p-10">
          <span v-if="product.category" class="badge badge-accent mb-4">{{ product.category.name }}</span>
          <h1 class="text-3xl font-black tracking-tight">{{ product.name }}</h1>
          <div class="mt-4 flex items-baseline gap-3">
            <span class="text-4xl font-black gradient-text">{{ money(product.price) }}</span>
            <span v-if="product.original_price" class="text-lg text-[#7b7990] line-through">{{ money(product.original_price) }}</span>
          </div>
          <div class="mt-7 h-px bg-gradient-to-r from-transparent via-white/15 to-transparent" />
          <p class="mt-7 whitespace-pre-line leading-8 text-[#b4b2c3]">{{ product.description }}</p>
        </div>
      </section>

      <!-- Purchase Card -->
      <aside class="glass h-fit p-7 lg:sticky lg:top-28">
        <h3 class="text-lg font-bold tracking-tight">立即购买</h3>
        <div class="mt-4 rounded-2xl border border-purple-400/25 bg-gradient-to-br from-purple-500/15 to-indigo-500/10 p-5 text-center shadow-[inset_0_1px_0_rgba(255,255,255,0.12)]">
          <p class="text-xs tracking-widest text-[#b4b2c3] uppercase">商品价格</p>
          <p class="mt-1 text-3xl font-black gradient-text">{{ money(product.price) }}</p>
        </div>
        <form class="mt-6 space-y-4" @submit.prevent="purchase">
          <div>
            <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">订单号</label>
            <input v-model="orderNo" class="input" required placeholder="请输入待支付订单号" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">备注（选填）</label>
            <textarea v-model="description" class="input min-h-[88px] resize-none" placeholder="补充订单说明"></textarea>
          </div>
          <p v-if="error" class="fade-in rounded-2xl border border-rose-400/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{{ error }}</p>
          <button class="btn btn-primary w-full py-3.5" :disabled="paying || product.stock === 0">
            {{ product.stock === 0 ? '暂时缺货' : paying ? '正在创建支付…' : '立即购买' }}
          </button>
        </form>
        <div class="mt-5 flex items-center justify-center gap-4 text-[11px] text-[#7b7990]">
          <span class="flex items-center gap-1.5"><span class="size-1 rounded-full bg-emerald-400" />安全支付</span>
          <span class="flex items-center gap-1.5"><span class="size-1 rounded-full bg-purple-400" />自动发货</span>
          <span class="flex items-center gap-1.5"><span class="size-1 rounded-full bg-sky-400" />订单可追踪</span>
        </div>
      </aside>
    </div>

    <!-- Error -->
    <p v-else class="glass p-10 text-center text-rose-300">{{ error || '未找到该商品' }}</p>
  </div>
</template>
