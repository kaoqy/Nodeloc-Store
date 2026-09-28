<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useAuthStore } from '../stores/auth'

defineProps<{ open: boolean }>()
defineEmits(['close'])

const auth = useAuthStore()
const route = useRoute()

const roleLabel = computed(() => {
  const role = auth.user?.role
  if (role === 'super_admin') return '超级管理员'
  if (role === 'admin') return '管理员'
  if (role === 'operator') return '运营'
  if (role === 'support') return '客服'
  return auth.user?.email || '管理员'
})

const groups = [
  { label: '概览', items: [{ path: '/', label: '仪表盘', permission: 'stats:view' }] },
  {
    label: '运营',
    items: [
      { path: '/products', label: '商品管理', permission: 'products:view' },
      { path: '/orders', label: '订单管理', permission: 'orders:view' },
      { path: '/cards', label: '卡密管理', permission: 'cards:view' },
      { path: '/categories', label: '分类管理', permission: 'categories:view' },
      { path: '/coupons', label: '优惠券', permission: 'coupons:view' },
    ],
  },
  { label: '用户', items: [{ path: '/users', label: '用户管理', permission: 'users:view' }] },
  {
    label: '系统',
    items: [
      { path: '/notifications', label: '通知中心', permission: 'notifications:view' },
      { path: '/roles', label: '角色权限', permission: 'roles:view' },
      { path: '/logs', label: '审计日志', permission: 'logs:view' },
      { path: '/settings', label: '系统设置', permission: 'settings:view' },
    ],
  },
]

// A flat, ordered list of only the screens this role may open, so each entry can
// carry a stable two-digit index. Hiding is presentation: the router guard and
// the API gate the same routes again.
const numbered = computed(() => {
  let index = 0
  return groups
    .map((group) => ({
      label: group.label,
      items: group.items
        .filter((item) => auth.allows(item.permission.split(':')[0], item.permission.split(':')[1]))
        .map((item) => {
          index += 1
          return {
            ...item,
            index: String(index).padStart(2, '0'),
            active: item.path === '/' ? route.path === '/' : route.path.startsWith(item.path),
          }
        }),
    }))
    .filter((group) => group.items.length)
})
</script>

<template>
  <div v-if="open" class="scrim fixed inset-0 z-30 lg:hidden" @click="$emit('close')" />

  <aside
    :class="[
      'fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-[var(--stroke)] bg-[var(--surface)] transition-transform duration-300 lg:translate-x-0',
      open ? 'translate-x-0' : '-translate-x-full',
    ]"
  >
    <RouterLink to="/" class="flex h-[68px] items-center gap-3 border-b border-[var(--stroke)] px-5" @click="$emit('close')">
      <span class="brand-mark">N</span>
      <span class="min-w-0">
        <span class="block truncate text-[14px] font-semibold">Nodeloc Store</span>
        <span class="hint block truncate">管理后台</span>
      </span>
    </RouterLink>

    <nav class="flex-1 overflow-y-auto px-3 py-5">
      <div v-for="group in numbered" :key="group.label" class="mb-6 last:mb-0">
        <p class="eyebrow mb-2 px-2.5">{{ group.label }}</p>
        <ul class="space-y-0.5">
          <li v-for="item in group.items" :key="item.path">
            <RouterLink
              :to="item.path"
              class="side-link"
              :class="{ 'side-link-active': item.active }"
              @click="$emit('close')"
            >
              <span class="nums side-index">{{ item.index }}</span>
              <span class="truncate">{{ item.label }}</span>
            </RouterLink>
          </li>
        </ul>
      </div>
    </nav>

    <div class="border-t border-[var(--stroke)] p-3">
      <div class="card-quiet !bg-[var(--surface-sunken)] mb-2.5 flex items-center gap-3 !p-3">
        <span class="grid size-8 shrink-0 place-items-center rounded-full bg-[var(--accent-soft)] text-[13px] font-bold text-[var(--accent)]">
          {{ auth.user?.username?.[0]?.toUpperCase() || 'A' }}
        </span>
        <span class="min-w-0">
          <span class="block truncate text-[13px] font-semibold">{{ auth.user?.username || '管理员' }}</span>
          <span class="hint block truncate">{{ roleLabel }}</span>
        </span>
      </div>
      <a href="/" class="btn btn-quiet btn-sm mb-2 w-full">前往商店前台</a>
      <button class="btn btn-quiet btn-sm w-full" @click="auth.logout(); $router.push('/login')">退出登录</button>
    </div>
  </aside>
</template>

<style scoped>
.side-link {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text-dim);
  transition: background var(--fast), color var(--fast);
}
.side-link:hover {
  background: var(--surface-hi);
  color: var(--text);
}
.side-index {
  font-size: 11px;
  color: var(--text-quiet);
  transition: color var(--fast);
}
.side-link-active {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 600;
}
.side-link-active:hover {
  background: var(--accent-soft);
  color: var(--accent);
}
.side-link-active .side-index {
  color: var(--accent);
}
</style>
