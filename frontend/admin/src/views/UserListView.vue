<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listUsers } from '../api/users'
import type { User } from '../types'

const loading = ref(true)
const users = ref<User[]>([])
const search = ref('')
const page = ref(1)
const perPage = ref(10)

const roleText: Record<string, string> = {
  super_admin: '超级管理员',
  admin: '管理员',
  operator: '运营',
  support: '客服',
  user: '普通用户',
}

async function load() {
  loading.value = true
  try {
    const params: any = { page: page.value, per_page: perPage.value }
    if (search.value) params.q = search.value
    const result = await listUsers(params)
    users.value = result.data
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <input v-model="search" class="input w-64" placeholder="搜索用户..." @keyup.enter="load" />
        <button class="btn-secondary" @click="load">筛选</button>
      </div>
      <button class="btn-primary">+ 新建用户</button>
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>ID</th>
            <th>用户名</th>
            <th>邮箱</th>
            <th>角色</th>
            <th>状态</th>
            <th>注册时间</th>
            <th>操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-if="loading">
            <td colspan="7">
              <div class="space-y-2">
                <div v-for="i in 5" :key="i" class="skeleton h-8" />
              </div>
            </td>
          </tr>
          <tr v-else-if="!users.length">
            <td colspan="7" class="py-12 text-center text-[#6b6b80]">暂无用户</td>
          </tr>
          <tr v-for="user in users" :key="user.id">
            <td>{{ user.id }}</td>
            <td>
              <div class="flex items-center gap-3">
                <div class="flex h-9 w-9 items-center justify-center rounded-full bg-gradient-to-br from-indigo-500/30 to-purple-500/30 text-sm font-medium text-indigo-300">
                  {{ user.username[0].toUpperCase() }}
                </div>
                <span class="font-medium">{{ user.username }}</span>
              </div>
            </td>
            <td class="text-[#a1a1b5]">{{ user.email || '-' }}</td>
            <td>
              <span
                :class="[
                  'badge',
                  user.role === 'super_admin' ? 'badge-danger' :
                  user.role === 'admin' ? 'badge-warning' :
                  user.role === 'operator' ? 'badge-info' : 'badge-neutral'
                ]"
              >
                {{ roleText[user.role] || user.role }}
              </span>
            </td>
            <td>
              <span
                :class="[
                  'badge',
                  user.is_active ? 'badge-success' : 'badge-neutral'
                ]"
              >
                {{ user.is_active ? '正常' : '禁用' }}
              </span>
            </td>
            <td class="text-[#a1a1b5]">{{ user.created_at }}</td>
            <td>
              <div class="flex items-center gap-2">
                <RouterLink :to="`/users/${user.id}`" class="btn-ghost text-xs">详情</RouterLink>
                <button class="btn-ghost text-xs">编辑</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">显示 {{ users.length }} 条</p>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-sm text-[#a1a1b5]">第 {{ page }} 页</span>
        <button class="btn-secondary" @click="page++; load()">下一页</button>
      </div>
    </div>
  </section>
</template>
