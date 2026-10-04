<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from './components/AdminIcon.vue'
import AppSidebar from './components/AppSidebar.vue'
import RouteProgress from './components/RouteProgress.vue'
import { breadcrumbsOf, titleOf } from './navigation'
import { useAuthStore } from './stores/auth'
import { useInboxStore } from './stores/inbox'
import { useThemeStore } from './stores/theme'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const inbox = useInboxStore()
const theme = useThemeStore()

const navOpen = ref(false)
const collapsed = ref(localStorage.getItem('admin.sidebar.collapsed') === '1')
const userMenu = ref(false)

function toggleCollapse() {
  collapsed.value = !collapsed.value
  localStorage.setItem('admin.sidebar.collapsed', collapsed.value ? '1' : '0')
}

// 页面标题：详情页用页面自己登记的标题（route.meta.title 会被子页面覆盖），
// 其余从导航表推导，保证侧栏、标题、面包屑永远说的是同一个名字。
const pageTitle = computed(() => titleOf(route.path, dynamicTitle.value))
const dynamicTitle = computed(() => {
  const meta = route.meta.title
  if (typeof meta === 'string' && meta && !['详情', '编辑'].includes(meta)) return meta
  return undefined
})
const crumbs = computed(() => breadcrumbsOf(route))

function logout() {
  userMenu.value = false
  auth.logout()
  void router.push('/login')
}

function goSettings() {
  userMenu.value = false
  void router.push('/settings')
}
</script>

<template>
  <RouteProgress />

  <!-- 登录与初始化是独立整页，不套后台外壳 -->
  <div v-if="route.path === '/login' || route.path === '/setup'" class="min-h-screen">
    <RouterView />
  </div>

  <div v-else class="min-h-screen">
    <AppSidebar
      :open="navOpen"
      :collapsed="collapsed"
      @close="navOpen = false"
      @toggle-collapse="toggleCollapse"
    />

    <div :class="['transition-[padding] duration-300', collapsed ? 'lg:pl-[72px]' : 'lg:pl-[248px]']">
      <header class="site-header sticky top-0 z-20">
        <div class="admin-topbar flex h-[64px] items-center gap-2.5 px-4 sm:px-6">
          <button class="icon-btn lg:hidden" type="button" aria-label="打开导航菜单" @click="navOpen = true">
            <AdminIcon name="menu" :size="18" />
          </button>

          <div class="admin-titleblock min-w-0">
            <h1 class="truncate text-[16px] font-bold leading-tight">{{ pageTitle }}</h1>
            <nav v-if="crumbs.length" class="crumb-row mt-0.5 hidden items-center gap-1.5 text-[11.5px] sm:flex" aria-label="路径">
              <template v-for="(crumb, index) in crumbs" :key="crumb.path">
                <AdminIcon v-if="index > 0" name="chevronRight" :size="11" class="text-[var(--text-quiet)]" />
                <RouterLink
                  v-if="!crumb.last"
                  :to="crumb.path"
                  class="text-[var(--text-quiet)] transition-colors hover:text-[var(--text-dim)]"
                >
                  {{ crumb.label }}
                </RouterLink>
                <span v-else class="text-[var(--text-quiet)]">{{ crumb.label }}</span>
              </template>
            </nav>
          </div>

          <div class="admin-toolbar ml-auto flex items-center gap-1">
            <RouterLink
              v-if="auth.allows('notifications', 'view')"
              to="/notifications"
              class="icon-btn"
              aria-label="通知中心"
            >
              <AdminIcon name="notifications" :size="18" />
              <span v-if="inbox.unread" class="count nums">{{ inbox.unread > 99 ? '99+' : inbox.unread }}</span>
            </RouterLink>

            <button
              class="icon-btn"
              type="button"
              :title="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
              :aria-label="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
              @click="theme.toggle"
            >
              <AdminIcon :name="theme.theme === 'dark' ? 'sun' : 'moon'" :size="18" />
            </button>

            <a href="/" class="btn btn-quiet btn-sm hidden sm:inline-flex" target="_blank" rel="noopener">
              看店铺
              <AdminIcon name="external" :size="13" />
            </a>

            <!-- 管理员菜单 -->
            <div class="relative">
              <button
                class="admin-chip"
                type="button"
                :aria-expanded="userMenu"
                aria-haspopup="menu"
                @click="userMenu = !userMenu"
              >
                <span class="grid size-6 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--accent-soft)] text-[11px] font-bold text-[var(--accent)]">
                  <img v-if="auth.user?.avatar_url" :src="auth.user.avatar_url" alt="" class="size-full object-cover" />
                  <span v-else>{{ (auth.user?.nickname || auth.user?.username || 'A').slice(0, 1).toUpperCase() }}</span>
                </span>
                <span class="hidden max-w-[7rem] truncate text-[12.5px] sm:block">
                  {{ auth.user?.nickname || auth.user?.username || '管理员' }}
                </span>
                <AdminIcon name="chevronDown" :size="13" />
              </button>

              <div v-if="userMenu" class="menu-panel" role="menu">
                <p class="menu-head">
                  <span class="block truncate font-semibold">{{ auth.user?.username }}</span>
                  <span class="quiet block truncate text-[11.5px]">{{ auth.user?.email || '未绑定邮箱' }}</span>
                </p>
                <button class="menu-item" type="button" role="menuitem" @click="goSettings">
                  <AdminIcon name="settings" :size="15" />
                  系统设置
                </button>
                <a class="menu-item" href="/" target="_blank" rel="noopener" role="menuitem" @click="userMenu = false">
                  <AdminIcon name="external" :size="15" />
                  返回前台
                </a>
                <button class="menu-item menu-item-danger" type="button" role="menuitem" @click="logout">
                  <AdminIcon name="logout" :size="15" />
                  退出登录
                </button>
              </div>
            </div>
          </div>
        </div>
      </header>

      <!-- 点击空白处收起管理员菜单 -->
      <div v-if="userMenu" class="fixed inset-0 z-10" @click="userMenu = false" />

      <main class="admin-main px-4 pb-14 pt-6 sm:px-6">
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
.page-enter-active { transition: opacity 200ms var(--ease), transform 200ms var(--spring); }
.page-leave-active { transition: opacity 100ms var(--ease); }
.page-enter-from { opacity: 0; transform: translateY(6px); }
.page-leave-to { opacity: 0; }

.admin-chip {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 4px 8px 4px 5px;
  border: 1px solid transparent;
  border-radius: var(--radius-pill);
  color: var(--text-dim);
  transition: background var(--fast), border-color var(--fast);
}
.admin-chip:hover { background: var(--surface-hi); border-color: var(--stroke); color: var(--text); }

.menu-panel {
  position: absolute;
  right: 0;
  top: calc(100% + 8px);
  z-index: 30;
  width: 15rem;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-md);
  background: var(--surface);
  box-shadow: var(--shadow-lg);
  padding: 5px;
}
.menu-head {
  border-bottom: 1px solid var(--stroke-quiet);
  padding: 8px 10px 10px;
  font-size: 13px;
}
.menu-item {
  display: flex;
  align-items: center;
  gap: 9px;
  width: 100%;
  border-radius: var(--radius-sm);
  padding: 8px 10px;
  font-size: 13px;
  color: var(--text-dim);
  transition: background var(--fast), color var(--fast);
}
.menu-item:hover { background: var(--surface-hi); color: var(--text); }
.menu-item-danger:hover { background: var(--danger-soft); color: var(--danger); }
</style>
