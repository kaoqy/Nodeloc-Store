<script setup lang="ts">
import { onMounted } from 'vue'
import NavBar from './components/NavBar.vue'
import RouteProgress from './components/RouteProgress.vue'
import { useAuthStore } from './stores/auth'
import { useSiteStore } from './stores/site'

const auth = useAuthStore()
const site = useSiteStore()

onMounted(async () => {
  await site.load()
  if (auth.token && !auth.user) {
    await auth.fetchUser().catch(() => undefined)
  }
})
</script>

<template>
  <div class="flex min-h-screen flex-col">
    <RouteProgress />
    <NavBar />

    <main class="flex flex-1 flex-col">
      <div v-if="!site.initialized" class="mx-auto w-full max-w-lg px-4 py-24">
        <div class="card rise-in text-center">
          <div class="brand-mark mx-auto mb-5">!</div>
          <h1 class="text-lg font-bold">商店尚未初始化</h1>
          <p class="mt-3 text-sm text-[var(--text-dim)]">
            管理员需要先打开
            <a href="/admin/" class="accent-text underline underline-offset-4">后台初始化向导</a>
            完成站点、数据库与 NodeLoc 集成配置。
          </p>
        </div>
      </div>

      <div v-else class="flex-1">
        <RouterView v-slot="{ Component }">
          <Transition name="page" mode="out-in">
            <component :is="Component" />
          </Transition>
        </RouterView>
      </div>
    </main>

    <footer class="mt-20 border-t border-[var(--stroke)] py-8">
      <div class="mx-auto flex w-full max-w-6xl flex-col items-center gap-1.5 px-4 text-center sm:px-6">
        <p class="text-[13px] text-[var(--text-quiet)]">
          {{ site.name }}
          <template v-if="site.slogan"> · {{ site.slogan }}</template>
        </p>
        <p class="text-xs text-[var(--text-quiet)]/80">
          NodeLoc OAuth2 登录 · Nodeloc Payments 支付 · 卡密自动交付
          <span v-if="site.version" class="nums"> · v{{ site.version }}</span>
        </p>
      </div>
    </footer>
  </div>
</template>

<style scoped>
.page-enter-active {
  transition: opacity 240ms var(--ease), transform 240ms var(--spring);
}
.page-leave-active { transition: opacity 120ms var(--ease); }
.page-enter-from { opacity: 0; transform: translateY(10px); }
.page-leave-to { opacity: 0; }
</style>
