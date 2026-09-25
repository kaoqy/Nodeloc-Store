<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '../stores/auth'
import { useRoute } from 'vue-router'

defineProps<{ open: boolean }>()
defineEmits(['close'])

const auth = useAuthStore()
const route = useRoute()

const active = (path: string) => route.path.startsWith(path) && path !== '/' ? 'active' : route.path === path ? 'active' : ''

const groups = [
  {
    label: '概览',
    items: [
      { path: '/', label: '仪表盘', icon: '📊' },
    ],
  },
  {
    label: '运营',
    items: [
      { path: '/products', label: '商品管理', icon: '🛍️' },
      { path: '/orders', label: '订单管理', icon: '📦' },
      { path: '/cards', label: '卡密管理', icon: '🔑' },
      { path: '/categories', label: '分类管理', icon: '🏷️' },
      { path: '/coupons', label: '优惠券', icon: '🎫' },
    ],
  },
  {
    label: '用户',
    items: [
      { path: '/users', label: '用户管理', icon: '👥' },
    ],
  },
  {
    label: '系统',
    items: [
      { path: '/notifications', label: '通知中心', icon: '🔔' },
      { path: '/logs', label: '审计日志', icon: '📝' },
      { path: '/settings', label: '系统设置', icon: '⚙️' },
    ],
  },
]
</script>

<template>
  <!-- Mobile overlay -->
  <div v-if="open" class="fixed inset-0 z-30 bg-black/60 backdrop-blur-sm lg:hidden" @click="$emit('close')" />

  <!-- Sidebar -->
  <aside
    :class="[
      open ? 'translate-x-0' : '-translate-x-full',
      'fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-white/[0.12] bg-[#0b0a18]/70 backdrop-blur-3xl transition-transform duration-300 lg:translate-x-0',
    ]"
  >
    <!-- Logo -->
    <div class="flex h-[72px] items-center gap-3 border-b border-white/[0.08] px-5">
      <div
        class="flex h-10 w-10 items-center justify-center rounded-full border border-white/25 bg-gradient-to-br from-indigo-400 via-indigo-600 to-purple-600 text-sm font-black text-white shadow-[inset_0_1.5px_0_rgba(255,255,255,0.45),0_8px_24px_-6px_rgba(99,102,241,0.6)]"
      >
        NL
      </div>
      <div class="flex flex-col">
        <span class="text-sm font-semibold tracking-tight">Nodeloc <span class="gradient-text">Store</span></span>
        <span class="text-[11px] text-[#7a7890]">数字商品平台</span>
      </div>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 space-y-1 overflow-y-auto px-3 py-4">
      <div v-for="group in groups" :key="group.label" class="mb-5">
        <p class="mb-2 px-3 text-[11px] font-semibold uppercase tracking-[0.14em] text-[#7a7890]">
          {{ group.label }}
        </p>
        <div class="space-y-1">
          <RouterLink
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            :class="[
              active(item.path),
              'nav-link group flex items-center gap-3 rounded-full px-3.5 py-2.5 text-sm text-[#b3b1c4] transition-all duration-200 hover:bg-white/[0.07] hover:text-white hover:shadow-[inset_0_1px_0_rgba(255,255,255,0.1)]',
            ]"
            @click="$emit('close')"
          >
            <span class="text-base opacity-80 transition group-hover:opacity-100">{{ item.icon }}</span>
            <span class="font-medium">{{ item.label }}</span>
          </RouterLink>
        </div>
      </div>
    </nav>

    <!-- User section -->
    <div class="border-t border-white/[0.08] p-4">
      <div class="mb-3 flex items-center gap-3 rounded-2xl border border-white/[0.07] bg-white/[0.04] p-3 backdrop-blur-md">
        <div class="flex h-9 w-9 items-center justify-center rounded-full border border-white/20 bg-gradient-to-br from-indigo-500/40 to-purple-500/30 text-sm font-semibold text-indigo-200 shadow-[inset_0_1px_0_rgba(255,255,255,0.25)]">
          {{ auth.user?.username?.[0]?.toUpperCase() || 'A' }}
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm font-medium">{{ auth.user?.username || '管理员' }}</p>
          <p class="truncate text-[11px] text-[#7a7890]">{{ auth.user?.email || auth.user?.role || '超级管理员' }}</p>
        </div>
      </div>
      <button class="btn btn-secondary w-full" @click="auth.logout(); $router.push('/login')">
        退出登录
      </button>
    </div>
  </aside>

  <style scoped>
  .nav-link.active {
    background: linear-gradient(140deg, rgba(99, 102, 241, 0.26), rgba(168, 85, 247, 0.14));
    border: 1px solid rgba(99, 102, 241, 0.4);
    color: #fff;
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.18), 0 0 22px -6px rgba(99, 102, 241, 0.5);
  }
  .nav-link span {
    opacity: 1;
  }
  </style>
</template>
