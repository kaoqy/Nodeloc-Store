<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from './AdminIcon.vue'
import { NAV_GROUPS, searchNav, type NavItem } from '../navigation'
import { useAuthStore } from '../stores/auth'
import { useInboxStore } from '../stores/inbox'
import { shopInitials, shopLogo, shopName } from '../utils/identity'

const props = withDefaults(defineProps<{ open: boolean; collapsed: boolean }>(), { collapsed: false })
const emit = defineEmits<{ close: []; toggleCollapse: [] }>()

const auth = useAuthStore()
const inbox = useInboxStore()
const route = useRoute()
const router = useRouter()

// 桌面端侧栏常驻、可折叠；移动端是抽屉。用同一个断点判断，
// 避免在桌面宽度下把常驻侧栏误设成 inert 而点不动。
const isDesktop = ref(false)
let media: MediaQueryList | undefined
function syncDesktop(event?: MediaQueryListEvent) {
  isDesktop.value = event ? event.matches : Boolean(media?.matches)
}

// 全局搜索：按标题、路径与关键词匹配，只列出当前角色真有权限打开的页面。
const query = ref('')
const searchOpen = ref(false)
const searchInput = ref<HTMLInputElement | null>(null)

const results = computed<NavItem[]>(() =>
  searchNav(query.value).filter((item) => allowed(item)).slice(0, 8),
)

function allowed(item: NavItem): boolean {
  const [resource, action] = item.permission.split(':')
  if (auth.allows(resource, action)) return true
  if (item.altPermission) {
    const [altResource, altAction] = item.altPermission.split(':')
    return auth.allows(altResource, altAction)
  }
  return false
}

// 每个分组只保留当前角色能打开的条目；空分组直接消失，避免「点进去 403」。
const groups = computed(() =>
  NAV_GROUPS.map((group) => ({ label: group.label, items: group.items.filter(allowed) })).filter(
    (group) => group.items.length > 0,
  ),
)

function isActive(path: string): boolean {
  if (path === '/') return route.path === '/'
  return route.path === path || route.path.startsWith(path + '/')
}

const roleLabel = computed(() => {
  const role = auth.user?.role ?? auth.accountRole
  const map: Record<string, string> = {
    super_admin: '超级管理员', admin: '管理员', operator: '运营', support: '客服',
    ops_manager: '运营管理员', product_manager: '商品管理员', order_manager: '订单管理员',
    finance: '财务', support_lead: '客服主管', support_agent: '普通客服',
    ai_admin: '管理员', data_viewer: '数据查看员',
  }
  return map[role] ?? '成员'
})

const unread = computed(() => (inbox.unread > 99 ? '99+' : String(inbox.unread)))

function go(path: string) {
  query.value = ''
  searchOpen.value = false
  emit('close')
  void router.push(path)
}

// 快捷键：⌘/Ctrl + K 打开搜索，Esc 关闭。店主不用离开键盘就能跳页面。
function onKeydown(event: KeyboardEvent) {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
    event.preventDefault()
    searchOpen.value = true
    void Promise.resolve().then(() => searchInput.value?.focus())
    return
  }
  if (event.key === 'Escape') {
    searchOpen.value = false
    query.value = ''
  }
}

watch(
  () => route.fullPath,
  () => {
    // 切页时关掉搜索面板并清空关键词，避免下次打开还停在上一次的查询上。
    searchOpen.value = false
    query.value = ''
    // 移动端抽屉：点完菜单就自动收起，不再是「点了菜单还要再点一次关闭」。
    if (props.open) emit('close')
  },
  { immediate: true },
)

