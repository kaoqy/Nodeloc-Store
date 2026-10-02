import { createRouter, createWebHistory } from 'vue-router'
import { beginNavigation, endNavigation } from '../utils/progress'
import { setPageTitle } from '../utils/identity'
import HomeView from '../views/HomeView.vue'
import LoginView from '../views/LoginView.vue'
import RegisterView from '../views/RegisterView.vue'
import ProductDetailView from '../views/ProductDetailView.vue'
import OrderListView from '../views/OrderListView.vue'
import OrderDetailView from '../views/OrderDetailView.vue'
import ProfileView from '../views/ProfileView.vue'
import OAuthCallbackView from '../views/OAuthCallbackView.vue'
import NotFoundView from '../views/NotFoundView.vue'
import HelpView from '../views/HelpView.vue'
import SupportView from '../views/SupportView.vue'

// meta.title is the words this page puts in front of the shop's name on the
// browser tab. The two detail screens leave it out and set it themselves once
// they know which product or order the visitor opened.
const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', name: 'home', component: HomeView },
    { path: '/login', name: 'login', component: LoginView, meta: { guestOnly: true, title: '登录' } },
    { path: '/register', name: 'register', component: RegisterView, meta: { guestOnly: true, title: '注册' } },
    { path: '/products/:slug', name: 'product-detail', component: ProductDetailView },
    { path: '/orders', name: 'orders', component: OrderListView, meta: { requiresAuth: true, title: '我的订单' } },
    { path: '/orders/:orderNo', name: 'order-detail', component: OrderDetailView, meta: { requiresAuth: true } },
    { path: '/profile', name: 'profile', component: ProfileView, meta: { requiresAuth: true, title: '个人中心' } },
    { path: '/activities', name: 'activities', component: SupportView, meta: { title: '活动中心' } },
    { path: '/tickets', name: 'tickets', component: SupportView, meta: { requiresAuth: true, title: '我的工单' } },
    { path: '/help', name: 'help', component: HelpView, meta: { title: '帮助中心' } },
    { path: '/oauth/callback', name: 'oauth-callback', component: OAuthCallbackView, meta: { title: '登录中' } },
    // Last, so a named route never loses to it. Without this an address the shop
    // does not have leaves the page body empty, which reads as a broken store.
    { path: '/:pathMatch(.*)*', name: 'not-found', component: NotFoundView, meta: { title: '页面不存在' } },
  ],
  scrollBehavior: () => ({ top: 0 }),
})

router.beforeEach((to) => {
  beginNavigation()
  const authenticated = Boolean(localStorage.getItem('token'))
  if (to.meta.requiresAuth && !authenticated) {
    return { name: 'login', query: { redirect: to.fullPath } }
  }
  if (to.meta.guestOnly && authenticated) {
    return { name: 'home' }
  }
})

router.afterEach((to) => {
  endNavigation()
  // Navigating clears the previous page's words: a product name that outlives
  // its own route would sit on the tab until the next reload.
  setPageTitle(typeof to.meta.title === 'string' ? to.meta.title : '')
})

export default router
