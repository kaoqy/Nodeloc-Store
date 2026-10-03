<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import StatusBadge from '../components/StatusBadge.vue'
import TicketPanel from './support/TicketPanel.vue'
import {
  getTicketStats,
  listTickets,
  ticketPriorityLabels,
  ticketStatusLabels,
  ticketTypeOptions,
  type TicketStats,
  type TicketView,
} from '../api/support'
import { errorMessage, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'

/**
 * 客服中心：左侧队列 + 右侧处理面板。
 *
 * 筛选条件全部来自地址栏，因此总览的每一张卡片、搜索框、快速筛选都能
 * 还原出同一个列表——这也是「点进去筛选不生效」这类问题的根治办法：
 * 只有一个筛选来源，不存在两套状态。
 */

const PAGE_SIZE = 15

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const loading = ref(true)
const error = ref('')
const rows = ref<TicketView[]>([])
const total = ref(0)
const page = ref(1)
const search = ref('')
const status = ref('all')
const priority = ref('all')
const type = ref('all')
const attention = ref('')
const mine = ref(false)
const stats = ref<TicketStats | null>(null)
const activeId = ref(0)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const canView = computed(() => auth.allows('tickets', 'view'))

const QUICK = [
  { key: '', label: '全部' },
  { key: 'pending_human', label: '待人工处理', countKey: 'pending_human' as const },
  { key: 'unread', label: '未读', countKey: 'unread' as const },
  { key: 'overdue', label: '即将超时', countKey: 'overdue' as const },
  { key: 'urgent', label: '紧急', countKey: 'urgent' as const },
  { key: 'refund', label: '退款' },
  { key: 'card', label: '卡密' },
  { key: 'payment', label: '支付' },
]

function quickCount(key?: string): number | null {
  if (!key || !stats.value) return null
  const value = (stats.value as unknown as Record<string, number>)[key]
  return typeof value === 'number' ? value : null
}

/**
 * 地址栏是筛选的唯一来源。attention 表示业务视角（未读/超时/退款…），
 * status 是工单状态，mine 是「我的工单」。三者可以组合。
 */
async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, metric] = await Promise.all([
      listTickets({
        status: status.value === 'all' ? undefined : status.value,
        priority: priority.value === 'all' ? undefined : priority.value,
        type: type.value === 'all' ? undefined : type.value,
        attention: attention.value || undefined,
        mine: mine.value,
        q: search.value.trim() || undefined,
        limit: PAGE_SIZE,
        offset: (page.value - 1) * PAGE_SIZE,
      }),
      getTicketStats(mine.value).catch(() => null),
    ])
    rows.value = list.data
    total.value = list.total
    stats.value = metric
    // 选中的工单可能已被筛选排除，这时改选列表第一张，避免右侧停在一张
    // 看不见的记录上（曾经的 bug：列表换了，面板还显示旧单）。
    if (!rows.value.some((row) => row.id === activeId.value)) {
      activeId.value = rows.value.length ? rows.value[0].id : 0
    }
  } catch (err) {
    error.value = errorMessage(err, '加载工单失败')
  } finally {
    loading.value = false
  }
}

function syncUrl() {
  const query: Record<string, string> = {}
  if (attention.value) query.attention = attention.value
  if (status.value !== 'all') query.status = status.value
  if (priority.value !== 'all') query.priority = priority.value
  if (type.value !== 'all') query.type = type.value
  if (mine.value) query.mine = '1'
  if (search.value.trim()) query.q = search.value.trim()
  if (page.value > 1) query.page = String(page.value)
  void router.replace({ path: '/service', query })
}

function apply() {
  page.value = 1
  syncUrl()
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  syncUrl()
  void load()
}

function quickView(key: string) {
  attention.value = key
  status.value = 'all'
  mine.value = false
  apply()
}

function toggleMine() {
  mine.value = !mine.value
  attention.value = ''
  apply()
}

