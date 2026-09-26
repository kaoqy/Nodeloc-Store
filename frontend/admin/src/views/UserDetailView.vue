<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { adjustPoints, getUser, setRole, toggleActive, toggleAdmin } from '../api/users'
import { errorMessage, when } from '../utils/format'
import type { User } from '../types'

const route = useRoute()
const router = useRouter()

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const user = ref<User | null>(null)
const delta = ref<number | ''>('')

const roleMeta: Record<string, { label: string; badge: string }> = {
  super_admin: { label: '超级管理员', badge: 'badge-danger' },
  admin: { label: '管理员', badge: 'badge-warning' },
  user: { label: '普通用户', badge: 'badge-neutral' },
}

const current = computed(() => roleMeta[user.value?.role || 'user'] || roleMeta.user)
const bound = computed(() => Boolean(user.value?.oauth_provider))
const points = computed(() => Number(delta.value || 0))

async function load() {
  loading.value = true
  error.value = ''
  const id = Number(route.params.id)
  if (!id) {
    error.value = '无效的用户 ID'
    loading.value = false
    return
  }
  try {
    user.value = await getUser(id)
  } catch (err) {
    user.value = null
    error.value = errorMessage(err, '用户加载失败')
  } finally {
    loading.value = false
  }
}

async function run(action: () => Promise<User>, message: string) {
  if (!user.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    user.value = await action()
    notice.value = message
  } catch (err) {
    error.value = errorMessage(err, '操作失败')
  } finally {
    busy.value = false
  }
}

async function submitPoints() {
  if (!user.value || !points.value) return
  await run(() => adjustPoints(user.value!.id, points.value as number), `已调整 ${points.value > 0 ? '+' : ''}${points.value} 积分`)
  delta.value = ''
}

function changeRole(event: Event) {
  const role = (event.target as HTMLSelectElement).value
  run(() => setRole(user.value!.id, role), '角色已更新')
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-48" />
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="skeleton h-56" />
      <div class="skeleton h-56 lg:col-span-2" />
    </div>
  </section>

  <section v-else-if="!user" class="card py-16 text-center">
    <p class="muted">{{ error || '用户不存在' }}</p>
    <button class="btn btn-secondary btn-sm mt-4" @click="router.push('/users')">返回用户列表</button>
  </section>

  <section v-else class="space-y-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <RouterLink to="/users" class="quiet text-xs hover:text-[var(--text)]">← 返回用户列表</RouterLink>
        <div class="mt-1.5 flex flex-wrap items-center gap-2.5">
          <h2 class="truncate text-xl font-bold">{{ user.username }}</h2>
          <span class="badge" :class="current.badge">{{ current.label }}</span>
          <span class="badge" :class="user.is_active ? 'badge-success' : 'badge-danger'">
            {{ user.is_active ? '正常' : '已禁用' }}
          </span>
        </div>
      </div>
      <button
        class="btn btn-sm"
        :class="user.is_active ? 'btn-danger' : 'btn-secondary'"
        :disabled="busy"
        @click="run(() => toggleActive(user!.id), user!.is_active ? '账号已禁用' : '账号已启用')"
      >
        {{ user.is_active ? '禁用账号' : '启用账号' }}
      </button>
    </div>

    <div v-if="error" class="alert alert-danger">{{ error }}</div>
    <div v-if="notice" class="alert alert-success">{{ notice }}</div>

    <div class="grid gap-5 lg:grid-cols-3">
      <div class="card text-center">
        <img
          v-if="user.oauth_avatar || user.avatar_url"
          :src="user.oauth_avatar || user.avatar_url"
          :alt="user.username"
          class="mx-auto mb-4 h-20 w-20 rounded-full border border-[var(--stroke)] object-cover"
        />
        <div
          v-else
          class="mx-auto mb-4 flex h-20 w-20 items-center justify-center rounded-full bg-[var(--accent-soft)] text-2xl font-bold accent-text"
        >
          {{ user.username.slice(0, 1).toUpperCase() }}
        </div>
        <h3 class="text-lg font-semibold">{{ user.nickname || user.username }}</h3>
        <p class="quiet mt-1 text-sm mono">#{{ user.id }}</p>
        <p class="quiet mt-0.5 text-sm">{{ user.email || '未设置邮箱' }}</p>
        <div class="mt-5 grid grid-cols-2 gap-3 text-left">
          <div class="card-quiet !p-3">
            <p class="eyebrow">积分</p>
            <p class="nums mt-1 text-lg font-semibold">{{ user.points }}</p>
          </div>
          <div class="card-quiet !p-3">
            <p class="eyebrow">连续签到</p>
            <p class="nums mt-1 text-lg font-semibold">{{ user.consecutive_days || 0 }} 天</p>
          </div>
        </div>
      </div>

      <div class="space-y-5 lg:col-span-2">
        <div class="card">
          <h3 class="mb-4 text-sm font-semibold">账号信息</h3>
          <dl class="grid gap-x-6 gap-y-3 sm:grid-cols-2">
            <div class="flex justify-between gap-3 border-b border-[var(--stroke-quiet)] pb-2">
              <dt class="quiet">注册时间</dt>
              <dd class="text-sm">{{ when(user.created_at) }}</dd>
            </div>
            <div class="flex justify-between gap-3 border-b border-[var(--stroke-quiet)] pb-2">
              <dt class="quiet">最后登录</dt>
              <dd class="text-sm">{{ user.last_login_at ? when(user.last_login_at) : '从未登录' }}</dd>
            </div>
            <div class="flex justify-between gap-3 border-b border-[var(--stroke-quiet)] pb-2">
              <dt class="quiet">NodeLoc</dt>
              <dd class="text-sm">
                <span v-if="bound" class="badge badge-accent">{{ user.oauth_username || user.oauth_name || user.oauth_provider }}</span>
                <span v-else class="quiet">未绑定</span>
              </dd>
            </div>
            <div class="flex justify-between gap-3 border-b border-[var(--stroke-quiet)] pb-2">
              <dt class="quiet">信任等级</dt>
              <dd class="nums text-sm">{{ user.oauth_trust_level ?? '—' }}</dd>
            </div>
          </dl>
        </div>

        <div class="card">
          <h3 class="mb-1 text-sm font-semibold">权限</h3>
          <p class="quiet mb-4 text-xs">角色决定管理端可见范围；不能修改自己的角色。</p>
          <div class="flex flex-wrap items-end gap-3">
            <div>
              <label class="label" for="role">角色</label>
              <select id="role" class="input w-40" :value="user.role" :disabled="busy" @change="changeRole">
                <option value="user">普通用户</option>
                <option value="admin">管理员</option>
                <option value="super_admin">超级管理员</option>
              </select>
            </div>
            <button
              class="btn btn-secondary btn-sm"
              :disabled="busy"
              @click="run(() => toggleAdmin(user!.id), '管理员标记已更新')"
            >
              {{ user.is_admin ? '取消管理员' : '设为管理员' }}
            </button>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-1 text-sm font-semibold">积分调账</h3>
          <p class="quiet mb-4 text-xs">当前余额 {{ user.points }} 分。扣减不能低于 0。</p>
          <div class="flex flex-wrap items-end gap-3">
            <div>
              <label class="label" for="delta">调整数量</label>
              <input id="delta" v-model="delta" type="number" class="input nums w-32" placeholder="正数加 / 负数扣" />
            </div>
            <button class="btn btn-primary btn-sm" :disabled="busy || !points" @click="submitPoints">
              {{ busy ? '处理中…' : '确认调账' }}
            </button>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
