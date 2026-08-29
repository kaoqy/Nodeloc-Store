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
  <header class="sticky top-0 z-50 border-b border-white/10 bg-zinc-950/75 backdrop-blur-xl">
    <nav class="mx-auto flex max-w-7xl items-center justify-between px-4 py-4 sm:px-6">
      <RouterLink to="/" class="flex items-center gap-2 font-bold"><span class="grid size-9 place-items-center rounded-xl bg-gradient-to-br from-fuchsia-500 to-indigo-600">N</span><span>Nodeloc Store</span></RouterLink>
      <button class="rounded-lg border border-white/10 p-2 md:hidden" aria-label="打开导�航" @click="open = !open">☰</button>
      <div :class="[open ? 'flex' : 'hidden', 'absolute inset-x-4 top-20 flex-col gap-3 rounded-2xl p-4 glass md:static md:flex md:flex-row md:items-center md:border-0 md:bg-transparent md:p-0 md:backdrop-blur-none']">
        <RouterLink to="/" class="rounded-lg px-3 py-2 text-zinc-300 hover:bg-white/5 hover:text-white" @click="open=false">商品</RouterLink>
        <template v-if="auth.isAuthenticated">
          <RouterLink to="/orders" class="rounded-lg px-3 py-2 text-zinc-300 hover:bg-white/5 hover:text-white" @click="open=false">我的订单</RouterLink>
          <RouterLink to="/profile" class="rounded-lg px-3 py-2 text-zinc-300 hover:bg-white/5 hover:text-white" @click="open=false">{{ auth.user?.username || '个人中心' }}</RouterLink>
          <button class="rounded-lg px-3 py-2 text-left text-zinc-400 hover:text-white" @click="logout">退出</button>
        </template>
        <template v-else><RouterLink to="/login" class="rounded-lg px-3 py-2 text-zinc-300" @click="open=false">登录</RouterLink><RouterLink to="/register" class="btn-primary text-center" @click="open=false">注册</RouterLink></template>
      </div>
    </nav>
  </header>
</template>
