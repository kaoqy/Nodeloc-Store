<script setup lang="ts">
import { onMounted, ref } from 'vue'
import NavBar from './components/NavBar.vue'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()
const uninitialized = ref(false)

onMounted(async () => {
  try {
    const res = await fetch('/api/v1/system/status', { headers: { Accept: 'application/json' } })
    if (res.ok) {
      const status = await res.json()
      uninitialized.value = status.initialized === false
    }
  } catch {
    // 状态接口不可用时按已初始化处理
  }
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
    <main class="flex flex-1 items-center justify-center px-4">
      <div v-if="uninitialized" class="card rise-in w-full max-w-md p-8 text-center">
        <div class="mx-auto mb-6 grid size-16 place-items-center rounded-[20px] bg-gradient-to-br from-purple-400 to-fuchsia-600 text-3xl">🛠️</div>
        <h1 class="text-xl font-bold">商店尚未初始化</h1>
        <p class="mt-3 text-sm text-[#b3b1c4]">
          管理员需要先到
          <a class="text-[#a855f7] underline underline-offset-4" href="/admin/">后台初始化向导</a>
          完成配置，初始化后这里将展示商品与下单入口。
        </p>
      </div>
      <div v-else class="w-full flex-1">
        <router-view v-slot="{ Component }">
          <transition name="page" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
      </div>
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
