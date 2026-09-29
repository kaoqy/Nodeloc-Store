<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import PaginationFooter from '../components/PaginationFooter.vue'
import { exportAuditLogs, listAuditActions, listAuditLogs } from '../api/logs'
import { errorMessage, when } from '../utils/format'
import type { AuditLog } from '../types'

const Limit = 25

const loading = ref(true)
const error = ref('')
const logs = ref<AuditLog[]>([])
const page = ref(1)
const totalPages = ref(1)
const total = ref(0)

// Filters: action prefix, keyword, actor, and a day-granular time window.
const actionFilter = ref('')
const searchFilter = ref('')
const actorFilter = ref('')
const systemOnly = ref(false)
const since = ref('')
const until = ref('')
const knownActions = ref<string[]>([])
const openDetail = ref(0)

const range = computed(() => ({ first: (page.value - 1) * Limit + 1, last: (page.value - 1) * Limit + logs.value.length }))

const presets = [
  { label: '今天', days: 0 },
  { label: '近 7 天', days: 6 },
  { label: '近 30 天', days: 29 },
]

const filtered = computed(
  () =>
    Boolean(
      actionFilter.value.trim() ||
        searchFilter.value.trim() ||
        actorFilter.value.trim() ||
        systemOnly.value ||
        since.value ||
        until.value,
    ),
)

const actorLabel = computed(() =>
  systemOnly.value ? '系统操作' : actorFilter.value.trim() ? `操作者 #${actorFilter.value.trim()}` : '',
)

/** local calendar day, `days` back from today */
function day(daysBack: number): string {
  const date = new Date()
  date.setDate(date.getDate() - daysBack)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const dayOfMonth = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${dayOfMonth}`
}

// A shortcut is lit by the range it produced, not by a separate flag: editing
// the date boxes by hand takes the highlight off without any bookkeeping.
function presetActive(days: number): boolean {
  return since.value === day(days) && !until.value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    let result = await listAuditLogs(query())
    // 地址栏里的页码可能比实际页数还大：有记录就退回最后一页再取一次，
    // 一条都没有就回第 1 页，别把店家丢在一个看着像坏掉的空白页上。
    if (!result.items.length && result.total > 0 && page.value > 1) {
      page.value = Math.max(1, result.total_pages)
      syncUrl()
      result = await listAuditLogs(query())
    } else if (!result.total && page.value > 1) {
      page.value = 1
      syncUrl()
    }
    logs.value = result.items
    total.value = result.total
    totalPages.value = result.total_pages || 1
    openDetail.value = 0
  } catch (err) {
    error.value = errorMessage(err, '加载日志失败')
  } finally {
    loading.value = false
  }
}

function query() {
  return {
    page: page.value,
    limit: Limit,
    action: actionFilter.value.trim() || undefined,
    search: searchFilter.value.trim() || undefined,
    actor: systemOnly.value ? 'system' : actorFilter.value.trim() || undefined,
    since: since.value || undefined,
    until: until.value || undefined,
  }
}

/**
 * The filters live in the address bar: a log page that already points at one
 * member or one day can be pasted into a support thread. This rewrites the URL
 * rather than routing to it, because the back-office shell keys its view by
 * fullPath and a route change would throw away and rebuild this very screen on
 * every filter click.
 */
function syncUrl() {
  const params = new URLSearchParams()
  if (actionFilter.value.trim()) params.set('action', actionFilter.value.trim())
  if (searchFilter.value.trim()) params.set('search', searchFilter.value.trim())
  if (systemOnly.value) params.set('actor', 'system')
  else if (actorFilter.value.trim()) params.set('actor', actorFilter.value.trim())
  if (since.value) params.set('since', since.value)
  if (until.value) params.set('until', until.value)
  if (page.value > 1) params.set('page', String(page.value))
  const query = params.toString()
  window.history.replaceState(null, '', query ? `/admin/logs?${query}` : '/admin/logs')
}

function apply() {
  page.value = 1
  syncUrl()
  load()
}

function pickPreset(days: number) {
  if (presetActive(days)) {
    since.value = ''
    until.value = ''
  } else {
    since.value = day(days)
    until.value = ''
  }
  apply()
}

function toggleSystem() {
  systemOnly.value = !systemOnly.value
  if (systemOnly.value) actorFilter.value = ''
  apply()
}

function clearActor() {
  systemOnly.value = false
  actorFilter.value = ''
  apply()
}

function reset() {
  actionFilter.value = ''
  searchFilter.value = ''
  actorFilter.value = ''
  systemOnly.value = false
  since.value = ''
  until.value = ''
  apply()
}

function go(next: number) {
  page.value = Math.min(Math.max(next, 1), totalPages.value)
  syncUrl()
  load()
}

const exporting = ref(false)
const notice = ref('')

/**
 * 导出的是当前这批日志：出事之后要把记录贴进工单、或者自己拉进表格里排时间线，
 * 照着屏幕上 25 条一页地抄是办不成的。传的是筛选条件，不含分页。
 */
async function downloadLogs() {
  if (exporting.value) return
  exporting.value = true
  error.value = ''
  notice.value = ''
  try {
    const { rows, truncated } = await exportAuditLogs({
      action: actionFilter.value.trim() || undefined,
      search: searchFilter.value.trim() || undefined,
      actor: systemOnly.value ? 'system' : actorFilter.value.trim() || undefined,
      since: since.value || undefined,
      until: until.value || undefined,
    })
    notice.value = truncated
      ? `已导出 ${rows} 条日志，但符合条件的记录比这更多，文件只装了前面那些 —— 请把日期范围缩小后再导一次。`
      : `已导出${filtered.value ? '当前筛选的' : '全部'} ${rows} 条日志。`
  } catch (err) {
    error.value = errorMessage(err, '导出日志失败')
  } finally {
    exporting.value = false
  }
}

function toggleDetail(id: number) {
  openDetail.value = openDetail.value === id ? 0 : id
}

/** What the log calls an operator: the account's name while it exists. */
function who(log: AuditLog): string {
  if (!log.actor_id) return '系统'
  return log.actor_name || `#${log.actor_id}`
}

