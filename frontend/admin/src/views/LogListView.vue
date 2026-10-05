<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import ManagementPage from '../components/ManagementPage.vue'
import { exportAuditLogs, listAuditActions, listAuditLogs } from '../api/logs'
import { errorMessage, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { AuditLog } from '../types'

/**
 * 操作日志：谁、在什么时候、改了什么、结果如何。
 *
 * 筛选条件全部写进地址栏，于是「昨天某位管理员改的那条」是一条可以发给别人的链接。
 * 关键词参数名必须与后端一致（search），否则搜索框看着能用、其实什么都没过滤。
 */

const PAGE_SIZE = 30

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const COLUMNS: Column[] = [
  { label: '时间', width: '150px' },
  { label: '操作者' },
  { label: '操作' },
  { label: '对象', hideOnMobile: true },
  { label: '摘要' },
  { label: '来源 IP', hideOnMobile: true },
  { label: '', actions: true, width: '90px' },
]

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const logs = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const action = ref('')
const keyword = ref('')
const actor = ref('')
const since = ref('')
const until = ref('')
const actionOptions = ref<string[]>([])
const expanded = ref<number | null>(null)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(() =>
  Boolean(action.value || keyword.value.trim() || actor.value || since.value || until.value),
)
const expandedLog = computed(() => logs.value.find((item) => item.id === expanded.value) ?? null)

/** 把审计详情里的 JSON 拆成人看的键值对；不是 JSON 就原样给出。 */
const expandedFields = computed(() => {
  const raw = expandedLog.value?.detail
  if (!raw) return []
  try {
    const parsed = JSON.parse(raw) as unknown
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return Object.entries(parsed as Record<string, unknown>).map(([key, value]) => ({
        key,
        value: typeof value === 'string' ? value : JSON.stringify(value, null, 1),
      }))
    }
    return [{ key: 'detail', value: JSON.stringify(parsed, null, 1) }]
  } catch {
    return [{ key: 'detail', value: raw }]
  }
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAuditLogs({
      page: page.value,
      limit: PAGE_SIZE,
      action: action.value || undefined,
      // 参数名是 search：写成 q 会被后端忽略，搜索框就成了摆设。
      search: keyword.value.trim() || undefined,
      actor: actor.value.trim() || undefined,
      since: since.value || undefined,
      until: until.value || undefined,
    })
    logs.value = result.items
    total.value = result.total
    if (!logs.value.some((item) => item.id === expanded.value)) expanded.value = null
  } catch (err) {
    error.value = errorMessage(err, '加载日志失败')
  } finally {
    loading.value = false
  }
}

function syncUrl() {
  const query: Record<string, string> = {}
  if (action.value) query.action = action.value
  if (keyword.value.trim()) query.search = keyword.value.trim()
  if (actor.value.trim()) query.actor = actor.value.trim()
  if (since.value) query.since = since.value
  if (until.value) query.until = until.value
  if (page.value > 1) query.page = String(page.value)
  void router.replace({ path: '/logs', query })
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

function clearFilters() {
  action.value = ''
  keyword.value = ''
  actor.value = ''
  since.value = ''
  until.value = ''
  apply()
}

async function download() {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const result = await exportAuditLogs({
      action: action.value || undefined,
      search: keyword.value.trim() || undefined,
      actor: actor.value.trim() || undefined,
      since: since.value || undefined,
      until: until.value || undefined,
    })
    notice.value = result.truncated
      ? '已导出 ' + result.rows + ' 条（达到上限被截断，请缩小时间范围再导出）。'
      : '已导出 ' + result.rows + ' 条记录。'
  } catch (err) {
    error.value = errorMessage(err, '导出失败')
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  if (typeof route.query.action === 'string') action.value = route.query.action
  if (typeof route.query.search === 'string') keyword.value = route.query.search
  if (typeof route.query.actor === 'string') actor.value = route.query.actor
  if (typeof route.query.since === 'string') since.value = route.query.since
  if (typeof route.query.until === 'string') until.value = route.query.until
  if (typeof route.query.page === 'string') page.value = Math.max(1, Number(route.query.page) || 1)
  actionOptions.value = await listAuditActions().catch(() => [])
  await load()
})
</script>

