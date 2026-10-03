<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { adjustPoints, grantTransfer, listUsers, toggleActive } from '../api/users'
import { errorMessage, money, roleMeta, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { User } from '../types'

/**
 * 用户管理。除了列表，这里承担两个高频动作：调整积分、直接向买家转账 NL。
 * 两者都在抽屉里完成，不离开列表。
 */

const PAGE_SIZE = 20

const auth = useAuthStore()
const canManageUsers = computed(() => auth.allows('users', 'manage'))

const COLUMNS: Column[] = [
  { label: '用户' },
  { label: '角色' },
  { label: '积分', numeric: true, hideOnMobile: true },
  { label: '绑定', hideOnMobile: true },
  { label: '状态' },
  { label: '注册时间', hideOnMobile: true },
  { label: '', actions: true, width: '190px' },
]

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const users = ref<User[]>([])
const total = ref(0)
const page = ref(1)
const search = ref('')
const role = ref('all')

const pointsTarget = ref<User | null>(null)
const pointsDelta = ref<number | ''>('')
const pointsNote = ref('')

const transferTarget = ref<User | null>(null)
const transferAmount = ref<number | ''>(10)
const transferNote = ref('')

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(() => Boolean(search.value.trim() || role.value !== 'all'))

// 角色只有两个：超级管理员负责配置与人员，管理员负责日常经营与客服。
// 历史角色名仍然会被渲染成对应标签，已有账号不会显示成空白。
const ROLES = [
  { value: 'all', label: '全部角色' },
  { value: 'user', label: '普通用户' },
  { value: 'admin', label: '管理员' },
  { value: 'super_admin', label: '超级管理员' },
]

const LEGACY_ROLE_LABELS: Record<string, string> = {
  operator: '管理员', support: '管理员', ops_manager: '管理员', product_manager: '管理员',
  order_manager: '管理员', finance: '管理员', support_lead: '管理员',
  support_agent: '管理员', ai_admin: '管理员', data_viewer: '管理员',
}

function roleLabel(value: string): string {
  return (
    ROLES.find((r) => r.value === value)?.label ??
    LEGACY_ROLE_LABELS[value] ??
    roleMeta(value)?.label ??
    value
  )
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listUsers({ page: page.value, limit: PAGE_SIZE, q: search.value.trim() || undefined, role: role.value === 'all' ? undefined : role.value } as never)
    users.value = result.data
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载用户失败')
  } finally {
    loading.value = false
  }
}

function apply() {
  page.value = 1
  void load()
}
function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}
function clearFilters() {
  search.value = ''
  role.value = 'all'
  apply()
}

async function toggle(user: User) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const next = await toggleActive(user.id)
    users.value = users.value.map((u) => (u.id === next.id ? next : u))
    notice.value = next.is_active ? '账号已启用。' : '账号已停用。'
  } catch (err) {
    error.value = errorMessage(err, '更新账号状态失败')
  } finally {
    busy.value = false
  }
}

async function submitPoints() {
  const delta = Number(pointsDelta.value)
  if (!pointsTarget.value || !Number.isInteger(delta) || delta === 0) {
    error.value = '请输入非零的整数积分。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const next = await adjustPoints(pointsTarget.value.id, delta, pointsNote.value.trim() || undefined)
    users.value = users.value.map((u) => (u.id === next.id ? next : u))
    notice.value = '已为 ' + next.username + ' 调整 ' + (delta > 0 ? '+' : '') + delta + ' 积分。'
    pointsTarget.value = null
    pointsDelta.value = ''
    pointsNote.value = ''
  } catch (err) {
    error.value = errorMessage(err, '调整积分失败')
  } finally {
    busy.value = false
  }
}