const route = useRoute()

onMounted(async () => {
  const { action, search, actor, since: from, until: to, page: asked } = route.query
  if (typeof action === 'string') actionFilter.value = action
  if (typeof search === 'string') searchFilter.value = search
  if (actor === 'system') systemOnly.value = true
  else if (typeof actor === 'string' && /^\d+$/.test(actor)) actorFilter.value = actor
  if (typeof from === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(from)) since.value = from
  if (typeof to === 'string' && /^\d{4}-\d{2}-\d{2}$/.test(to)) until.value = to
  if (typeof asked === 'string' && /^\d+$/.test(asked)) page.value = Math.max(1, Number(asked))

  // The suggestion list is history, not a fixed catalogue: a shop that has only
  // ever created orders sees order.* actions here.
  loadAuditActions()
  await load()
})

async function loadAuditActions() {
  try {
    knownActions.value = await listAuditActions()
  } catch {
    // Losing suggestions must not cost the operator the log page itself.
    knownActions.value = []
  }
}
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center gap-2">
      <input
        v-model="actionFilter"
        class="input w-52 mono"
        type="search"
        list="known-audit-actions"
        placeholder="操作类型，如 order.create"
        aria-label="按操作类型过滤日志"
        @keyup.enter="apply"
      />
      <datalist id="known-audit-actions">
        <option v-for="name in knownActions" :key="name" :value="name" />
      </datalist>
      <input
        v-model="searchFilter"
        class="input w-56"
        type="search"
        placeholder="关键词：操作 / 对象 / 详情 / 用户名"
        aria-label="按关键词搜索日志内容"
        @keyup.enter="apply"
      />
      <input
        v-model="actorFilter"
        class="input w-28"
        type="search"
        inputmode="numeric"
        placeholder="操作者 ID"
        aria-label="按操作者用户 ID 过滤日志"
        :disabled="systemOnly"
        @keyup.enter="apply"
      />
      <input v-model="since" class="input w-40" type="date" aria-label="开始日期" @change="apply" />
      <span class="quiet text-xs" aria-hidden="true">至</span>
      <input v-model="until" class="input w-40" type="date" aria-label="结束日期" @change="apply" />
      <button class="btn btn-secondary btn-sm" @click="apply">筛选</button>
      <button
        class="chip"
        :class="{ 'chip-active': systemOnly }"
        :aria-pressed="systemOnly"
        title="只看商店自己写下的记录（对账、交付重试等），不含成员操作"
        @click="toggleSystem"
      >
        只看系统操作
      </button>
      <button
        v-for="item in presets"
        :key="item.label"
        class="chip"
        :class="{ 'chip-active': presetActive(item.days) }"
        :aria-pressed="presetActive(item.days)"
        @click="pickPreset(item.days)"
      >
        {{ item.label }}
      </button>
      <button v-if="filtered" class="btn btn-quiet btn-sm" @click="reset">清空筛选</button>
    </div>

    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="flex flex-wrap items-center gap-2 text-sm">
        <span class="quiet text-xs mono">{{ filtered ? `符合筛选 ${total} 条` : `共 ${total} 条` }}</span>
        <template v-if="actorLabel">
          <span class="chip chip-active">{{ actorLabel }}</span>
          <button class="btn btn-quiet btn-sm" @click="clearActor">取消该筛选</button>
        </template>
      </p>
      <!-- 导出只是读，所以跟这一页本身走：能打开日志页就导得动。 -->
      <button
        class="btn btn-quiet btn-sm"
        :disabled="exporting || loading"
        :title="filtered ? '按当前筛选导出' : '导出全部日志'"
        @click="downloadLogs"
      >
        <span v-if="exporting" class="spinner" />
        {{ exporting ? '正在生成…' : filtered ? '导出当前筛选' : '导出日志 CSV' }}
      </button>
    </div>

    <p v-if="notice" class="alert alert-info" role="status">{{ notice }}</p>
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>操作</th>
            <th>对象</th>
            <th>详情</th>
            <th>操作者</th>
            <th>IP</th>
            <th>时间</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 6" :key="`skeleton-${i}`">
              <td colspan="7"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!logs.length">
            <td colspan="7">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">{{ filtered ? '没有符合条件的日志' : '暂无操作日志' }}</p>
                <p class="empty-hint">
                  {{
                    filtered
                      ? '把时间放宽一点，或者少填几个条件再试。'
                      : '后台的每一次写入都会记录在这里。'
                  }}
                </p>
              </div>
            </td>
          </tr>
          <template v-else>
            <template v-for="log in logs" :key="log.id">
              <tr>
                <td class="nums text-sm quiet">{{ log.id }}</td>
                <td><span class="badge badge-neutral mono">{{ log.action }}</span></td>
                <td class="max-w-[180px] truncate text-sm muted" :title="log.target || ''">{{ log.target || '—' }}</td>
                <td class="max-w-[280px] truncate text-sm quiet">
                  <button
                    class="w-full truncate text-left hover:underline"
                    :aria-expanded="openDetail === log.id"
                    :title="log.detail || ''"
                    @click="toggleDetail(log.id)"
                  >
                    {{ log.detail || '—' }}
                  </button>
                </td>
                <td class="text-sm">
                  <RouterLink
                    v-if="log.actor_id"
                    :to="`/users/${log.actor_id}`"
                    class="accent-text underline-offset-2 hover:underline"
                    :title="`用户 #${log.actor_id}`"
                  >
                    {{ who(log) }}
                  </RouterLink>
                  <span v-else class="quiet">系统</span>
                </td>
                <td class="mono text-sm quiet">{{ log.ip || '—' }}</td>
                <td class="whitespace-nowrap text-sm quiet">{{ when(log.created_at) }}</td>
              </tr>
              <!-- One click opens the whole record line: the detail column is the
                   one that gets truncated, and it is the one threads quote. -->
              <tr v-if="openDetail === log.id" class="bg-[var(--surface-sunken)]">
                <td colspan="7" class="py-2">
                  <p class="mono text-xs mb-1">
                    {{ log.action }} · {{ who(log) }} · {{ log.target || '无对象' }} · {{ when(log.created_at) }}
                  </p>
                  <p class="hint whitespace-pre-wrap break-all">{{ log.detail || '这条日志没有更多详情。' }}</p>
                </td>
              </tr>
            </template>
          </template>
        </tbody>
      </table>
    </div>

    <PaginationFooter
      :page="page"
      :pages="totalPages"
      :loading="loading"
      :summary="logs.length ? `第 ${range.first}–${range.last} 条 · 共 ${total} 条` : `共 ${total} 条`"
      @change="go"
    />
  </section>
</template>
