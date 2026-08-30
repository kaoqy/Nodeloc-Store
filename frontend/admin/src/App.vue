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
  <div v-if="route.path === '/login'" class="min-h-screen">
    <RouterView />
  </div>

  <div v-else class="min-h-screen bg-[#0a0a0f]">
    <SideBar :open="open" @close="open = false" />

    <div class="lg:pl-60">
      <!-- Header -->
      <header class="sticky top-0 z-20 flex h-16 items-center justify-between border-b border-white/[0.08] bg-[#0a0a0f]/80 px-6 backdrop-blur-xl">
        <div class="flex items-center gap-4">
          <button class="btn-secondary lg:hidden" @click="open = true">☰</button>

          <!-- Breadcrumb -->
          <nav class="flex items-center gap-2 text-sm">
            <span class="text-[#6b6b80]">管理后台</span>
            <template v-for="(crumb, i) in breadcrumb" :key="i">
              <span class="text-[#6b6b80]">/</span>
              <RouterLink
                :to="crumb.path"
                :class="[
                  i === breadcrumb.length - 1 ? 'text-white font-medium' : 'text-[#a1a1b5] hover:text-white',
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
              placeholder="搜索..."
              class="input w-52 pl-9"
            />
            <span class="absolute left-3 top-1/2 -translate-y-1/2 text-[#6b6b80]">🔍</span>
          </div>

          <!-- Notifications -->
          <relative class="relative">
            <button class="btn-secondary relative">
              🔔
              <span class="absolute -right-1 -top-1 flex h-4 w-4 items-center justify-center rounded-full bg-red-500 text-[10px] font-bold text-white">3</span>
            </button>
          </relative>
        </div>
      </header>

      <!-- Main content -->
      <main class="p-6">
        <div class="mb-6 flex items-center justify-between">
          <div>
            <h1 class="text-2xl font-bold tracking-tight">{{ pageTitle }}</h1>
            <p class="mt-1 text-sm text-[#6b6b80]">管理和监控您的数字商品平台</p>
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
