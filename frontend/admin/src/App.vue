<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute } from 'vue-router'
import SideBar from './components/SideBar.vue'
import RouteProgress from './components/RouteProgress.vue'
import { useAuthStore } from './stores/auth'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const auth = useAuthStore()
const theme = useThemeStore()
const open = ref(false)

const titles: Record<string, string> = {
  '/': '仪表盘',
  '/products': '商品管理',
  '/orders': '订单管理',
  '/cards': '卡密管理',
  '/categories': '分类管理',
  '/coupons': '优惠券',
  '/users': '用户管理',
  '/notifications': '通知中心',
  '/roles': '角色权限',
  '/logs': '审计日志',
  '/settings': '系统设置',
  '/forbidden': '权限不足',
}

const label = (segment: string) => titles['/' + segment] || segment

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
        <div class="flex h-[68px] items-center gap-3 px-5 sm:px-7">
          <button class="btn btn-quiet !px-3 lg:hidden" aria-label="打开导航菜单" @click="open = true">☰</button>

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

          <div class="ml-auto flex items-center gap-2">
            <button
              class="btn btn-ghost !px-2.5"
              :title="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
              :aria-label="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
              @click="theme.toggle"
            >
              <span aria-hidden="true">{{ theme.theme === 'dark' ? '☀' : '☾' }}</span>
            </button>
            <RouterLink v-if="auth.allows('notifications', 'view')" to="/notifications" class="btn btn-quiet btn-sm">通知</RouterLink>
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
