<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const open = ref(false)

function logout() { auth.logout(); open.value = false; router.push('/') }
</script>

<template>
  <header class="sticky top-0 z-50 border-b border-white/[0.08] bg-[#09090b]/80 backdrop-blur-xl">
    <nav class="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 sm:px-6">
      <RouterLink to="/" class="flex items-center gap-2.5 font-bold">
        <span class="grid size-9 place-items-center rounded-xl bg-gradient-to-br from-purple-500 to-indigo-600 text-sm font-bold shadow-lg shadow-purple-500/20">N</span>
        <span class="text-[15px]">Nodeloc Store</span>
      </RouterLink>

      <button class="btn-secondary lg:hidden p-2" aria-label="菜单" @click="open = !open">☰</button>

      <div :class="[open ? 'flex' : 'hidden', 'absolute inset-x-4 top-20 flex-col gap-1 rounded-2xl p-4 bg-[#111114] border border-white/[0.08] lg:static lg:flex lg:flex-row lg:items-center lg:gap-1 lg:border-0 lg:bg-transparent lg:p-0']">
        <RouterLink to="/" class="rounded-lg px-3 py-2 text-sm text-[#a1a1aa] transition hover:bg-white/[0.06] hover:text-white" @click="open=false">商品</RouterLink>
        <template v-if="auth.isAuthenticated">
          <RouterLink to="/orders" class="rounded-lg px-3 py-2 text-sm text-[#a1a1aa] transition hover:bg-white/[0.06] hover:text-white" @click="open=false">我的订单</RouterLink>
          <RouterLink to="/profile" class="rounded-lg px-3 py-2 text-sm text-[#a1a1aa] transition hover:bg-white/[0.06] hover:text-white" @click="open=false">{{ auth.user?.username || '个人中心' }}</RouterLink>
          <button class="rounded-lg px-3 py-2 text-sm text-[#a1a1aa] text-left transition hover:bg-white/[0.06] hover:text-white" @click="logout">退出</button>
        </template>
        <template v-else>
          <RouterLink to="/login" class="rounded-lg px-3 py-2 text-sm text-[#a1a1aa] transition hover:bg-white/[0.06] hover:text-white" @click="open=false">登录</RouterLink>
          <RouterLink to="/register" class="btn-primary text-sm" @click="open=false">注册</RouterLink>
        </template>
      </div>
    </nav>
  </header>
</template>
