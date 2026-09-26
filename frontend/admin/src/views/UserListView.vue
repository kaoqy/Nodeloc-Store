<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import { listUsers, toggleActive } from '../api/users'
import { errorMessage, when } from '../utils/format'
import type { User } from '../types'

const PageSize = 20

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const users = ref<User[]>([])
const total = ref(0)
const offset = ref(0)
const search = ref('')

const page = computed(() => Math.floor(offset.value / PageSize) + 1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))
const from = computed(() => (users.value.length ? offset.value + 1 : 0))
const to = computed(() => offset.value + users.value.length)

const roleMeta: Record<string, { label: string; badge: string }> = {
  super_admin: { label: '超级管理员', badge: 'badge-danger' },
  admin: { label: '管理员', badge: 'badge-warning' },
  user: { label: '普通用户', badge: 'badge-neutral' },
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listUsers({
      limit: PageSize,
      offset: offset.value,
      q: search.value.trim() || undefined,
    })
    users.value = result.data
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载用户失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  offset.value = 0
  load()
}

function goTo(target: number) {
  const last = Math.max(0, Math.ceil(total.value / PageSize) - 1)
  offset.value = Math.min(last, Math.max(0, target - 1)) * PageSize
  load()
}

async function toggle(user: User) {
  busy.value = true
  error.value = ''
  try {
    const next = await toggleActive(user.id)
    users.value = users.value.map((item) => (item.id === next.id ? next : item))
  } catch (err) {
    error.value = errorMessage(err, '更新用户状态失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div class="flex flex-wrap items-center gap-2">
        <input
          v-model="search"
          class="input w-64"
          type="search"
          placeholder="用户名 / 邮箱 / 昵称"
          aria-label="搜索用户"
          @keyup.enter="applyFilters"
        />
        <button class="btn btn-secondary btn-sm" @click="applyFilters">筛选</button>
      </div>
      <p class="quiet text-xs mono">共 {{ total }} 位用户</p>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>用户</th>
            <th>邮箱</th>
            <th>NodeLoc</th>
            <th>角色</th>
            <th>积分</th>
            <th>状态</th>
            <th>注册时间</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 5" :key="`skeleton-${i}`">
              <td colspan="8"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!users.length">
            <td colspan="8">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">{{ search.trim() ? '没有符合条件的用户' : '还没有用户' }}</p>
                <p class="empty-hint">{{ search.trim() ? '换个关键词或清空筛选再试。' : '用户在商店前台注册后会出现在这里。' }}</p>
              </div>
            </td>
          </tr>
          <tr v-for="user in users" :key="user.id">
            <td>
              <div class="flex min-w-0 items-center gap-3">
                <img
                  v-if="user.oauth_avatar || user.avatar_url"
                  :src="user.oauth_avatar || user.avatar_url"
                  :alt="user.username"
                  class="h-9 w-9 shrink-0 rounded-full border border-[var(--stroke)] object-cover"
                />
                <span
                  v-else
                  class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-[var(--accent-soft)] text-sm font-semibold accent-text"
                >
                  {{ user.username.slice(0, 1).toUpperCase() }}
                </span>
                <div class="min-w-0">
                  <RouterLink :to="`/users/${user.id}`" class="block truncate text-sm font-medium hover:text-[var(--accent)]">
                    {{ user.username }}
                  </RouterLink>
                  <p class="quiet text-xs mono">#{{ user.id }}</p>
                </div>
              </div>
            </td>
            <td class="max-w-[200px] truncate text-sm muted">{{ user.email || '—' }}</td>
            <td class="text-sm">
              <span v-if="user.oauth_provider" class="badge badge-accent">{{ user.oauth_username || user.oauth_provider }}</span>
              <span v-else class="quiet text-xs">未绑定</span>
            </td>
            <td>
              <span class="badge" :class="(roleMeta[user.role] || roleMeta.user).badge">
                {{ (roleMeta[user.role] || roleMeta.user).label }}
              </span>
            </td>
            <td class="nums text-sm">{{ user.points }}</td>
            <td>
              <span class="badge" :class="user.is_active ? 'badge-success' : 'badge-neutral'">
                {{ user.is_active ? '正常' : '已禁用' }}
              </span>
            </td>
            <td class="text-sm quiet">{{ when(user.created_at) }}</td>
            <td class="text-right whitespace-nowrap">
              <RouterLink :to="`/users/${user.id}`" class="btn btn-ghost btn-sm">详情</RouterLink>
              <button class="btn btn-ghost btn-sm" :disabled="busy" @click="toggle(user)">
                {{ user.is_active ? '禁用' : '启用' }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <PaginationFooter
      :page="page"
      :pages="pages"
      :loading="loading"
      :summary="`第 ${from}–${to} 条 · 共 ${total} 条`"
      @change="goTo"
    />
  </section>
</template>
