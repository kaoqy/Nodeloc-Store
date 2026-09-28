<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import { useThemeStore } from '../stores/theme'

const auth = useAuthStore()
const site = useSiteStore()
const theme = useThemeStore()
const router = useRouter()
const menuOpen = ref(false)

async function logout() {
  menuOpen.value = false
  auth.logout()
  await router.push('/')
}
</script>

<template>
  <header class="site-header">
    <div class="mx-auto flex h-16 w-full max-w-6xl items-center gap-4 px-4 sm:px-6">
      <RouterLink to="/" class="flex items-center gap-2.5 font-semibold tracking-tight">
        <img v-if="site.logo" :src="site.logo" :alt="site.name" class="brand-mark object-cover" />
        <span v-else class="brand-mark">{{ site.initials }}</span>
        <span class="hidden text-[15px] sm:block">{{ site.name }}</span>
      </RouterLink>

      <nav class="ml-4 hidden items-center gap-6 lg:flex">
        <RouterLink to="/" class="nav-item">商品</RouterLink>
        <template v-if="auth.isAuthenticated">
          <RouterLink to="/orders" class="nav-item">我的订单</RouterLink>
          <RouterLink to="/profile" class="nav-item">个人中心</RouterLink>
        </template>
      </nav>

      <div class="ml-auto flex items-center gap-2">
        <button
          class="btn btn-ghost !px-2.5"
          :title="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
          :aria-label="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
          @click="theme.toggle"
        >
          <span aria-hidden="true">{{ theme.theme === 'dark' ? '☀' : '☾' }}</span>
        </button>

        <template v-if="auth.isAuthenticated">
          <RouterLink to="/profile" class="btn btn-quiet btn-sm max-w-[10rem] truncate">
            {{ auth.user?.nickname || auth.user?.username || '我的账户' }}
          </RouterLink>
          <!-- Plain anchor: /admin is a separate SPA, so it needs a full load. -->
          <a v-if="auth.canEnterAdmin" href="/admin/" class="btn btn-quiet btn-sm">进入后台</a>
          <button class="btn btn-ghost btn-sm hidden sm:inline-flex" @click="logout">退出</button>
        </template>
        <template v-else>
          <RouterLink to="/login" class="btn btn-ghost btn-sm">登录</RouterLink>
          <RouterLink to="/register" class="btn btn-primary btn-sm">注册</RouterLink>
        </template>

        <button class="btn btn-quiet btn-sm lg:hidden" aria-label="打开菜单" @click="menuOpen = !menuOpen">
          {{ menuOpen ? '✕' : '☰' }}
        </button>
      </div>
    </div>

    <nav
      v-if="menuOpen"
      class="fade-in flex flex-col gap-1 border-t border-[var(--stroke)] px-4 py-3 lg:hidden"
      @click="menuOpen = false"
    >
      <RouterLink to="/" class="nav-item w-full">全部商品</RouterLink>
      <template v-if="auth.isAuthenticated">
        <RouterLink to="/orders" class="nav-item w-full">我的订单</RouterLink>
        <RouterLink to="/profile" class="nav-item w-full">个人中心</RouterLink>
        <a v-if="auth.canEnterAdmin" href="/admin/" class="nav-item w-full">进入后台</a>
        <button class="btn btn-quiet btn-sm mt-1 self-start" @click="logout">退出登录</button>
      </template>
      <template v-else>
        <RouterLink to="/login" class="nav-item w-full">登录</RouterLink>
        <RouterLink to="/register" class="nav-item w-full">注册</RouterLink>
      </template>
    </nav>
  </header>
</template>
