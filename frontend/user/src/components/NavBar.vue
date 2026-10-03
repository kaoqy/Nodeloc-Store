<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { useInboxStore } from '../stores/inbox'
import { useSiteStore } from '../stores/site'
import { useThemeStore } from '../stores/theme'

const auth = useAuthStore()
const site = useSiteStore()
const theme = useThemeStore()
const inbox = useInboxStore()
const route = useRoute()
const router = useRouter()
const menuOpen = ref(false)
const isHome = computed(() => route.path === '/')

// The badge counts the whole inbox, not the page 个人中心 happens to show.
onMounted(() => {
  if (auth.isAuthenticated) void inbox.refresh()
})
watch(() => auth.isAuthenticated, (signedIn) => {
  if (signedIn) void inbox.refresh()
  else inbox.reset(0)
})

// The account's own picture, whichever side it came from: an uploaded or edited
// avatar wins, a bound NodeLoc account shows the forum's copy.
const avatar = computed(() => auth.user?.avatar_url || auth.user?.oauth_avatar || '')
const initial = computed(() =>
  (auth.user?.nickname || auth.user?.username || '我').slice(0, 1).toUpperCase(),
)
const badge = computed(() => (inbox.unread > 99 ? '99+' : String(inbox.unread)))

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
        <RouterLink to="/activities" class="nav-item">活动</RouterLink>
        <template v-if="auth.isAuthenticated">
          <RouterLink to="/orders" class="nav-item">我的订单</RouterLink>
          <RouterLink v-if="!isHome" to="/support" class="nav-item">客服中心</RouterLink>
          <RouterLink to="/profile" class="nav-item relative">
            个人中心
            <span
              v-if="inbox.unread"
              class="nums absolute -top-1 -right-3 rounded-full bg-[var(--accent)] px-1.5 text-[10px] leading-4 font-bold text-[var(--on-accent)]"
              :aria-label="`${inbox.unread} 条未读通知`"
              >{{ badge }}</span
            >
          </RouterLink>
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
          <RouterLink to="/profile" class="btn btn-quiet btn-sm max-w-[8rem] gap-2 sm:max-w-[12rem]">
            <span class="grid size-5 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--surface-hi)] text-[11px] font-bold">
              <img v-if="avatar" :src="avatar" alt="" class="size-full object-cover" />
              <span v-else>{{ initial }}</span>
            </span>
            <span class="truncate">{{ auth.user?.nickname || auth.user?.username || '我的账户' }}</span>
            <span
              v-if="inbox.unread"
              class="nums shrink-0 rounded-full bg-[var(--accent)] px-1.5 text-[10px] leading-4 font-bold text-[var(--on-accent)]"
              :aria-label="`${inbox.unread} 条未读通知`"
              >{{ badge }}</span
            >
          </RouterLink>
          <!-- Plain anchor: /admin is a separate SPA, so it needs a full load.
               Narrow phones list this in the ☰ drawer instead, next to 退出登录. -->
          <a v-if="auth.canEnterAdmin" href="/admin/" class="btn btn-quiet btn-sm hidden sm:inline-flex">进入后台</a>
          <button class="btn btn-ghost btn-sm hidden sm:inline-flex" @click="logout">退出</button>
        </template>
        <template v-else>
          <RouterLink to="/login" class="btn btn-ghost btn-sm">登录</RouterLink>
          <RouterLink v-if="site.registrationEnabled" to="/register" class="btn btn-primary btn-sm">注册</RouterLink>
        </template>

        <button
          class="btn btn-quiet btn-sm lg:hidden"
          :aria-label="menuOpen ? '关闭菜单' : '打开菜单'"
          :aria-expanded="menuOpen"
          @click="menuOpen = !menuOpen"
        >
          {{ menuOpen ? '✕' : '☰' }}
        </button>
      </div>
    </div>

    <nav
      class="mobile-nav flex max-h-[calc(100dvh-4rem)] flex-col gap-1 overflow-y-auto border-t border-[var(--stroke)] px-4 py-3 lg:hidden"
      :class="menuOpen ? 'mobile-nav-open' : 'mobile-nav-closed'"
      :aria-hidden="!menuOpen"
      :inert="!menuOpen"
      @click="menuOpen = false"
    >
      <RouterLink to="/" class="nav-item w-full">全部商品</RouterLink>
      <RouterLink to="/activities" class="nav-item w-full">活动中心</RouterLink>
      <template v-if="auth.isAuthenticated">
        <RouterLink to="/orders" class="nav-item w-full">我的订单</RouterLink>
        <RouterLink v-if="!isHome" to="/support" class="nav-item w-full">客服中心</RouterLink>
        <RouterLink to="/profile" class="nav-item w-full">
          个人中心<span v-if="inbox.unread" class="nums ml-2 text-[var(--accent)]">{{ badge }} 未读</span>
        </RouterLink>
        <a v-if="auth.canEnterAdmin" href="/admin/" class="nav-item w-full">进入后台</a>
        <button class="btn btn-quiet btn-sm mt-1 self-start" @click="logout">退出登录</button>
      </template>
      <template v-else>
        <RouterLink to="/login" class="nav-item w-full">登录</RouterLink>
        <RouterLink v-if="site.registrationEnabled" to="/register" class="nav-item w-full">注册</RouterLink>
      </template>
    </nav>
  </header>
</template>