onMounted(() => {
  media = window.matchMedia('(min-width: 1024px)')
  syncDesktop()
  media.addEventListener('change', syncDesktop)
  window.addEventListener('keydown', onKeydown)
})
onUnmounted(() => {
  media?.removeEventListener('change', syncDesktop)
  window.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div v-if="open" class="scrim fixed inset-0 z-30 lg:hidden" @click="emit('close')" />

  <aside
    :class="[
      'admin-aside fixed inset-y-0 left-0 z-40 flex flex-col border-r border-[var(--stroke)] bg-[var(--surface)] transition-[transform,width] duration-300',
      collapsed ? 'lg:w-[72px]' : 'lg:w-[248px]',
      'w-[min(280px,86vw)]',
      open ? 'translate-x-0' : '-translate-x-full lg:translate-x-0',
    ]"
    :aria-hidden="!open && !isDesktop"
    :inert="!open && !isDesktop"
    aria-label="后台导航"
  >
    <!-- 品牌区：折叠后只留标记 -->
    <div class="flex h-[64px] shrink-0 items-center gap-2.5 border-b border-[var(--stroke)] px-3.5">
      <RouterLink to="/" class="flex min-w-0 items-center gap-2.5" @click="emit('close')">
        <img v-if="shopLogo" :src="shopLogo" :alt="shopName" class="brand-mark object-cover" />
        <span v-else class="brand-mark">{{ shopInitials }}</span>
        <span v-if="!collapsed" class="min-w-0 flex-1">
          <span class="block truncate text-[13.5px] font-semibold">{{ shopName }}</span>
          <span class="hint block truncate">管理后台</span>
        </span>
      </RouterLink>
      <button
        v-if="!collapsed"
        class="icon-btn !size-8 ml-auto lg:hidden"
        type="button"
        aria-label="收起导航菜单"
        @click="emit('close')"
      >
        <AdminIcon name="close" :size="15" />
      </button>
    </div>

    <!-- 搜索：唯一入口，⌘K 也在同一处 -->
    <div class="shrink-0 px-3 pt-3">
      <button
        v-if="collapsed"
        class="icon-btn w-full"
        type="button"
        aria-label="搜索页面"
        @click="searchOpen = true"
      >
        <AdminIcon name="search" :size="17" />
      </button>
      <button
        v-else
        class="search-trigger w-full"
        type="button"
        @click="searchOpen = true"
      >
        <AdminIcon name="search" :size="15" />
        <span class="flex-1 text-left">搜索页面…</span>
        <kbd class="kbd">⌘K</kbd>
      </button>
    </div>

    <nav class="min-h-0 flex-1 overflow-y-auto overscroll-contain px-3 py-3">
      <div v-for="group in groups" :key="group.label" class="mb-4 last:mb-0">
        <p v-if="!collapsed" class="eyebrow mb-1.5 px-2">{{ group.label }}</p>
        <ul class="space-y-0.5">
          <li v-for="item in group.items" :key="item.path">
            <RouterLink
              :to="item.path"
              class="side-link"
              :class="{ 'side-link-active': isActive(item.path), 'justify-center !px-0': collapsed }"
              :title="collapsed ? item.label : undefined"
              @click="emit('close')"
            >
              <AdminIcon :name="item.icon" :size="17" class="side-icon" />
              <span v-if="!collapsed" class="truncate">{{ item.label }}</span>
              <span
                v-if="!collapsed && item.path === '/config' && inbox.unread"
                class="nums badge-count"
                :aria-label="inbox.unread + ' 条未读通知'"
                >{{ unread }}</span
              >
            </RouterLink>
          </li>
        </ul>
      </div>

    </nav>

    <!-- 底部：折叠开关 + 身份 -->
    <div class="shrink-0 border-t border-[var(--stroke)] p-3">
      <button
        class="side-link w-full"
        :class="{ 'justify-center !px-0': collapsed }"
        type="button"
        :aria-label="collapsed ? '展开侧栏' : '收起侧栏'"
        @click="emit('toggleCollapse')"
      >
        <AdminIcon :name="collapsed ? 'chevronRight' : 'chevronLeft'" :size="16" class="side-icon" />
        <span v-if="!collapsed">收起侧栏</span>
      </button>

      <div v-if="!collapsed" class="mt-2 flex items-center gap-2.5 rounded-[var(--radius-sm)] bg-[var(--surface-sunken)] p-2.5">
        <span class="grid size-8 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--accent-soft)] text-[13px] font-bold text-[var(--accent)]">
          <img v-if="auth.user?.avatar_url" :src="auth.user.avatar_url" alt="" class="size-full object-cover" />
          <span v-else>{{ (auth.user?.nickname || auth.user?.username || 'A').slice(0, 1).toUpperCase() }}</span>
        </span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[12.5px] font-semibold">{{ auth.user?.nickname || auth.user?.username || '管理员' }}</span>
          <span class="hint block truncate">{{ roleLabel }}</span>
        </span>
        <button class="icon-btn !size-7" type="button" aria-label="退出登录" @click="auth.logout(); router.push('/login')">
          <AdminIcon name="logout" :size="14" />
        </button>
      </div>
    </div>
  </aside>

  <!-- 搜索面板 -->
  <Teleport to="body">
    <div v-if="searchOpen" class="scrim fixed inset-0 z-50 flex items-start justify-center p-4 pt-[12vh]" @click.self="searchOpen = false">
      <div class="card w-full max-w-lg !p-0" role="dialog" aria-label="搜索页面">
        <div class="flex items-center gap-2.5 border-b border-[var(--stroke)] px-4 py-3">
          <AdminIcon name="search" :size="17" class="text-[var(--text-quiet)]" />
          <input
            ref="searchInput"
            v-model="query"
            class="flex-1 border-0 bg-transparent text-sm outline-none"
            placeholder="输入页面名称，例如 订单、卡密、工单"
            aria-label="搜索页面"
            @keydown.enter="results[0] && go(results[0].path)"
          />
          <kbd class="kbd">Esc</kbd>
        </div>
        <ul v-if="results.length" class="max-h-80 overflow-y-auto p-2">
          <li v-for="item in results" :key="item.path">
            <button class="side-link w-full text-left" type="button" @click="go(item.path)">
              <AdminIcon :name="item.icon" :size="16" class="side-icon" />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13px]">{{ item.label }}</span>
                <span class="quiet block truncate text-[11px]">{{ item.hint }}</span>
              </span>
            </button>
          </li>
        </ul>
        <p v-else-if="query" class="quiet px-4 py-8 text-center text-sm">没有匹配的页面</p>
        <p v-else class="quiet px-4 py-8 text-center text-sm">
          输入关键词搜索后台页面；当前角色只能搜到自己有权限的页面。
        </p>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* ── 侧栏条目 ───────────────────────────────────────────────────── */
.side-link {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 7.5px 10px 7.5px 12px;
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
.side-link:hover { background: var(--surface-hi); color: var(--text); }
.side-icon { flex-shrink: 0; opacity: 0.85; }
.side-link-active { background: var(--accent-soft); color: var(--accent); font-weight: 650; }
.side-link-active::before { height: 17px; }
.side-link-active .side-icon { opacity: 1; }

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
.side-link-active .badge-count { background: color-mix(in srgb, var(--accent) 20%, transparent); }

/* ── 搜索入口 ───────────────────────────────────────────────────── */
.search-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  padding: 7px 10px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-sm);
  background: var(--surface-sunken);
  color: var(--text-quiet);
  font-size: 12.5px;
  transition: border-color var(--fast), color var(--fast);
}
.search-trigger:hover { border-color: var(--stroke-hi); color: var(--text-dim); }
.kbd {
  border: 1px solid var(--stroke);
  border-radius: var(--radius-xs);
  padding: 1px 5px;
  font-size: 10.5px;
  font-family: var(--font-mono);
}
</style>
