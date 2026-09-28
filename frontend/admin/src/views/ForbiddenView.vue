<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const SCREENS: Record<string, string> = {
  stats: '仪表盘',
  products: '商品',
  cards: '卡密',
  orders: '订单',
  users: '用户',
  categories: '分类',
  coupons: '优惠码',
  notifications: '通知',
  settings: '设置',
  roles: '角色权限',
  logs: '审计日志',
}

const attempted = computed(() => {
  const target = String(route.query.to || '')
  const resource = target.split('/').filter(Boolean)[0] ?? ''
  return SCREENS[resource] || target || '这个页面'
})

// The server's permission list is the only source here too, so this page cannot
// promise a screen the role does not actually hold.
const reachable = computed(() => {
  if (auth.permissions.includes('*:*')) return ['全部后台页面']
  const seen = new Set<string>()
  for (const permission of auth.permissions) {
    const [resource, action] = permission.split(':')
    if (action !== 'view' || seen.has(resource)) continue
    const label = SCREENS[resource]
    if (label) seen.add(label)
  }
  return [...seen]
})

function back() {
  router.push('/')
}
</script>

<template>
  <section class="mx-auto max-w-xl">
    <div class="card !p-7">
      <p class="eyebrow">权限不足</p>
      <h2 class="mt-2 text-lg font-semibold">你的角色看不到「{{ attempted }}」</h2>
      <p class="quiet mt-3 text-sm leading-relaxed">
        后台的每个页面都按角色开放，这里缺的是「查看」这一项，并不是账号出了问题。需要更多页面的话，请让店铺管理员在
        角色权限 里为你的角色勾选。
      </p>

      <div class="mt-5">
        <p class="label">当前角色（{{ auth.user?.role || '—' }}）可以访问</p>
        <div v-if="reachable.length" class="mt-2 flex flex-wrap gap-2">
          <span v-for="screen in reachable" :key="screen" class="badge badge-neutral">{{ screen }}</span>
        </div>
        <p v-else class="quiet mt-2 text-sm">这个角色目前没有任何后台页面的查看权限。</p>
      </div>

      <div class="mt-6 flex gap-2">
        <button class="btn btn-primary btn-sm" @click="back">回到我的首页</button>
        <a href="/" class="btn btn-quiet btn-sm">前往商店前台</a>
      </div>
    </div>
  </section>
</template>
