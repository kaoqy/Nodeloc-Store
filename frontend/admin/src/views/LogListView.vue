<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listAuditLogs } from '../api/logs'
import type { AuditLog } from '../types'

const loading = ref(true)
const logs = ref<AuditLog[]>([])
const page = ref(1)
const perPage = ref(20)

async function load() {
  loading.value = true
  try {
    const result = await listAuditLogs({ page: page.value, per_page: perPage.value })
    logs.value = result.items
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <p class="text-sm text-[#6b6b80]">系统操作日志</p>

    <div class="table-container">
      <table>
        <thead>
          <tr><th>ID</th><th>操作</th><th>详情</th><th>操作者</th><th>IP</th><th>时间</th></tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="6"><div class="skeleton h-8" /></td></tr>
          <tr v-else-if="!logs.length"><td colspan="6" class="py-8 text-center text-[#6b6b80]">暂无日志</td></tr>
          <tr v-for="log in logs" :key="log.id">
            <td>{{ log.id }}</td>
            <td><span class="badge badge-neutral">{{ log.action }}</span></td>
            <td class="max-w-xs truncate text-[#a1a1b5]">{{ log.detail || '-' }}</td>
            <td>{{ log.user?.name || log.user?.email || '系统' }}</td>
            <td class="text-[#a1a1b5]">{{ log.ip || '-' }}</td>
            <td class="text-[#a1a1b5]">{{ log.created_at }}</td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">显示 {{ logs.length }} 条</p>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-sm text-[#a1a1b5]">第 {{ page }} 页</span>
        <button class="btn-secondary" @click="page++; load()">下一页</button>
      </div>
    </div>
  </section>
</template>
