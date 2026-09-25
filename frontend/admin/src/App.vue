<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import SideBar from './components/SideBar.vue'

const route = useRoute()
const open = ref(false)

const breadcrumb = computed(() => {
  const segments = route.path.split('/').filter(Boolean)
  return segments.map((seg, i) => ({
    label: seg.charAt(0).toUpperCase() + seg.slice(1),
    path: '/' + segments.slice(0, i + 1).join('/'),
  }))
})

const pageTitle = computed(() => {
  const titles: Record<string, string> = {
    '/': '仪表盘',
    '/products': '商品管理',
    '/orders': '订单管理',
    '/cards': '卡密管理',
    '/categories': '分类管理',
    '/coupons': '优惠券',
    '/users': '用户管理',
    '/notifications': '通知中心',
    '/logs': '审计日志',
    '/settings': '系统设置',
  }
  return titles[route.path] || 'Nodeloc Store'
})
</script>

<template>
  <div v-if="route.path === '/login' || route.path === '/setup'" class="min-h-screen">
    <RouterView />
  </div>

  <div v-else class="min-h-screen">
    <SideBar :open="open" @close="open = false" />

    <div class="lg:pl-64">
      <!-- Header -->
      <header class="sticky top-0 z-20 flex h-[72px] items-center justify-between px-5 sm:px-7">
        <div class="absolute inset-3 left-4 right-4 -z-10 rounded-full border border-white/[0.12] bg-gradient-to-b from-white/[0.08] to-white/[0.03] shadow-[inset_0_1px_0_rgba(255,255,255,0.14),0_14px_40px_-16px_rgba(0,0,0,0.5)] backdrop-blur-2xl" />
        <div class="flex items-center gap-4">
          <button class="btn btn-secondary !px-3 lg:hidden" @click="open = true">☰</button>

          <!-- Breadcrumb -->
          <nav class="flex items-center gap-2 text-sm">
            <span class="hidden text-[#7a7890] sm:inline">管理后台</span>
            <template v-for="(crumb, i) in breadcrumb" :key="i">
              <span class="hidden text-[#7a7890] sm:inline">/</span>
              <RouterLink
                :to="crumb.path"
                :class="[
                  i === breadcrumb.length - 1 ? 'font-semibold text-white' : 'text-[#b3b1c4] transition hover:text-white',
                ]"
              >
                {{ crumb.label }}
              </RouterLink>
            </template>
          </nav>
        </div>

        <div class="flex items-center gap-3">
          <!-- Search -->
          <div class="relative hidden md:block">
            <input
              type="text"
              placeholder="搜索…"
              class="input w-52 pl-9"
            />
            <span class="absolute left-3.5 top-1/2 -translate-y-1/2 text-[#7a7890]">🔍</span>
          </div>

          <!-- Notifications -->
          <div class="relative">
            <button class="btn btn-secondary !px-3.5">
              🔔
              <span class="absolute -right-1 -top-1 grid size-4.5 place-items-center rounded-full border border-white/20 bg-gradient-to-br from-rose-400 to-rose-600 text-[10px] font-bold text-white shadow-[0_0_12px_rgba(251,113,133,0.7)]">3</span>
            </button>
          </div>
        </div>
      </header>

      <!-- Main content -->
      <main class="px-5 pb-10 sm:px-7">
        <div class="mb-7 flex items-center justify-between">
          <div>
            <h1 class="text-2xl font-black tracking-tight">{{ pageTitle }}</h1>
            <p class="mt-1 text-sm text-[#7a7890]">管理和监控您的数字商品平台</p>
          </div>
          <div class="flex items-center gap-2">
            <slot name="actions" />
          </div>
        </div>

        <div class="fade-in">
          <RouterView />
        </div>
      </main>
    </div>
  </div>
</template>
