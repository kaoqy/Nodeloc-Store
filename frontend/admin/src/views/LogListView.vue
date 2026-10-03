<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { exportAuditLogs, listAuditActions, listAuditLogs } from '../api/logs'
import { errorMessage, when } from '../utils/format'
import type { AuditLog } from '../types'

/** 操作日志：谁在什么时候改了什么。筛选条件写进地址栏，便于分享与回溯。 */

const PAGE_SIZE = 30

const route = useRoute()
const router = useRouter()

const COLUMNS: Column[] = [
  { label: '时间', width: '150px' },
  { label: '操作者' },
  { label: '操作' },
  { label: '对象', hideOnMobile: true },
  { label: '详情' },
  { label: 'IP', hideOnMobile: true },
  { label: '', actions: true, width: '90px' },
]

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const logs = ref<AuditLog[]>([])
const total = ref(0)
const page = ref(1)
const action = ref('')
const search = ref('')
const actor = ref('')
const actionOptions = ref<string[]>([])
const expanded = ref<number | null>(null)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(() => Boolean(action.value || search.value.trim() || actor.value))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAuditLogs({
      page: page.value,
      limit: PAGE_SIZE,
      action: action.value || undefined,
      q: search.value.trim() || undefined,
      actor: actor.value ? Number(actor.value) : undefined,
    } as never)
    logs.value = result.items
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载日志失败')
  } finally {
    loading.value = false
  }
}

async function loadActions() {
  actionOptions.value = await listAuditActions().catch(() => [])
}

function syncUrl() {
  const query: Record<string, string> = {}
  if (action.value) query.action = action.value
  if (search.value.trim()) query.q = search.value.trim()
  if (actor.value) query.actor = actor.value
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
  search.value = ''
  actor.value = ''
  apply()
}

async function download() {
  busy.value = true
  try {
    await exportAuditLogs({
      action: action.value || undefined,
      q: search.value.trim() || undefined,
      actor: actor.value ? Number(actor.value) : undefined,
    } as never)
  } catch (err) {
    error.value = errorMessage(err, '导出失败')
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  if (typeof route.query.action === 'string') action.value = route.query.action
  if (typeof route.query.q === 'string') search.value = route.query.q
  if (typeof route.query.actor === 'string') actor.value = route.query.actor
  if (typeof route.query.page === 'string') page.value = Math.max(1, Number(route.query.page) || 1)
  await loadActions()
  await load()
})
</script>

<template>
  <section class="space-y-4">
    <PageHeader title="操作日志" description="后台的每一次写操作都记录在这里：操作者、对象、详情与来源 IP。" bordered>
      <template #actions>
        <button class="btn btn-quiet btn-sm" :disabled="busy" @click="download">
          <AdminIcon name="download" :size="14" />
          {{ busy ? '导出中…' : '导出 CSV' }}
        </button>
        <button class="btn btn-quiet btn-sm" :disabled="loading" @click="load">
          <AdminIcon name="refresh" :size="14" />
          刷新
        </button>
      </template>
    </PageHeader>

    <FilterBar :count="total ? '共 ' + total + ' 条记录' : ''">
      <select v-model="action" class="input !w-auto" aria-label="操作类型" @change="apply">
        <option value="">全部操作</option>
        <option v-for="item in actionOptions" :key="item" :value="item">{{ item }}</option>
      </select>
      <input v-model="search" class="input w-56" type="search" placeholder="关键词：详情 / 对象" aria-label="搜索日志" @keyup.enter="apply" />
      <input v-model="actor" class="input !w-32 nums" type="number" placeholder="操作者 ID" aria-label="操作者 ID" @keyup.enter="apply" />
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
      <tr v-for="log in logs" :key="log.id">
        <td class="quiet mono text-[11.5px]">{{ when(log.created_at) }}</td>
        <td>
          <RouterLink v-if="log.actor_id" :to="'/users/' + log.actor_id" class="hover:accent-text">
            {{ log.actor_name || '#' + log.actor_id }}
          </RouterLink>
          <span v-else class="quiet">系统</span>
        </td>
        <td><span class="mono text-[12px]">{{ log.action }}</span></td>
        <td class="mono hide-on-mobile max-w-[200px] truncate text-[11.5px]">{{ log.target || '—' }}</td>
        <td class="max-w-[280px] truncate text-[12.5px]">{{ log.detail || '—' }}</td>
        <td class="mono quiet hide-on-mobile text-[11px]">{{ log.ip || '—' }}</td>
        <td class="text-right">
          <button
            v-if="log.detail || log.target"
            class="btn btn-quiet btn-sm"
            @click="expanded = expanded === log.id ? null : log.id"
          >
            {{ expanded === log.id ? '收起' : '详情' }}
          </button>
        </td>
      </tr>
      <tr v-if="expanded !== null">
        <td colspan="7" class="bg-[var(--surface-sunken)]">
          <pre class="codebox max-h-64 overflow-auto text-[11.5px]">{{ logs.find((l) => l.id === expanded)?.detail || '无详情' }}</pre>
        </td>
      </tr>
    </DataTable>
  </section>
</template>