function clearFilters() {
  attention.value = ''
  status.value = 'all'
  priority.value = 'all'
  type.value = 'all'
  mine.value = false
  search.value = ''
  apply()
}

const narrowed = computed(
  () => Boolean(attention.value || mine.value || search.value.trim() || status.value !== 'all' || priority.value !== 'all' || type.value !== 'all'),
)

// 地址栏变化 → 重新同步筛选。这样从总览点卡片进来、以及浏览器前进后退
// 都能落到正确的列表上。
function fromRoute() {
  const q = route.query
  attention.value = typeof q.attention === 'string' ? q.attention : ''
  status.value = typeof q.status === 'string' && q.status ? q.status : 'all'
  priority.value = typeof q.priority === 'string' && q.priority ? q.priority : 'all'
  type.value = typeof q.type === 'string' && q.type ? q.type : 'all'
  mine.value = q.mine === '1'
  search.value = typeof q.q === 'string' ? q.q : ''
  const parsed = Number(q.page)
  page.value = Number.isInteger(parsed) && parsed > 0 ? parsed : 1
}

watch(
  () => [route.query.attention, route.query.status, route.query.priority, route.query.type, route.query.mine, route.query.q, route.query.page].join('|'),
  () => {
    fromRoute()
    void load()
  },
)

onMounted(() => {
  fromRoute()
  void load()
})
</script>

