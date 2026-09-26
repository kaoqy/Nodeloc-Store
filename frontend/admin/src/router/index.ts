import { createRouter, createWebHistory } from 'vue-router'
import { isUninitialized } from '../api/system'
import { useAuthStore } from '../stores/auth'

const routes = [
  { path: '/setup', component: () => import('../views/SetupView.vue'), meta: { public: true } },
  { path: '/login', component: () => import('../views/LoginView.vue'), meta: { public: true } },
  { path: '/', component: () => import('../views/DashboardView.vue') },
  { path: '/products', component: () => import('../views/ProductListView.vue') },
  { path: '/products/new', component: () => import('../views/ProductFormView.vue') },
  { path: '/products/:id/edit', component: () => import('../views/ProductFormView.vue') },
  { path: '/cards', component: () => import('../views/CardListView.vue') },
  { path: '/cards/:id', component: () => import('../views/CardListView.vue') },
  { path: '/orders', component: () => import('../views/OrderListView.vue') },
  { path: '/orders/:orderNo', component: () => import('../views/OrderDetailView.vue') },
  { path: '/users', component: () => import('../views/UserListView.vue') },
  { path: '/users/:id', component: () => import('../views/UserDetailView.vue') },
  { path: '/categories', component: () => import('../views/CategoryListView.vue') },
  { path: '/coupons', component: () => import('../views/CouponListView.vue') },
  { path: '/notifications', component: () => import('../views/NotificationView.vue') },
  { path: '/logs', component: () => import('../views/LogListView.vue') },
  { path: '/settings', component: () => import('../views/SettingsView.vue') },
]

const router = createRouter({ history: createWebHistory(import.meta.env.BASE_URL), routes })

// After an upgrade the hashed chunks an open tab has already loaded are gone, so
// a lazy route import fails and the view never renders. Reload once for that
// target to pick up the new index.html instead of showing a blank panel.
router.onError((error, to) => {
  const message = String((error as Error)?.message || '')
  const staleChunk = /dynamically imported module|Importing a module script failed|Failed to fetch/.test(message)
  if (!staleChunk || sessionStorage.getItem('chunk-reload') === to.fullPath) return
  sessionStorage.setItem('chunk-reload', to.fullPath)
  window.location.assign(to.fullPath)
})

router.afterEach(() => sessionStorage.removeItem('chunk-reload'))

router.beforeEach(async (to) => {
  if (await isUninitialized()) {
    return to.path === '/setup' ? true : { path: '/setup' }
  }
  if (to.path === '/setup') return '/login'

  const auth = useAuthStore()
  const admin = await auth.bootstrap()
  if (to.meta.public) {
    return to.path === '/login' && admin ? '/' : true
  }
  if (admin) return true
  // A storefront visitor who is not an admin is told why, instead of being
  // bounced into a login form they cannot satisfy.
  return auth.token ? { path: '/login', query: { reason: 'not_admin' } } : '/login'
})

export default router
