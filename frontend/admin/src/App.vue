<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import SideBar from './components/SideBar.vue'
import RouteProgress from './components/RouteProgress.vue'
import { useAuthStore } from './stores/auth'
import { useInboxStore } from './stores/inbox'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const auth = useAuthStore()
const inbox = useInboxStore()
const theme = useThemeStore()
const open = ref(false)

// A restock warning arrives from the background sweep rather than from a click,
// so nothing in the shell would otherwise refresh the dot: it is read when a
// session opens and when one ends. The inbox page and the dashboard each refresh
// it after they change the unread count themselves.
watch(
  () => auth.isAuthenticated,
  (signedIn) => {
    if (signedIn) void inbox.refresh()
    else inbox.reset(0)
  },
  { immediate: true },
)

const titles: Record<string, string> = {
  '/': '仪表盘',
  '/products': '商品管理',
  '/orders': '订单管理',
  '/cards': '卡密管理',
  '/categories': '分类管理',
  '/coupons': '优惠券',
  '/activities': '活动营销',
  '/service': '客服中心',
  '/knowledge': '知识库',
  '/config': '配置中心',
  '/users': '用户管理',
  '/notifications': '通知中心',
  '/plugins': '插件管理',
  '/roles': '角色权限',
  '/logs': '审计日志',
  '/settings': '系统设置',
  '/forbidden': '权限不足',
}

// 子页面段：/products/new、/products/7/edit 会在面包屑里落下最后一段，
// 原样写「new」「edit」就是把路由表贴给店家看。
const subTitles: Record<string, string> = {
  new: '新建',
  edit: '编辑',
  setup: '初始化',
  login: '登录',
}

const label = (segment: string) => titles['/' + segment] || subTitles[segment] || segment

// The catch-all has no section to name, and its raw path would otherwise sit in
// the header as if it were a screen the shop has.
const pageTitle = computed(() =>
  route.name === 'not-found'
    ? '页面不存在'
    : label(route.path.split('/').filter(Boolean)[0] ?? '') || '仪表盘',
)

const breadcrumb = computed(() => {
  if (route.name === 'not-found') return []
  const segments = route.path.split('/').filter(Boolean)
  return segments.map((segment, index) => ({
    label: label(segment),
    path: '/' + segments.slice(0, index + 1).join('/'),
    last: index === segments.length - 1,
  }))
})
</script>

<template>
  <RouteProgress />

  <div v-if="route.path === '/login' || route.path === '/setup'" class="min-h-screen">
    <RouterView />
  </div>

  <div v-else class="min-h-screen">
    <SideBar :open="open" @close="open = false" />

    <div class="lg:pl-64">
      <header class="site-header">
        <div class="admin-topbar flex h-[68px] items-center gap-3 px-5 sm:px-7">
          <button class="icon-btn lg:hidden" aria-label="打开导航菜单" @click="open = true">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="size-[18px]" aria-hidden="true">
              <path d="M4 7h16M4 12h16M4 17h16" />
            </svg>
          </button>

          <div class="min-w-0">
            <p class="eyebrow">管理后台</p>
            <h1 class="truncate text-[17px] font-bold leading-tight">{{ pageTitle }}</h1>
          </div>

          <nav class="ml-4 hidden items-center gap-2 text-[13px] md:flex" aria-label="路径">
            <template v-for="crumb in breadcrumb" :key="crumb.path">
              <span v-if="!crumb.last" class="text-[var(--text-quiet)]">/</span>
              <RouterLink
                v-if="!crumb.last"
                :to="crumb.path"
                class="text-[var(--text-quiet)] transition-colors hover:text-[var(--text-dim)]"
              >
                {{ crumb.label }}
              </RouterLink>
              <span v-else class="nums mono text-[var(--text-quiet)]">{{ crumb.label }}</span>
            </template>
          </nav>

          <div class="ml-auto flex items-center gap-1.5">
            <button
              class="icon-btn"
              type="button"
              :title="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
              :aria-label="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
              @click="theme.toggle"
            >
              <svg v-if="theme.theme === 'dark'" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" class="size-[18px]" aria-hidden="true">
                <circle cx="12" cy="12" r="4" />
                <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
              </svg>
              <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" class="size-[18px]" aria-hidden="true">
                <path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />
              </svg>
            </button>

            <span class="topbar-sep hidden sm:block" />

            <RouterLink v-if="auth.allows('notifications', 'view')" to="/notifications" class="icon-btn" aria-label="通知中心">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" class="size-[18px]" aria-hidden="true">
                <path d="M18 8A6 6 0 1 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" />
                <path d="M13.7 21a2 2 0 0 1-3.4 0" />
              </svg>
              <span v-if="inbox.unread" class="count nums">{{ inbox.unread > 99 ? '99+' : inbox.unread }}</span>
            </RouterLink>

            <a href="/" class="btn btn-quiet btn-sm ml-1 hidden sm:inline-flex" target="_blank" rel="noopener">
              看店铺
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" class="size-3.5" aria-hidden="true">
                <path d="M7 17 17 7M9 7h8v8" />
              </svg>
            </a>
          </div>
        </div>
      </header>

      <main class="px-5 pb-12 pt-7 sm:px-7">
        <!-- Keyed by fullPath: without it, moving between two records of the same
             view (order → order, user → user) reuses the component and shows the
             previous record's data. -->
        <RouterView v-slot="{ Component, route: view }">
          <Transition name="page" mode="out-in">
            <component :is="Component" :key="view.fullPath" />
          </Transition>
        </RouterView>
      </main>
    </div>
  </div>
</template>

<style scoped>
.page-enter-active {
  transition: opacity 220ms var(--ease), transform 220ms var(--spring);
}
.page-leave-active { transition: opacity 110ms var(--ease); }
.page-enter-from { opacity: 0; transform: translateY(8px); }
.page-leave-to { opacity: 0; }
</style>
