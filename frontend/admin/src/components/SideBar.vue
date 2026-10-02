<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import AdminIcon from './AdminIcon.vue'
import { useAuthStore } from '../stores/auth'
import { useInboxStore } from '../stores/inbox'
import { shopInitials, shopLogo, shopName } from '../utils/identity'

defineProps<{ open: boolean }>()
defineEmits(['close'])

const auth = useAuthStore()
const inbox = useInboxStore()
const route = useRoute()

const roleLabel = computed(() => {
  const role = auth.user?.role
  if (role === 'super_admin') return '超级管理员'
  if (role === 'admin') return '管理员'
  if (role === 'operator') return '运营'
  if (role === 'support') return '客服'
  return auth.user?.email || '管理员'
})

// Each entry carries the icon the sidebar draws beside it. Ordering follows the
// work a shop owner actually does in a day: sell, then look after buyers, then
// configure.
const groups = [
  { label: '概览', items: [{ path: '/', label: '仪表盘', icon: 'dashboard', permission: 'stats:view' }] },
  {
    label: '经营',
    items: [
      { path: '/products', label: '商品管理', icon: 'products', permission: 'products:view' },
      { path: '/orders', label: '订单管理', icon: 'orders', permission: 'orders:view' },
      { path: '/cards', label: '卡密管理', icon: 'cards', permission: 'cards:view' },
      { path: '/categories', label: '分类管理', icon: 'categories', permission: 'categories:view' },
      { path: '/coupons', label: '优惠券', icon: 'coupons', permission: 'coupons:view' },
      { path: '/activities', label: '活动营销', icon: 'coupons', permission: 'activities:view' },
      { path: '/plugins', label: '插件管理', icon: 'plugins', permission: 'plugins:view' },
    ],
  },
  {
    label: '客服与 AI',
    items: [
      // 工单与 AI 客服是同一套接待流程，只留一个一级入口。
      { path: '/service', label: '客服中心', icon: 'support', permission: 'tickets:view', altPermission: 'ai:view' },
    ],
  },
  { label: '客户', items: [{ path: '/users', label: '用户管理', icon: 'users', permission: 'users:view' }] },
  {
    label: '系统',
    items: [
      // 配置中心内部已有分组导航，侧栏不再为每个分组各开一个入口，
      // 否则同一个页面会在侧栏出现四次，点哪一次都到同一处。
      { path: '/config', label: '配置中心', icon: 'settings', permission: 'config_center:view', altPermission: 'ai:view' },
      { path: '/notifications', label: '通知中心', icon: 'notifications', permission: 'notifications:view' },
      { path: '/roles', label: '角色权限', icon: 'roles', permission: 'roles:view' },
      { path: '/logs', label: '审计日志', icon: 'logs', permission: 'logs:view' },
      { path: '/settings', label: '系统设置', icon: 'settings', permission: 'settings:view' },
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
        .filter((item) => {
          if (auth.allows(item.permission.split(':')[0], item.permission.split(':')[1])) return true
          // 客服中心同时承载工单与 AI 配置，任一项权限都可以进入。
          if (item.altPermission) {
            const [resource, action] = item.altPermission.split(':')
            return auth.allows(resource, action)
          }
          return false
        })
        .map((item) => {
          index += 1
          return {
            ...item,
            index: String(index).padStart(2, '0'),
            active: item.path === '/' ? route.path === '/' : route.path.startsWith(item.path),
            // The 通知中心 entry carries the unread count: restock warnings,
            // broadcasts and payment notices all land in the same inbox, and the
            // operator needs to see that something arrived without opening it.
            unread: item.path === '/notifications' ? inbox.unread : 0,
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
    <RouterLink to="/" class="flex h-[68px] items-center gap-3 border-b border-[var(--stroke)] px-4" @click="$emit('close')">
      <img v-if="shopLogo" :src="shopLogo" :alt="shopName" class="brand-mark object-cover" />
      <span v-else class="brand-mark">{{ shopInitials }}</span>
      <span class="min-w-0 flex-1">
        <span class="block truncate text-[14px] font-semibold">{{ shopName }}</span>
        <span class="hint block truncate">管理后台</span>
      </span>
      <button class="icon-btn !size-8 lg:hidden" type="button" aria-label="收起导航菜单" @click.prevent="$emit('close')">
        <AdminIcon name="chevron" :size="16" class="rotate-180" />
      </button>
    </RouterLink>

    <nav class="flex-1 overflow-y-auto px-3 py-4">
      <div v-for="group in numbered" :key="group.label" class="mb-5 last:mb-0">
        <p class="eyebrow mb-1.5 px-2.5">{{ group.label }}</p>
        <ul class="space-y-0.5">
          <li v-for="item in group.items" :key="item.path">
            <RouterLink
              :to="item.path"
              class="side-link"
              :class="{ 'side-link-active': item.active }"
              @click="$emit('close')"
            >
              <AdminIcon :name="item.icon" :size="17" class="side-icon" />
              <span class="truncate">{{ item.label }}</span>
              <span
                v-if="item.unread"
                class="nums badge-count"
                :aria-label="`${item.unread} 条未读通知`"
                >{{ item.unread > 99 ? '99+' : item.unread }}</span
              >
            </RouterLink>
          </li>
        </ul>
      </div>
    </nav>

    <div class="border-t border-[var(--stroke)] p-3">
      <div class="mb-2.5 flex items-center gap-3 rounded-[var(--radius-sm)] bg-[var(--surface-sunken)] p-3">
        <span class="grid size-8 shrink-0 place-items-center rounded-full bg-[var(--accent-soft)] text-[13px] font-bold text-[var(--accent)]">
          {{ auth.user?.username?.[0]?.toUpperCase() || 'A' }}
        </span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[13px] font-semibold">{{ auth.user?.username || '管理员' }}</span>
          <span class="hint block truncate">{{ roleLabel }}</span>
        </span>
      </div>
      <div class="flex gap-2">
        <a href="/" class="btn btn-quiet btn-sm flex-1" target="_blank" rel="noopener">
          <AdminIcon name="external" :size="15" />
          前台
        </a>
        <button class="btn btn-quiet btn-sm flex-1" @click="auth.logout(); $router.push('/login')">
          <AdminIcon name="logout" :size="15" />
          退出
        </button>
      </div>
    </div>
  </aside>
</template>

<style scoped>
/* The sidebar is a rail of icon + label rows. The active row is marked by an
   accent edge on the left rather than a filled background, so the list still
   reads as a list when the shop's accent colour is loud. */
.side-link {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px 8px 12px;
  border-radius: var(--radius-sm);
  font-size: 13.5px;
  font-weight: 500;
  color: var(--text-dim);
  transition: background var(--fast), color var(--fast);
}
.side-link::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  width: 3px;
  height: 0;
  border-radius: var(--radius-pill);
  background: var(--accent);
  transform: translateY(-50%);
  transition: height var(--normal) var(--spring);
}
.side-link:hover {
  background: var(--surface-hi);
  color: var(--text);
}
.side-icon {
  flex-shrink: 0;
  opacity: 0.85;
  transition: opacity var(--fast), color var(--fast);
}
.side-link-active {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 650;
}
.side-link-active::before { height: 17px; }
.side-link-active:hover {
  background: var(--accent-soft);
  color: var(--accent);
}
.side-link-active .side-icon { opacity: 1; }

/* The unread pill on 通知中心. */
.badge-count {
  margin-left: auto;
  flex-shrink: 0;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  padding: 1px 6px;
  font-size: 11px;
  font-weight: 700;
  color: var(--accent);
}
.side-link-active .badge-count {
  background: color-mix(in srgb, var(--accent) 20%, transparent);
}
</style>
