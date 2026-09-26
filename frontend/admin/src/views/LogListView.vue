<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { listAuditLogs } from '../api/logs'
import { errorMessage, when } from '../utils/format'
import type { AuditLog } from '../types'

const Limit = 25

const loading = ref(true)
const error = ref('')
const logs = ref<AuditLog[]>([])
const page = ref(1)
const totalPages = ref(1)
const total = ref(0)
const actionFilter = ref('')

const range = computed(() => ({ first: (page.value - 1) * Limit + 1, last: (page.value - 1) * Limit + logs.value.length }))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listAuditLogs({
      page: page.value,
      limit: Limit,
      action: actionFilter.value.trim() || undefined,
    })
    logs.value = result.items
    total.value = result.total
    totalPages.value = result.total_pages || 1
  } catch (err) {
    error.value = errorMessage(err, '加载日志失败')
  } finally {
    loading.value = false
  }
}

function go(next: number) {
  page.value = Math.min(Math.max(next, 1), totalPages.value)
  load()
}

function apply() {
  page.value = 1
  load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="actionFilter"
          class="input w-56"
          placeholder="按操作过滤，如 order.create"
          @keyup.enter="apply"
        />
        <button class="btn btn-secondary btn-sm" @click="apply">筛选</button>
      </div>
      <p class="quiet text-xs mono">共 {{ total }} 条</p>
    </div>

    <div v-if="error" class="alert alert-danger">{{ error }}</div>

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
          <tr v-if="loading">
            <td colspan="7"><div class="skeleton h-9" /></td>
          </tr>
          <tr v-else-if="!logs.length">
            <td colspan="7" class="py-12 text-center text-sm quiet">暂无日志</td>
          </tr>
          <tr v-for="log in logs" :key="log.id">
            <td class="nums text-sm quiet">{{ log.id }}</td>
            <td><span class="badge badge-neutral mono">{{ log.action }}</span></td>
            <td class="max-w-[180px] truncate text-sm muted">{{ log.target || '—' }}</td>
            <td class="max-w-[280px] truncate text-sm quiet">{{ log.detail || '—' }}</td>
            <td class="nums text-sm">{{ log.actor_id ? `#${log.actor_id}` : '系统' }}</td>
            <td class="mono text-sm quiet">{{ log.ip || '—' }}</td>
            <td class="whitespace-nowrap text-sm quiet">{{ when(log.created_at) }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <p class="quiet text-xs mono">第 {{ range.first }}–{{ range.last }} 条 · 共 {{ total }} 条</p>
      <div class="flex items-center gap-2">
        <button class="btn btn-secondary btn-sm" :disabled="page <= 1 || loading" @click="go(page - 1)">上一页</button>
        <span class="text-sm muted mono">{{ page }} / {{ totalPages }}</span>
        <button class="btn btn-secondary btn-sm" :disabled="page >= totalPages || loading" @click="go(page + 1)">下一页</button>
      </div>
    </div>
  </section>
</template>
