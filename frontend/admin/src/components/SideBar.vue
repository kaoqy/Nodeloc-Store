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
  <div v-if="open" class="fixed inset-0 z-30 bg-black/60 lg:hidden" @click="$emit('close')" />

  <!-- Sidebar -->
  <aside
    :class="[
      open ? 'translate-x-0' : '-translate-x-full',
      'fixed inset-y-0 left-0 z-40 flex w-60 flex-col border-r border-white/[0.08] bg-[#12121a]/95 backdrop-blur-xl transition-transform lg:translate-x-0',
    ]"
  >
    <!-- Logo -->
    <div class="flex h-16 items-center gap-3 border-b border-white/[0.08] px-5">
      <div
        class="flex h-9 w-9 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500 to-purple-600 text-sm font-bold shadow-lg shadow-indigo-500/20"
      >
        NL
      </div>
      <div class="flex flex-col">
        <span class="text-sm font-semibold tracking-tight">Nodeloc Store</span>
        <span class="text-[11px] text-[#6b6b80]">数字商品平台</span>
      </div>
    </div>

    <!-- Navigation -->
    <nav class="flex-1 overflow-y-auto px-3 py-4">
      <div v-for="group in groups" :key="group.label" class="mb-5">
        <p class="mb-2 px-3 text-[11px] font-semibold uppercase tracking-wider text-[#6b6b80]">
          {{ group.label }}
        </p>
        <div class="space-y-0.5">
          <RouterLink
            v-for="item in group.items"
            :key="item.path"
            :to="item.path"
            :class="[
              active(item.path),
              'nav-link group flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm text-[#a1a1b5] transition-all hover:bg-white/[0.06] hover:text-white',
            ]"
            @click="$emit('close')"
          >
            <span class="text-base opacity-80 group-hover:opacity-100">{{ item.icon }}</span>
            <span class="font-medium">{{ item.label }}</span>
          </RouterLink>
        </div>
      </div>
    </nav>

    <!-- User section -->
    <div class="border-t border-white/[0.08] p-4">
      <div class="mb-3 flex items-center gap-3">
        <div class="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-500/30 to-purple-500/30 text-sm font-medium text-indigo-300">
          {{ auth.user?.username?.[0]?.toUpperCase() || 'A' }}
        </div>
        <div class="min-w-0 flex-1">
          <p class="truncate text-sm font-medium">{{ auth.user?.username || '管理员' }}</p>
          <p class="truncate text-[11px] text-[#6b6b80]">{{ auth.user?.email || auth.user?.role || '超级管理员' }}</p>
        </div>
      </div>
      <button class="btn-secondary w-full" @click="auth.logout(); $router.push('/login')">
        退出登录
      </button>
    </div>
  </aside>

  <style scoped>
  .nav-link.active {
    background: rgba(99, 102, 241, 0.12);
    color: #818cf8;
  }
  .nav-link.active span {
    opacity: 1;
  }
  </style>
</template>
