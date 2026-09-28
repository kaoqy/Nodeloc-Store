<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const attempted = route.fullPath

// Not always the dashboard: a role without 仪表盘 is sent to its own landing by
// the router guard, so '/' is the right way back for every staff member.
const screens: { permission: string; label: string; path: string }[] = [
  { permission: 'stats:view', label: '仪表盘', path: '/' },
  { permission: 'products:view', label: '商品', path: '/products' },
  { permission: 'cards:view', label: '卡密', path: '/cards' },
  { permission: 'orders:view', label: '订单', path: '/orders' },
  { permission: 'categories:view', label: '分类', path: '/categories' },
  { permission: 'coupons:view', label: '优惠码', path: '/coupons' },
  { permission: 'users:view', label: '用户', path: '/users' },
  { permission: 'notifications:view', label: '通知', path: '/notifications' },
  { permission: 'logs:view', label: '审计日志', path: '/logs' },
  { permission: 'settings:view', label: '设置', path: '/settings' },
  { permission: 'roles:view', label: '角色权限', path: '/roles' },
]

const openable = screens.filter(
  (screen) => auth.permissions.includes('*:*') || auth.permissions.includes(screen.permission),
)
</script>

<template>
  <section class="mx-auto max-w-xl">
    <div class="card rise-in !p-7">
      <p class="eyebrow">页面不存在</p>
      <h2 class="mt-2 text-lg font-semibold">后台里没有这一页</h2>
      <p class="quiet mt-3 text-sm leading-relaxed">
        地址可能来自旧链接，或者这一页换了名字。这不是权限问题——有权限的页面会正常打开，不会跳到这里。
      </p>
      <p class="mono mt-4 break-all text-xs text-[var(--text-quiet)]">{{ attempted }}</p>

      <div class="mt-5">
        <p class="label">可以去的页面</p>
        <div v-if="openable.length" class="mt-2 flex flex-wrap gap-2">
          <RouterLink v-for="screen in openable" :key="screen.path" :to="screen.path" class="badge badge-neutral">
            {{ screen.label }}
          </RouterLink>
        </div>
        <p v-else class="quiet mt-2 text-sm">当前角色没有任何后台页面的查看权限。</p>
      </div>

      <div class="mt-6 flex gap-2">
        <button class="btn btn-primary btn-sm" @click="router.push('/')">回到我的首页</button>
        <a href="/" class="btn btn-quiet btn-sm">前往商店前台</a>
      </div>
    </div>
  </section>
</template>
