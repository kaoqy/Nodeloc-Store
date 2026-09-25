import { createRouter, createWebHistory } from 'vue-router'
import { isUninitialized } from '../api/system'
const routes=[
{path:'/setup',component:()=>import('../views/SetupView.vue'),meta:{public:true}},
{path:'/login',component:()=>import('../views/LoginView.vue'),meta:{public:true}},
{path:'/',component:()=>import('../views/DashboardView.vue')},
{path:'/products',component:()=>import('../views/ProductListView.vue')},
{path:'/products/new',component:()=>import('../views/ProductFormView.vue')},
{path:'/products/:id/edit',component:()=>import('../views/ProductFormView.vue')},
{path:'/products/:id/cards',component:()=>import('../views/CardListView.vue')},
{path:'/orders',component:()=>import('../views/OrderListView.vue')},
{path:'/orders/:orderNo',component:()=>import('../views/OrderDetailView.vue')},
{path:'/users',component:()=>import('../views/UserListView.vue')},{path:'/users/:id',component:()=>import('../views/UserDetailView.vue')},
{path:'/categories',component:()=>import('../views/CategoryListView.vue')},{path:'/coupons',component:()=>import('../views/CouponListView.vue')},
{path:'/settings',component:()=>import('../views/SettingsView.vue')},{path:'/logs',component:()=>import('../views/LogListView.vue')},{path:'/notifications',component:()=>import('../views/NotificationView.vue')}]
const router=createRouter({history:createWebHistory(import.meta.env.BASE_URL),routes})
router.beforeEach(async (to) => {
  if (await isUninitialized()) {
    return to.path === '/setup' ? true : { path: '/setup' }
  }
  if (to.path === '/setup') return { path: '/login' }
  if (!to.meta.public && !localStorage.getItem('admin_token')) return '/login'
  if (to.path === '/login' && localStorage.getItem('admin_token')) return '/'
})
export default router