<template>
  <section class="space-y-4">
    <PageHeader
      title="工单列表"
      description="买家提交的工单在这里处理：查看内容、回复、分派与流转，完整沟通记录一并保留。"
      bordered
    >
      <template #actions>
        <button class="btn btn-quiet btn-sm" :disabled="loading" @click="load">
          <AdminIcon name="refresh" :size="14" />
          {{ loading ? '刷新中…' : '刷新' }}
        </button>
      </template>
    </PageHeader>

    <p v-if="!canView" class="alert alert-warning" role="alert">当前账号没有工单查看权限。</p>

    <template v-else>
      <!-- 统计卡：数字与该筛选下的列表条数一致 -->
      <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        <RouterLink to="/service?attention=pending_human" class="todo-card">
          <span class="min-w-0 flex-1">
            <span class="quiet block text-xs">待人工处理</span>
            <span class="nums accent-text mt-1 block text-xl font-bold">{{ stats?.pending_human ?? 0 }}</span>
          </span>
          <AdminIcon name="chevronRight" :size="15" class="text-[var(--text-quiet)]" />
        </RouterLink>
        <div class="card-quiet">
          <p class="quiet text-xs">解决率</p>
          <p class="nums mt-1 text-xl font-bold">{{ stats ? Math.round(stats.resolve_rate * 100) : 0 }}%</p>
        </div>
        <div class="card-quiet">
          <p class="quiet text-xs">满意度</p>
          <p class="nums mt-1 text-xl font-bold">
            {{ stats && stats.satisfaction_avg > 0 ? stats.satisfaction_avg.toFixed(1) : '—' }}
          </p>
        </div>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <button
          v-for="item in QUICK"
          :key="item.key"
          class="chip"
          :class="attention === item.key ? 'chip-active' : ''"
          @click="quickView(item.key)"
        >
          {{ item.label }}
          <span v-if="quickCount(item.countKey)" class="nums opacity-70">{{ quickCount(item.countKey) }}</span>
        </button>
        <button class="chip" :class="mine ? 'chip-active' : ''" @click="toggleMine">我的工单</button>
      </div>

      <div class="card flex flex-wrap items-center gap-2 !p-3">
        <input
          v-model="search"
          class="input w-52"
          type="search"
          placeholder="工单号 / 标题 / 订单号"
          aria-label="搜索工单"
          @keyup.enter="apply"
        />
        <select v-model="status" class="input !w-auto" aria-label="工单状态" @change="apply">
          <option value="all">全部状态</option>
          <option v-for="(label, value) in ticketStatusLabels" :key="value" :value="value">{{ label }}</option>
        </select>
        <select v-model="priority" class="input !w-auto" aria-label="优先级" @change="apply">
          <option value="all">全部优先级</option>
          <option v-for="(label, value) in ticketPriorityLabels" :key="value" :value="value">{{ label }}</option>
        </select>
        <select v-model="type" class="input !w-auto" aria-label="类型" @change="apply">
          <option value="all">全部类型</option>
          <option v-for="option in ticketTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
        </select>
        <button class="btn btn-secondary btn-sm" :disabled="loading" @click="apply">查询</button>
        <span class="nums quiet ml-auto text-xs">共 {{ total }} 张</span>
        <button v-if="narrowed" class="btn btn-quiet btn-sm" @click="clearFilters">清除筛选</button>
      </div>

      <div class="grid gap-4 xl:grid-cols-[minmax(0,340px)_minmax(0,1fr)]">
        <!-- 队列 -->
        <div class="space-y-2">
          <div v-if="loading" class="space-y-2">
            <div v-for="i in 6" :key="i" class="skeleton h-16 w-full" />
          </div>
          <div v-else-if="error" class="card text-center">
            <p class="alert alert-danger text-left">{{ error }}</p>
            <button class="btn btn-secondary btn-sm mt-3" @click="load">重新加载</button>
          </div>
          <div v-else-if="!rows.length" class="card py-16 text-center">
            <p class="font-semibold">没有符合条件的工单</p>
            <p class="quiet mt-1.5 text-sm">换个筛选条件，或等待买家提交新问题。</p>
            <button v-if="narrowed" class="btn btn-secondary btn-sm mt-5" @click="clearFilters">清除筛选</button>
          </div>
          <template v-else>
            <button
              v-for="row in rows"
              :key="row.id"
              class="queue-item"
              :class="activeId === row.id ? 'queue-item-active' : ''"
              @click="activeId = row.id"
            >
              <span class="flex items-start gap-2">
                <span class="min-w-0 flex-1 truncate text-[13px] font-semibold">{{ row.subject }}</span>
                <span v-if="row.unread" class="badge-info shrink-0">未读</span>
              </span>
              <span class="mono quiet mt-1 block truncate text-[11px]">
                {{ row.ticket_no }} · {{ row.username || '#' + row.user_id }}
              </span>
              <span class="mt-1.5 flex flex-wrap items-center gap-1.5">
                <StatusBadge :value="row.status" :label="ticketStatusLabels[row.status]" />
                <StatusBadge :value="row.priority" :label="ticketPriorityLabels[row.priority]" />
                <span class="badge-neutral">{{ row.handler === 'ai' ? 'AI' : '人工' }}</span>
                <span class="quiet ml-auto text-[11px]">{{ when(row.last_message_at || row.created_at) }}</span>
              </span>
            </button>
            <PaginationFooter
              :page="page"
              :pages="pageCount"
              :loading="loading"
              :summary="'共 ' + total + ' 张'"
              @change="goPage"
            />
          </template>
        </div>

        <!-- 处理面板 -->
        <TicketPanel v-if="activeId" :key="activeId" :ticket-id="activeId" @changed="load" />
        <div v-else class="card py-16 text-center text-sm text-[var(--text-quiet)]">
          选择左侧工单开始处理
        </div>
      </div>
    </template>
  </section>
</template>

<style scoped>
.queue-item {
  display: block;
  width: 100%;
  text-align: left;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
  padding: 10px 12px;
  transition: border-color var(--fast), background var(--fast);
}
.queue-item:hover { border-color: var(--stroke-hi); background: var(--surface-hi); }
.queue-item-active { border-color: var(--accent-line); background: var(--accent-soft); }

.todo-card {
  display: flex;
  align-items: center;
  gap: 10px;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
  padding: 12px 14px;
  transition: border-color var(--fast), transform var(--normal) var(--spring);
}
.todo-card:hover { border-color: var(--stroke-hi); transform: translateY(-1px); }
</style>