async function submitTransfer() {
  const amount = Number(transferAmount.value)
  if (!transferTarget.value || !Number.isInteger(amount) || amount < 1 || amount > 1000) {
    error.value = '转账金额需为 1 到 1000 的整数。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    const row = await grantTransfer(transferTarget.value.id, { amount, note: transferNote.value.trim() || undefined })
    notice.value = '已向 ' + transferTarget.value.username + ' 转出 ' + money(row.amount) + '（流水号 ' + row.reference + '）。'
    transferTarget.value = null
    transferNote.value = ''
    transferAmount.value = 10
  } catch (err) {
    error.value = errorMessage(err, '转账未能完成')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <PageHeader title="用户管理" description="账号状态、积分调整与直接转账都在这里。转账会把 NL 从商店的 NodeLoc 账户打给买家。" bordered />

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <FilterBar :count="total ? '共 ' + total + ' 位用户' : ''">
      <input v-model="search" class="input w-52" type="search" placeholder="用户名 / 邮箱 / 昵称" aria-label="搜索用户" @keyup.enter="apply" />
      <select v-model="role" class="input !w-auto" aria-label="角色" @change="apply">
        <option v-for="option in ROLES" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
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
      :summary="'共 ' + total + ' 位用户'"
      empty-title="没有符合条件用户"
      empty-hint="换个关键词或角色再试。"
      @retry="load"
      @clear-filters="clearFilters"
      @change="goPage"
    >
      <tr v-for="user in users" :key="user.id">
        <td>
          <div class="flex items-center gap-2.5">
            <span class="grid size-8 shrink-0 place-items-center overflow-hidden rounded-full bg-[var(--surface-hi)] text-[12px] font-bold">
              <img v-if="user.avatar_url || user.oauth_avatar" :src="user.avatar_url || user.oauth_avatar || ''" alt="" class="size-full object-cover" />
              <span v-else>{{ (user.nickname || user.username).slice(0, 1).toUpperCase() }}</span>
            </span>
            <span class="min-w-0">
              <RouterLink :to="'/users/' + user.id" class="block truncate font-semibold hover:accent-text">
                {{ user.nickname || user.username }}
              </RouterLink>
              <span class="quiet block truncate text-[11px]">{{ user.email || '未绑定邮箱' }}</span>
            </span>
          </div>
        </td>
        <td><span :class="user.role === 'user' ? 'badge-neutral' : 'badge-accent'">{{ roleLabel(user.role) }}</span></td>
        <td class="nums hide-on-mobile">{{ user.points }}</td>
        <td class="hide-on-mobile">
          <StatusBadge :value="user.oauth_uid ? 'ok' : 'draft'" :label="user.oauth_uid ? '已绑定' : '未绑定'" />
        </td>
        <td>
          <StatusBadge :value="user.is_active ? 'ok' : 'disabled'" :label="user.is_active ? '正常' : '已停用'" />
        </td>
        <td class="quiet hide-on-mobile text-xs">{{ when(user.created_at) }}</td>
        <td class="text-right">
          <div class="flex flex-wrap justify-end gap-1.5">
            <RouterLink :to="'/users/' + user.id" class="btn btn-quiet btn-sm">详情</RouterLink>
            <button v-if="canManageUsers" class="btn btn-quiet btn-sm" @click="pointsTarget = user; pointsDelta = ''">积分</button>
            <button v-if="canManageUsers" class="btn btn-quiet btn-sm" @click="transferTarget = user">转账</button>
            <button v-if="canManageUsers" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggle(user)">
              {{ user.is_active ? '停用' : '启用' }}
            </button>
          </div>
        </td>
      </tr>
    </DataTable>

    <!-- 调整积分 -->
    <AppDrawer :open="Boolean(pointsTarget)" :title="'调整积分 · ' + (pointsTarget?.username ?? '')" width="sm" @close="pointsTarget = null">
      <div class="space-y-3">
        <div>
          <label class="label" for="pt-delta">变动值（正数加分，负数扣分）</label>
          <input id="pt-delta" v-model.number="pointsDelta" class="input nums" type="number" placeholder="例如 100 或 -50" />
        </div>
        <div>
          <label class="label" for="pt-note">原因</label>
          <input id="pt-note" v-model="pointsNote" class="input" placeholder="会记入积分流水" />
        </div>
        <p class="quiet text-xs">当前积分：<span class="nums">{{ pointsTarget?.points ?? 0 }}</span></p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="pointsTarget = null">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="submitPoints">{{ busy ? '提交中…' : '确认调整' }}</button>
        </div>
      </template>
    </AppDrawer>

    <!-- 转账 -->
    <AppDrawer :open="Boolean(transferTarget)" :title="'转账 NL · ' + (transferTarget?.username ?? '')" width="sm" @close="transferTarget = null">
      <div class="space-y-3">
        <p class="quiet text-xs">
          从商店的 NodeLoc 应用直接转到买家的 NodeLoc 账户。收款方需要先用 NodeLoc 登录过一次。
        </p>
        <div>
          <label class="label" for="tf-amount">金额（1–1000 的整数）</label>
          <input id="tf-amount" v-model.number="transferAmount" class="input nums" type="number" min="1" max="1000" />
        </div>
        <div>
          <label class="label" for="tf-note">留言（可选）</label>
          <input id="tf-note" v-model="transferNote" class="input" />
        </div>
        <p v-if="transferTarget && !(transferTarget.oauth_uid || transferTarget.oauth_username)" class="alert alert-warning">
          这个账号还没有绑定 NodeLoc，请让对方先用 NodeLoc 登录一次。
        </p>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="transferTarget = null">取消</button>
          <button
            class="btn btn-primary btn-sm"
            :disabled="busy || !(transferTarget?.oauth_uid || transferTarget?.oauth_username)"
            @click="submitTransfer"
          >
            {{ busy ? '转账中…' : '确认转账' }}
          </button>
        </div>
      </template>
    </AppDrawer>
  </section>
</template>
