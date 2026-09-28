import { createRouter, createWebHistory } from 'vue-router'
import type { RouteLocationNormalized } from 'vue-router'
import { isUninitialized } from '../api/system'
import { useAuthStore } from '../stores/auth'
import { beginNavigation, endNavigation } from '../utils/progress'

// meta.permission is the screen's entry ticket, expressed in the same
// "resource:action" vocabulary the Casbin policies use. It only decides what a
// staff member can reach from the navigation — the API checks every call again,
// so a hidden page is never the only thing standing between them and a write.
const routes = [
  { path: '/setup', component: () => import('../views/SetupView.vue'), meta: { public: true } },
  { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
  { path: '/', component: () => import('../views/DashboardView.vue'), meta: { permission: 'stats:view' } },
  { path: '/products', component: () => import('../views/ProductListView.vue'), meta: { permission: 'products:view' } },
  { path: '/products/new', component: () => import('../views/ProductFormView.vue'), meta: { permission: 'products:manage' } },
  { path: '/products/:id/edit', component: () => import('../views/ProductFormView.vue'), meta: { permission: 'products:manage' } },
  { path: '/cards', component: () => import('../views/CardListView.vue'), meta: { permission: 'cards:view' } },
  { path: '/cards/:id', component: () => import('../views/CardListView.vue'), meta: { permission: 'cards:view' } },
  { path: '/orders', component: () => import('../views/OrderListView.vue'), meta: { permission: 'orders:view' } },
  { path: '/orders/:orderNo', component: () => import('../views/OrderDetailView.vue'), meta: { permission: 'orders:view' } },
  { path: '/users', component: () => import('../views/UserListView.vue'), meta: { permission: 'users:view' } },
  { path: '/users/:id', component: () => import('../views/UserDetailView.vue'), meta: { permission: 'users:view' } },
  { path: '/categories', component: () => import('../views/CategoryListView.vue'), meta: { permission: 'categories:view' } },
  { path: '/coupons', component: () => import('../views/CouponListView.vue'), meta: { permission: 'coupons:view' } },
  { path: '/notifications', component: () => import('../views/NotificationView.vue'), meta: { permission: 'notifications:view' } },
  { path: '/logs', component: () => import('../views/LogListView.vue'), meta: { permission: 'logs:view' } },
  { path: '/settings', component: () => import('../views/SettingsView.vue'), meta: { permission: 'settings:view' } },
  { path: '/roles', component: () => import('../views/RoleListView.vue'), meta: { permission: 'roles:view' } },
  { path: '/forbidden', component: () => import('../views/ForbiddenView.vue'), meta: { staffOnly: true } },
]

const router = createRouter({ history: createWebHistory(import.meta.env.BASE_URL), routes })

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

router.afterEach(() => {
  endNavigation()
  sessionStorage.removeItem('chunk-reload')
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
