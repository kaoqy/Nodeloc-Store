<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { adjustPoints, getUser, grantTransfer, listUserTransfers, setRole, toggleActive, toggleAdmin } from '../api/users'
import { errorMessage, money, roleMeta, transferStatus, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Transfer, User } from '../types'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const user = ref<User | null>(null)
const delta = ref<number | ''>('')

const GrantMax = 1000
const grantAmount = ref<number | ''>(10)
const grantNote = ref('')
const granting = ref(false)
const transfers = ref<Transfer[]>([])
const transferTotal = ref(0)

const current = computed(() => roleMeta(user.value?.role))
const bound = computed(() => Boolean(user.value?.oauth_provider))
const points = computed(() => Number(delta.value || 0))
// 转账 needs more than a bound provider: the money goes to a NodeLoc uid, and an
// account that predates the binding has nowhere to arrive.
const nodeLocBound = computed(
  () => Boolean((user.value?.oauth_uid || '').trim() || (user.value?.oauth_username || '').trim()),
)
const grantAmountValid = computed(
  () => Number.isInteger(Number(grantAmount.value)) && Number(grantAmount.value) >= 1 && Number(grantAmount.value) <= GrantMax,
)

const canManageUsers = computed(() => auth.allows('users', 'manage'))
const canManageRoles = computed(() => auth.allows('roles', 'manage'))
const canViewLogs = computed(() => auth.allows('logs', 'view'))
const isSelf = computed(() => Boolean(user.value && auth.user && user.value.id === auth.user.id))
// The API refuses to let anyone but a super_admin touch a back-office account,
// or grant 管理员 at all; the picker says so instead of failing on submit.
const staffTarget = computed(() => {
  const role = user.value?.role ?? 'user'
  return role !== 'user'
})
const canChangeRole = computed(() => {
  if (!canManageRoles.value || isSelf.value || !user.value) return false
  return auth.isSuperAdmin || !staffTarget.value
})

// ASSIGNABLE lists what this account may hand out, in role order.
const ASSIGNABLE = [
  { role: 'user', label: '普通用户', superOnly: false },
  { role: 'support', label: '客服', superOnly: false },
  { role: 'operator', label: '运营', superOnly: false },
  { role: 'admin', label: '管理员', superOnly: true },
  { role: 'super_admin', label: '超级管理员', superOnly: true },
]

const assignable = computed(() => ASSIGNABLE.filter((item) => !item.superOnly || auth.isSuperAdmin))

// The picker is disabled for two different reasons, and telling them apart is
// what stops the owner hunting for a permission they really do hold.
const roleHint = computed(() => {
  if (canChangeRole.value) return ''
  if (isSelf.value) return '这是你自己的账号，改自己的角色要由另一位超级管理员来操作。'
  return '当前账号无权调整这名成员的角色。'
})

// The picker must still show a role this account cannot grant, otherwise it
// would look as though the member held the first option.
const roleMissing = computed(
  () => Boolean(user.value) && !assignable.value.some((option) => option.role === user.value?.role),
)

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
    loadTransfers(id)
  } catch (err) {
    user.value = null
    error.value = errorMessage(err, '用户加载失败')
  } finally {
    loading.value = false
  }
}

// The ledger read is not allowed to fail the page: an owner who cannot see the
// history still has the account in front of them, and vice versa.
async function loadTransfers(id: number) {
  try {
    const result = await listUserTransfers(id, { limit: 8 })
    transfers.value = result.data
    transferTotal.value = result.total
  } catch {
    transfers.value = []
    transferTotal.value = 0
  }
}

