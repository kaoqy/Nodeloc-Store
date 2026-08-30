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
    <div v-if="loading" class="py-28 text-center text-[#a1a1aa]">
      <div class="skeleton mx-auto mb-4 h-8 w-48 rounded-lg" />
      <div class="skeleton mx-auto h-64 max-w-3xl rounded-2xl" />
    </div>

    <!-- Content -->
    <div v-else-if="product" class="grid gap-8 lg:grid-cols-[1.4fr_0.6fr]">
      <!-- Product Info -->
      <section class="overflow-hidden rounded-2xl border border-white/[0.08] bg-[#111114]">
        <div class="aspect-video bg-gradient-to-br from-purple-950/40 to-indigo-950/40">
          <img v-if="product.cover_image" :src="product.cover_image" :alt="product.name" class="h-full w-full object-cover" />
        </div>
        <div class="p-7 sm:p-9">
          <span v-if="product.category" class="badge-accent mb-3">{{ product.category.name }}</span>
          <h1 class="text-3xl font-bold tracking-tight">{{ product.name }}</h1>
          <div class="mt-4 flex items-baseline gap-3">
            <span class="text-3xl font-black gradient-text">{{ money(product.price) }}</span>
            <span v-if="product.original_price" class="text-lg text-[#71717a] line-through">{{ money(product.original_price) }}</span>
          </div>
          <div class="mt-6 h-px bg-white/[0.08]" />
          <p class="mt-6 whitespace-pre-line leading-8 text-[#a1a1aa]">{{ product.description }}</p>
        </div>
      </section>

      <!-- Purchase Card -->
      <aside class="h-fit rounded-2xl border border-white/[0.08] bg-[#111114] p-7 lg:sticky lg:top-24">
        <h3 class="text-lg font-bold">立即购买</h3>
        <div class="mt-4 rounded-xl bg-gradient-to-br from-purple-500/10 to-indigo-500/10 p-4">
          <p class="text-sm text-[#a1a1aa]">商品价格</p>
          <p class="mt-1 text-3xl font-black gradient-text">{{ money(product.price) }}</p>
        </div>
        <form class="mt-5 space-y-4" @submit.prevent="purchase">
          <div>
            <label class="mb-1.5 block text-sm font-medium">订单号</label>
            <input v-model="orderNo" class="input" required placeholder="请输入待支付订单号" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium">备注（选填）</label>
            <textarea v-model="description" class="input min-h-[80px] resize-none" placeholder="补充订单说明"></textarea>
          </div>
          <p v-if="error" class="rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-300">{{ error }}</p>
          <button class="btn-primary w-full py-3" :disabled="paying || product.stock === 0">
            {{ product.stock === 0 ? '暂时缺货' : paying ? '正在创建支付…' : '立即购买' }}
          </button>
        </form>
        <p class="mt-4 text-center text-xs text-[#71717a]">安全支付 · 自动发货 · 订单可追踪</p>
      </aside>
    </div>

    <!-- Error -->
    <p v-else class="rounded-2xl border border-red-500/20 bg-red-500/10 p-8 text-center text-red-300">{{ error || '未找到该商品' }}</p>
  </div>
</template>
