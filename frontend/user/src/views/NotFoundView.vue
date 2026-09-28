<script setup lang="ts">
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const route = useRoute()

// The address is echoed back because a dead link is usually a typo the visitor
// can see and fix, and a shop has no way to know which one they meant.
const attempted = route.fullPath
</script>

<template>
  <div class="mx-auto w-full max-w-4xl px-4 py-16 sm:px-6">
    <div class="card rise-in mx-auto max-w-lg text-center !py-12">
      <p class="eyebrow">404</p>
      <h1 class="mt-3 text-xl font-bold">这个地址在店里没有对应的页面</h1>
      <p class="mt-3 text-sm leading-relaxed text-[var(--text-dim)]">
        链接可能来自旧的页面，或者这件商品已经下架、改名。货架上的商品随时可以重新挑。
      </p>
      <p class="mono mt-4 break-all text-xs text-[var(--text-quiet)]">{{ attempted }}</p>

      <div class="mt-8 flex flex-wrap justify-center gap-2">
        <RouterLink to="/" class="btn btn-primary btn-sm">回到店面</RouterLink>
        <RouterLink v-if="auth.isAuthenticated" to="/orders" class="btn btn-quiet btn-sm">我的订单</RouterLink>
        <RouterLink v-if="auth.isAuthenticated" to="/profile" class="btn btn-quiet btn-sm">个人中心</RouterLink>
        <RouterLink v-if="!auth.isAuthenticated" to="/login" class="btn btn-quiet btn-sm">登录</RouterLink>
      </div>
    </div>
  </div>
</template>