async function submitGrant() {
  if (!user.value || granting.value || !grantAmountValid.value) return
  granting.value = true
  error.value = ''
  notice.value = ''
  const target = user.value.id
  try {
    const row = await grantTransfer(target, {
      amount: Number(grantAmount.value),
      note: grantNote.value.trim() || undefined,
    })
    notice.value = `已向 ${user.value.username} 的 NodeLoc 账户转出 ${money(row.amount)}（流水号 ${row.reference}）`
    grantNote.value = ''
    grantAmount.value = 10
  } catch (err) {
    // NodeLoc's own reason is what makes a refusal fixable, so it is shown as the
    // server worded it.
    error.value = errorMessage(err, '转账未能完成')
  } finally {
    granting.value = false
    await loadTransfers(target)
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
  if (!canChangeRole.value) return
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
        <div class="flex flex-wrap items-center gap-3">
          <RouterLink to="/users" class="quiet text-xs hover:text-[var(--text)]">← 返回用户列表</RouterLink>
          <RouterLink
            v-if="canViewLogs"
            :to="`/logs?actor=${user.id}`"
            class="quiet text-xs hover:text-[var(--text)]"
            title="只看这名成员在后台留下的操作记录"
          >
            查看其操作记录 →
          </RouterLink>
        </div>
        <div class="mt-1.5 flex flex-wrap items-center gap-2.5">
          <h2 class="truncate text-xl font-bold">{{ user.username }}</h2>
          <span class="badge" :class="current.badge">{{ current.label }}</span>
          <span class="badge" :class="user.is_active ? 'badge-success' : 'badge-danger'">
            {{ user.is_active ? '正常' : '已禁用' }}
          </span>
        </div>
      </div>
      <button
        v-if="canManageUsers"
        class="btn btn-sm"
        :class="user.is_active ? 'btn-danger' : 'btn-secondary'"
        :disabled="busy"
        @click="run(() => toggleActive(user!.id), user!.is_active ? '账号已禁用' : '账号已启用')"
      >
        {{ user.is_active ? '禁用账号' : '启用账号' }}
      </button>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <div v-if="notice" class="alert alert-success" role="status">{{ notice }}</div>

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
        <h3 class="break-words text-lg font-semibold">{{ user.nickname || user.username }}</h3>
        <p class="quiet mt-1 text-sm mono">#{{ user.id }}</p>
        <p class="quiet mt-0.5 break-words text-sm">{{ user.email || '未设置邮箱' }}</p>
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

        <div v-if="canManageRoles" class="card">
          <h3 class="mb-1 text-sm font-semibold">权限</h3>
          <p class="quiet mb-4 text-xs">
            角色决定这个账号在后台能看到哪些页面，具体能做什么由 角色权限 里的细分项决定。{{ roleHint }}
          </p>
          <div class="flex flex-wrap items-end gap-3">
            <div>
              <label class="label" for="role">角色</label>
              <select
                id="role"
                class="input w-40"
                :value="user.role"
                :disabled="busy || !canChangeRole"
                @change="changeRole"
              >
                <option v-for="option in assignable" :key="option.role" :value="option.role">
                  {{ option.label }}
                </option>
                <option v-if="roleMissing" :value="user.role" disabled>
                  {{ current.label }}
                </option>
              </select>
            </div>
            <button
              v-if="!staffTarget"
              class="btn btn-secondary btn-sm"
              :disabled="busy || isSelf || !auth.isSuperAdmin"
              @click="run(() => toggleAdmin(user!.id), '管理员标记已更新')"
            >
              {{ user.is_admin ? '取消管理员' : '设为管理员' }}
            </button>
          </div>
        </div>

        <div v-if="canManageUsers" class="card">
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

        <div class="card">
          <h3 class="mb-1 text-sm font-semibold">转账 NL</h3>
          <p class="quiet mb-4 text-xs">
            <template v-if="canManageUsers">
              通过 NodeLoc 把积分转到这位买家的论坛账户，单次 1–{{ GrantMax }} NL 的整数。这不是商店积分，转出后商店无法自行撤回。
            </template>
            <template v-else>这名账号的转账流水。当前角色只能查看，转出需要 users:manage。</template>
          </p>
          <p v-if="canManageUsers && !nodeLocBound" class="alert alert-warning mb-4">
            这个账号还没有绑定 NodeLoc，积分没有可转入的账户。请让对方先在个人中心用 NodeLoc 登录一次。
          </p>
          <div v-if="canManageUsers" class="flex flex-wrap items-end gap-3">
            <div>
              <label class="label" for="grant-amount">金额（NL）</label>
              <input
                id="grant-amount"
                v-model.number="grantAmount"
                type="number"
                class="input nums w-32"
                min="1"
                :max="GrantMax"
                step="1"
                :disabled="granting"
              />
            </div>
            <div class="min-w-40 flex-1">
              <label class="label" for="grant-note">留言（可选，买家可见）</label>
              <input id="grant-note" v-model="grantNote" class="input" maxlength="200" placeholder="例如：补差价" :disabled="granting" />
            </div>
            <button
              class="btn btn-primary btn-sm"
              :disabled="granting || !nodeLocBound || !grantAmountValid"
              @click="submitGrant"
            >
              {{ granting ? '转账中…' : '确认转出' }}
            </button>
          </div>

          <div class="mt-5">
            <p class="quiet text-xs">最近转账<template v-if="transferTotal"> · 共 {{ transferTotal }} 笔</template></p>
            <div v-if="transfers.length" class="mt-2 space-y-1.5">
              <div
                v-for="row in transfers"
                :key="row.id"
                class="flex flex-wrap items-center justify-between gap-2 border-b border-[var(--stroke-quiet)] pb-1.5 text-sm"
              >
                <span class="nums font-medium">{{ money(row.amount) }}</span>
                <span class="badge" :class="transferStatus(row.status).badge">{{ transferStatus(row.status).label }}</span>
                <span class="mono quiet flex-1 truncate text-xs" :title="row.note || row.detail || row.reference">
                  {{ row.note || row.detail || row.reference }}
                </span>
                <span class="quiet text-xs whitespace-nowrap">{{ when(row.created_at) }}</span>
                <span v-if="row.operator_name" class="quiet text-xs whitespace-nowrap">by {{ row.operator_name }}</span>
              </div>
            </div>
            <p v-else class="quiet mt-2 text-xs">还没有向这个账号转过账。</p>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
