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
  <div class="pt-4">
    <header class="nav-shell">
      <nav class="flex items-center justify-between py-2.5 pl-3.5 pr-3">
        <RouterLink to="/" class="group flex items-center gap-2.5 pl-1 font-bold">
          <span class="grid size-9 place-items-center rounded-full border border-white/25 bg-gradient-to-br from-purple-400 via-purple-600 to-indigo-600 text-sm font-black text-white shadow-[inset_0_1px_0_rgba(255,255,255,0.45),0_6px_20px_-4px_rgba(168,85,247,0.65)] transition-transform duration-300 group-hover:scale-105 group-hover:rotate-3">N</span>
          <span class="hidden text-[15px] tracking-tight sm:block">Nodeloc <span class="gradient-text">Store</span></span>
        </RouterLink>

        <button class="nav-link-pill lg:hidden" aria-label="菜单" @click="open = !open">☰</button>

        <div
          :class="[
            open ? 'flex' : 'hidden',
            'absolute inset-x-2 top-[68px] z-50 flex-col gap-1 rounded-3xl border border-white/[0.14] bg-[#0d0b1a]/85 p-4 shadow-2xl backdrop-blur-2xl lg:static lg:flex lg:flex-row lg:items-center lg:gap-1 lg:rounded-none lg:border-0 lg:bg-transparent lg:p-0 lg:shadow-none lg:backdrop-blur-none',
          ]"
        >
          <RouterLink to="/" class="nav-link-pill" @click="open = false">商品</RouterLink>
          <template v-if="auth.isAuthenticated">
            <RouterLink to="/orders" class="nav-link-pill" @click="open = false">我的订单</RouterLink>
            <RouterLink to="/profile" class="nav-link-pill max-w-40 truncate" @click="open = false">{{ auth.user?.username || '个人中心' }}</RouterLink>
            <button class="nav-link-pill text-left" @click="logout">退出</button>
          </template>
          <template v-else>
            <RouterLink to="/login" class="nav-link-pill" @click="open = false">登录</RouterLink>
            <RouterLink to="/register" class="btn btn-primary ml-1 !py-2 text-sm" @click="open = false">注册</RouterLink>
          </template>
        </div>
      </nav>
    </header>
  </div>
</template>
