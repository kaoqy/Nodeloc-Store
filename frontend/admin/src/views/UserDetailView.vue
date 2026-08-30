<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getUser, toggleActive, adjustPoints } from '../api/users'
import type { User } from '../types'

const route = useRoute()
const router = useRouter()
const loading = ref(true)
const user = ref<User | null>(null)
const error = ref('')

const pointsForm = reactive({ delta: 0, reason: '' })
const pointsLoading = ref(false)
const pointsMessage = ref('')

const roleText: Record<string, string> = {
  super_admin: '超级管理员',
  admin: '管理员',
  operator: '运营',
  support: '客服',
  user: '普通用户',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const id = Number(route.params.id)
    if (!id) {
      error.value = '无效的用户 ID'
      return
    }
    const result = await getUser(id)
    user.value = result.user
  } catch (err: any) {
    error.value = err.response?.data?.message || '用户加载失败'
  } finally {
    loading.value = false
  }
}

async function toggleUserActive() {
  if (!user.value) return
  try {
    const result = await toggleActive(user.value.id)
    user.value = result.user
  } catch {
    // silent
  }
}

async function submitPoints() {
  if (!user.value || !pointsForm.delta) return
  pointsLoading.value = true
  pointsMessage.value = ''
  try {
    const result = await adjustPoints(user.value.id, pointsForm.delta, pointsForm.reason)
    user.value = result.user
    pointsForm.delta = 0
    pointsForm.reason = ''
    pointsMessage.value = '积分调整成功'
  } catch {
    pointsMessage.value = '积分调整失败'
  } finally {
    pointsLoading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-48" />
    <div class="grid gap-4 sm:grid-cols-2">
      <div v-for="i in 4" :key="i" class="skeleton h-24" />
    </div>
  </section>

  <section v-else-if="error" class="py-20 text-center">
    <p class="text-[#ef4444]">{{ error }}</p>
    <button class="btn-secondary mt-4" @click="router.push('/users')">返回用户列表</button>
  </section>

  <section v-else-if="user" class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <RouterLink to="/users" class="text-sm text-[#6b6b80] hover:text-white">← 返回用户列表</RouterLink>
        <h2 class="mt-2 text-xl font-bold">用户详情</h2>
      </div>
      <button
        :class="['btn', user.is_active ? 'btn-danger' : 'btn-primary']"
        @click="toggleUserActive"
      >
        {{ user.is_active ? '禁用账号' : '启用账号' }}
      </button>
    </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <!-- Profile Card -->
      <div class="card text-center">
        <div class="mx-auto mb-4 flex h-20 w-20 items-center justify-center rounded-full bg-gradient-to-br from-indigo-500/30 to-purple-500/30 text-2xl font-bold text-indigo-300">
          {{ user.username?.[0]?.toUpperCase() || 'U' }}
        </div>
        <h3 class="text-lg font-semibold">{{ user.username }}</h3>
        <p class="mt-1 text-sm text-[#6b6b80]">{{ user.email || '未设置邮箱' }}</p>
        <div class="mt-3">
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
        </div>
      </div>

      <!-- Details -->
      <div class="space-y-4 lg:col-span-2">
        <div class="card">
          <h3 class="mb-4 font-semibold">基本信息</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <p class="text-xs text-[#6b6b80]">用户 ID</p>
              <p class="mt-1 text-sm font-medium">{{ user.id }}</p>
            </div>
            <div>
              <p class="text-xs text-[#6b6b80]">积分余额</p>
              <p class="mt-1 text-sm font-medium text-[#f59e0b]">{{ user.points }}</p>
            </div>
            <div>
              <p class="text-xs text-[#6b6b80]">注册时间</p>
              <p class="mt-1 text-sm font-medium">{{ user.created_at || '-' }}</p>
            </div>
            <div>
              <p class="text-xs text-[#6b6b80]">最后登录</p>
              <p class="mt-1 text-sm font-medium">{{ user.last_login_at || '-' }}</p>
            </div>
            <div>
              <p class="text-xs text-[#6b6b80]">连续签到</p>
              <p class="mt-1 text-sm font-medium">{{ user.consecutive_days || 0 }} 天</p>
            </div>
            <div>
              <p class="text-xs text-[#6b6b80]">累计签到</p>
              <p class="mt-1 text-sm font-medium">{{ user.total_checkins || 0 }} 次</p>
            </div>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">账号状态</h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-sm">OAuth 绑定</span>
              <span :class="['badge', user.oauth_bound ? 'badge-success' : 'badge-neutral']">
                {{ user.oauth_bound ? '已绑定' : '未绑定' }}
              </span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-sm">管理员</span>
              <span :class="['badge', user.is_admin ? 'badge-warning' : 'badge-neutral']">
                {{ user.is_admin ? '是' : '否' }}
              </span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-sm">账号状态</span>
              <span :class="['badge', user.is_active ? 'badge-success' : 'badge-danger']">
                {{ user.is_active ? '正常' : '已禁用' }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Points Adjustment -->
    <div class="card">
      <h3 class="mb-4 font-semibold">积分调账</h3>
      <div class="flex flex-wrap items-end gap-4">
        <div>
          <label class="mb-1 block text-sm text-[#a1a1b5]">调整数量</label>
          <input v-model.number="pointsForm.delta" type="number" class="input w-32" placeholder="正数加 / 负数扣" />
        </div>
        <div class="flex-1">
          <label class="mb-1 block text-sm text-[#a1a1b5]">原因</label>
          <input v-model="pointsForm.reason" class="input" placeholder="调账原因..." />
        </div>
        <button class="btn-primary" :disabled="pointsLoading || !pointsForm.delta" @click="submitPoints">
          {{ pointsLoading ? '处理中...' : '确认调账' }}
        </button>
      </div>
      <p v-if="pointsMessage" class="mt-3 text-sm" :class="pointsMessage.includes('成功') ? 'text-[#22c55e]' : 'text-[#ef4444]'">
        {{ pointsMessage }}
      </p>
    </div>
  </section>
</template>
