<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PaginationFooter from '../components/PaginationFooter.vue'
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
          type="search"
          placeholder="按操作过滤，如 order.create"
          aria-label="按操作类型过滤日志"
          @keyup.enter="apply"
        />
        <button class="btn btn-secondary btn-sm" @click="apply">筛选</button>
      </div>
      <p class="quiet text-xs mono">共 {{ total }} 条</p>
    </div>

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
                <p class="empty-title">{{ actionFilter.trim() ? '没有匹配的日志' : '暂无操作日志' }}</p>
                <p class="empty-hint">
                  {{ actionFilter.trim() ? '换一个操作名试试。' : '后台的每一次写入都会记录在这里。' }}
                </p>
              </div>
            </td>
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

    <PaginationFooter
      :page="page"
      :pages="totalPages"
      :loading="loading"
      :summary="`第 ${range.first}–${range.last} 条 · 共 ${total} 条`"
      @change="go"
    />
  </section>
</template>