<template>
  <section class="space-y-4">
    <ManagementPage
      title="操作日志"
      description="后台的每一次写操作都记在这里：操作者、对象、变更内容与来源 IP。"
      eyebrow="安全与审计"
      :metrics="[
        { label: '记录总数', value: total, hint: '当前筛选结果' },
        { label: '本页操作', value: logs.length, hint: '当前页日志条数' },
       { label: '来源 IP', value: logs.filter((item) => item.ip).length, hint: '本页可追溯到来源' },
      ]"
    >
      <template #actions>
        <button v-if="auth.allows('logs', 'manage')" class="btn btn-quiet btn-sm" :disabled="busy" @click="download">
          <AdminIcon name="download" :size="14" />
          {{ busy ? '导出中…' : '导出 CSV' }}
        </button>
        <button class="btn btn-quiet btn-sm" :disabled="loading" @click="load">
          <AdminIcon name="refresh" :size="14" />
          {{ loading ? '刷新中…' : '刷新' }}
        </button>
      </template>
    </ManagementPage>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <FilterBar :count="total ? '共 ' + total + ' 条记录' : ''">
      <select v-model="action" class="input !w-auto" aria-label="操作类型" @change="apply">
        <option value="">全部操作</option>
        <option v-for="item in actionOptions" :key="item" :value="item">{{ item }}</option>
      </select>
      <input
        v-model="keyword"
        class="input w-56"
        type="search"
        placeholder="搜索详情或对象"
        aria-label="搜索日志"
        @keyup.enter="apply"
      />
      <input
        v-model="actor"
        class="input !w-40"
        placeholder="操作者 ID 或 system"
        aria-label="操作者"
        @keyup.enter="apply"
      />
      <label class="flex items-center gap-1.5 text-xs text-[var(--text-dim)]">
        从
        <input v-model="since" class="input !w-36" type="date" aria-label="开始日期" @change="apply" />
      </label>
      <label class="flex items-center gap-1.5 text-xs text-[var(--text-dim)]">
        到
        <input v-model="until" class="input !w-36" type="date" aria-label="结束日期" @change="apply" />
      </label>
      <button class="btn btn-secondary btn-sm" @click="apply">查询</button>
      <template #actions>
        <button v-if="filtered" class="btn btn-quiet btn-sm" @click="clearFilters">清除筛选</button>
      </template>
    </FilterBar>

    <DataTable
      :columns="COLUMNS"
      :loading="loading"
      :error="error"
      :filtered="filtered"
      :page="page"
      :pages="pageCount"
      :total="total"
      :summary="'共 ' + total + ' 条记录'"
      empty-title="没有日志记录"
      empty-hint="后台发生写操作后就会出现在这里。"
      @retry="load"
      @clear-filters="clearFilters"
      @change="goPage"
    >
<template v-for="log in logs" :key="log.id">
          <tr>
            <td class="quiet mono text-[11.5px]">{{ when(log.created_at) }}</td>
            <td>
              <RouterLink v-if="log.actor_id" :to="'/users/' + log.actor_id" class="hover:accent-text">
                {{ log.actor_name || '#' + log.actor_id }}
              </RouterLink>
              <span v-else class="quiet">系统</span>
            </td>
            <td><span class="log-action">{{ log.action }}</span></td>
            <td class="mono hide-on-mobile max-w-[200px] truncate text-[11.5px]">{{ log.target || '—' }}</td>
            <td class="max-w-[280px] truncate text-[12.5px]">{{ log.detail || '—' }}</td>
            <td class="mono quiet hide-on-mobile text-[11px]">{{ log.ip || '—' }}</td>
            <td class="text-right">
              <button
                class="btn btn-quiet btn-sm"
                :aria-expanded="expanded === log.id"
                @click="expanded = expanded === log.id ? null : log.id"
              >
                {{ expanded === log.id ? '收起' : '详情' }}
              </button>
            </td>
          </tr>
          <tr v-if="expanded === log.id">
            <td colspan="7" class="bg-[var(--surface-sunken)]">
              <div class="log-detail">
                <div class="log-detail-head">
                  <span class="mono text-[11.5px]">{{ log.action }}</span>
                  <span class="quiet text-[11.5px]">{{ when(log.created_at) }} · {{ log.actor_name || (log.actor_id ? '#' + log.actor_id : '系统') }}</span>
                </div>
                <dl v-if="expandedFields.length" class="log-detail-grid">
                  <template v-for="field in expandedFields" :key="field.key">
                    <dt>{{ field.key }}</dt>
                    <dd>{{ field.value }}</dd>
                  </template>
                </dl>
                <p v-else class="quiet text-xs">这条记录没有更详细的字段。</p>
              </div>
            </td>
          </tr>
      </template>
    </DataTable>
  </section>
</template>

<style scoped>
.log-action {
  display: inline-block;
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke);
  background: var(--surface-sunken);
  padding: 1px 8px;
  font-family: var(--font-mono);
  font-size: 11.5px;
  color: var(--text-dim);
}
.log-detail { display: flex; flex-direction: column; gap: 8px; padding: 4px 2px; }
.log-detail-head { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; }
.log-detail-grid {
  display: grid;
  grid-template-columns: minmax(90px, 160px) minmax(0, 1fr);
  gap: 4px 14px;
  font-size: 12.5px;
}
.log-detail-grid dt { color: var(--text-quiet); font-family: var(--font-mono); font-size: 11.5px; }
.log-detail-grid dd { margin: 0; white-space: pre-wrap; word-break: break-word; color: var(--text-dim); }
</style>
