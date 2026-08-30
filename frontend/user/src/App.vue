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
  <div class="flex min-h-screen flex-col bg-[#09090b] text-zinc-100">
    <NavBar />
    <main class="flex-1">
      <router-view />
    </main>
    <footer class="border-t border-white/[0.08] py-6 text-center text-xs text-[#71717a]">
      <p>Nodeloc Store · 数字商品交易平台</p>
    </footer>
  </div>
</template>
