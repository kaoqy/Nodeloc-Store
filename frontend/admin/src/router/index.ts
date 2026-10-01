import { createRouter, createWebHistory } from 'vue-router'
import type { RouteLocationNormalized } from 'vue-router'
import { isUninitialized } from '../api/system'
import { useAuthStore } from '../stores/auth'
import { beginNavigation, endNavigation } from '../utils/progress'
import { setPageTitle } from '../utils/identity'

// meta.permission is the screen's entry ticket, expressed in the same
// "resource:action" vocabulary the Casbin policies use. It only decides what a
// staff member can reach from the navigation — the API checks every call again,
// so a hidden page is never the only thing standing between them and a write.
// meta.title is this screen's own words on the browser tab, in front of the shop
// name; it names the detail screens too, which the sidebar never lists.
const routes = [
  { path: '/setup', component: () => import('../views/SetupView.vue'), meta: { public: true, title: '初始化' } },
  { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true, title: '登录' } },
  { path: '/', component: () => import('../views/DashboardView.vue'), meta: { permission: 'stats:view', title: '仪表盘' } },
  { path: '/products', component: () => import('../views/ProductListView.vue'), meta: { permission: 'products:view', title: '商品管理' } },
  { path: '/products/new', component: () => import('../views/ProductFormView.vue'), meta: { permission: 'products:manage', title: '新建商品' } },
  { path: '/products/:id/edit', component: () => import('../views/ProductFormView.vue'), meta: { permission: 'products:manage', title: '编辑商品' } },
  { path: '/cards', component: () => import('../views/CardListView.vue'), meta: { permission: 'cards:view', title: '卡密管理' } },
  { path: '/cards/:id', component: () => import('../views/CardListView.vue'), meta: { permission: 'cards:view', title: '卡密管理' } },
  { path: '/orders', component: () => import('../views/OrderListView.vue'), meta: { permission: 'orders:view', title: '订单管理' } },
  { path: '/orders/:orderNo', component: () => import('../views/OrderDetailView.vue'), meta: { permission: 'orders:view', title: '订单详情' } },
  { path: '/users', component: () => import('../views/UserListView.vue'), meta: { permission: 'users:view', title: '用户管理' } },
  { path: '/users/:id', component: () => import('../views/UserDetailView.vue'), meta: { permission: 'users:view', title: '用户详情' } },
  { path: '/categories', component: () => import('../views/CategoryListView.vue'), meta: { permission: 'categories:view', title: '分类管理' } },
  { path: '/coupons', component: () => import('../views/CouponListView.vue'), meta: { permission: 'coupons:view', title: '优惠券' } },
  { path: '/notifications', component: () => import('../views/NotificationView.vue'), meta: { permission: 'notifications:view', title: '通知中心' } },
  { path: '/logs', component: () => import('../views/LogListView.vue'), meta: { permission: 'logs:view', title: '审计日志' } },
  { path: '/settings', component: () => import('../views/SettingsView.vue'), meta: { permission: 'settings:view', title: '系统设置' } },
  { path: '/roles', component: () => import('../views/RoleListView.vue'), meta: { permission: 'roles:view', title: '角色权限' } },
  { path: '/forbidden', component: () => import('../views/ForbiddenView.vue'), meta: { staffOnly: true, title: '无访问权限' } },
  // Last, so a declared route never loses to it. An address the back office does
  // not have used to render the shell around an empty panel.
  { path: '/:pathMatch(.*)*', name: 'not-found', component: () => import('../views/NotFoundView.vue'), meta: { staffOnly: true, title: '页面不存在' } },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes,
  // The back office is long pages: opening a paginated list from the bottom of
  // the dashboard left the new page scrolled to some unrelated middle row.
  scrollBehavior: () => ({ top: 0 }),
})

// After an upgrade the hashed chunks an open tab has already loaded are gone, so
// a lazy route import fails and the view never renders. Reload once for that
// target to pick up the new index.html instead of showing a blank panel.
router.onError((error, to) => {
  endNavigation()
  const message = String((error as Error)?.message || '')
  const staleChunk = /dynamically imported module|Importing a module script failed|Failed to fetch/.test(message)
  if (!staleChunk || sessionStorage.getItem('chunk-reload') === to.fullPath) return
  sessionStorage.setItem('chunk-reload', to.fullPath)
  window.location.assign(to.fullPath)
})

router.beforeEach(() => beginNavigation())

router.afterEach((to) => {
  endNavigation()
  sessionStorage.removeItem('chunk-reload')
  setPageTitle(typeof to.meta.title === 'string' ? to.meta.title : '')
})

function landing(permissions: string[], isSuper: boolean): string {
  if (isSuper) return '/'
  const order = [
    ['stats:view', '/'],
    ['products:view', '/products'],
    ['orders:view', '/orders'],
    ['cards:view', '/cards'],
    ['coupons:view', '/coupons'],
    ['categories:view', '/categories'],
    ['users:view', '/users'],
    ['notifications:view', '/notifications'],
    ['logs:view', '/logs'],
    ['settings:view', '/settings'],
    ['roles:view', '/roles'],
  ] as const
  for (const [permission, path] of order) {
    if (permissions.includes(permission)) return path
  }
  return '/forbidden'
}

function permitted(auth: ReturnType<typeof useAuthStore>, to: RouteLocationNormalized): boolean {
  const needed = to.meta.permission as string | undefined
  if (!needed) return true
  if (!auth.permissionsLoaded) return true
  if (auth.permissions.includes('*:*')) return true
  return auth.permissions.includes(needed)
}

router.beforeEach(async (to) => {
  if (await isUninitialized()) {
    return to.path === '/setup' ? true : { path: '/setup' }
  }
  if (to.path === '/setup') return '/login'

  const auth = useAuthStore()
  const staff = await auth.bootstrap()
  if (to.meta.public) {
    return to.path === '/login' && staff ? landing(auth.permissions, auth.isSuperAdmin) : true
  }
  if (!staff) {
    // A storefront visitor who is not staff is told why, instead of being
    // bounced into a login form they cannot satisfy.
    return auth.token ? { path: '/login', query: { reason: 'not_staff' } } : '/login'
  }
  if (permitted(auth, to)) return true
  // 仪表盘 is what a bare "sign in and take me in" lands on; for a role without
  // stats it is not a dead end, just the wrong room. Send them to their own
  // landing instead of the notice.
  if (to.path === '/') return landing(auth.permissions, auth.isSuperAdmin)
  // Reaching a screen the role does not cover is a navigation dead end, not an
  // error: send the visitor to their own landing page and say so on it.
  return { path: '/forbidden', query: { to: to.fullPath } }
})

export default router
