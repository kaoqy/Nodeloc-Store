<script setup lang="ts">
import { onMounted } from 'vue'
import NavBar from './components/NavBar.vue'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()

onMounted(async () => {
  if (auth.token && !auth.user) {
    try {
      await auth.fetchUser()
    } catch {
      // 认证失败由拦截器和 auth store 统一处理
    }
  }
})
</script>

<template>
  <div class="relative flex min-h-screen flex-col text-zinc-100">
    <NavBar />
    <main class="flex-1">
      <router-view v-slot="{ Component }">
        <transition name="page" mode="out-in">
          <component :is="Component" />
        </transition>
      </router-view>
    </main>
    <footer class="mx-auto mt-16 w-full max-w-7xl px-4 pb-8 sm:px-6">
      <div class="glass flex flex-col items-center gap-2 rounded-full px-6 py-4 text-center">
        <p class="text-xs tracking-wide text-[#7b7990]">
          Nodeloc Store <span class="mx-2 text-white/20">·</span> 数字商品交易平台
          <span class="mx-2 text-white/20">·</span> Powered by NodeLoc OAuth2 &amp; Nodeloc Payments
        </p>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.page-enter-active { transition: opacity 0.35s ease, transform 0.35s cubic-bezier(0.34, 1.56, 0.64, 1); }
.page-leave-active { transition: opacity 0.18s ease; }
.page-enter-from { opacity: 0; transform: translateY(12px); }
.page-leave-to { opacity: 0; }
</style>
