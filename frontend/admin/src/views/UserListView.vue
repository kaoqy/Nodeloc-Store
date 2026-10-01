<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import { grantTransfer, listTransfers, listUserTransfers, listUsers, toggleActive } from '../api/users'
import { errorMessage, money, roleMeta, transferStatus, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { closeOnEscape } from '../utils/dialog'
import type { Transfer, User } from '../types'

const PageSize = 20
// The shop's own ceiling, kept in step with the server's; NodeLoc's application
// limits are stricter and are reported by the provider when they bite.
const GrantMax = 1000

const auth = useAuthStore()
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const users = ref<User[]>([])
const total = ref(0)
const offset = ref(0)
const search = ref('')

const canManage = computed(() => auth.allows('users', 'manage'))

const page = computed(() => Math.floor(offset.value / PageSize) + 1)
const pages = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))
const from = computed(() => (users.value.length ? offset.value + 1 : 0))
const to = computed(() => offset.value + users.value.length)
// 一行都没有时不报「第 0–0 条」那种范围，只说总数。
const summary = computed(() =>
  users.value.length ? `第 ${from.value}–${to.value} 条 · 共 ${total.value} 条` : `共 ${total.value} 条`,
)

const transferTarget = ref<User | null>(null)
const transferHistory = ref<Transfer[]>([])
const transferTotal = ref(0)
const amount = ref(10)
const note = ref('')
const sending = ref(false)
const transferError = ref('')
const sent = ref<Transfer | null>(null)
const ledgerOpen = ref(false)
closeOnEscape(transferTarget, null)
closeOnEscape(ledgerOpen, false)
const ledger = ref<Transfer[]>([])
const ledgerTotal = ref(0)
const ledgerLoading = ref(false)
const ledgerError = ref('')

// 转账 needs a NodeLoc account: the money moves at the provider, and a shop
// account with no bound forum user has nowhere to send it.
const recipientBound = computed(() => {
  const target = transferTarget.value
  if (!target) return false
  return Boolean((target.oauth_uid || '').trim() || (target.oauth_username || '').trim())
})
const amountValid = computed(() => Number.isInteger(amount.value) && amount.value >= 1 && amount.value <= GrantMax)

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

function openTransfer(user: User) {
  transferTarget.value = user
  amount.value = 10
  note.value = ''
  transferError.value = ''
  sent.value = null
  transferHistory.value = []
  transferTotal.value = 0
  // Past transfers are context, not a gate: an owner about to pay somebody twice
  // should see it, but a slow read must not hold the form up.
  listUserTransfers(user.id, { limit: 5 })
    .then((result) => {
      transferHistory.value = result.data
      transferTotal.value = result.total
    })
    .catch(() => {})
}

async function submitTransfer() {
  const target = transferTarget.value
  if (!target || sending.value) return
  sending.value = true
  transferError.value = ''
  try {
    const row = await grantTransfer(target.id, { amount: amount.value, note: note.value.trim() || undefined })
    sent.value = row
    transferHistory.value = [row, ...transferHistory.value].slice(0, 5)
    transferTotal.value += 1
  } catch (err) {
    // NodeLoc's refusal comes back already worded in Chinese, with the reason the
    // shop has to fix: balance, limits, or a credential mismatch.
    transferError.value = errorMessage(err, '转账未能完成')
  } finally {
    sending.value = false
  }
}

// The whole shop's 转账流水, opened from the header. It reads fresh every time
// because a refusal is written to the ledger too: caching here would drop the
// rows the owner most wants to see after a failed attempt.
async function loadLedger() {
  ledgerLoading.value = true
  ledgerError.value = ''
  try {
    const result = await listTransfers({ limit: 50 })
    ledger.value = result.data
    ledgerTotal.value = result.total
  } catch (err) {
    ledgerError.value = errorMessage(err, '转账流水加载失败')
  } finally {
    ledgerLoading.value = false
  }
}

async function openLedger() {
  ledgerOpen.value = true
  await loadLedger()
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
      <div class="flex items-center gap-3">
        <p class="quiet text-xs mono">共 {{ total }} 位用户</p>
        <button class="btn btn-secondary btn-sm" title="查看店家向用户转出 NL 的流水" @click="openLedger">转账流水</button>
      </div>
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
              <span class="badge" :class="roleMeta(user.role).badge">{{ roleMeta(user.role).label }}</span>
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
              <button v-if="canManage" class="btn btn-ghost btn-sm" @click="openTransfer(user)">转账</button>
              <button v-if="canManage" class="btn btn-ghost btn-sm" :disabled="busy" @click="toggle(user)">
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
      :summary="summary"
      @change="goTo"
    />

    <!-- 转账 overlay -->
    <div
      v-if="transferTarget"
      class="overlay" role="dialog" aria-modal="true" aria-label="向用户转账"
      @click.self="transferTarget = null"
    >
      <div class="card w-full max-w-md !p-5">
        <h3 class="text-base font-semibold">转账 NL · {{ transferTarget.username }}</h3>
        <p class="quiet mt-1 text-xs">
          通过 NodeLoc 把积分转到这位买家的论坛账户。这不是商店积分，转出后商店无法自行撤回。
        </p>

        <p v-if="recipientBound" class="mt-3 text-xs mono muted">
          收款账户：{{ transferTarget.oauth_username || '（无用户名）' }}
          <span v-if="transferTarget.oauth_uid" class="quiet"> · uid {{ transferTarget.oauth_uid }}</span>
        </p>
        <p v-else class="alert alert-warning mt-3">
          这个账号还没有绑定 NodeLoc，积分没有可转入的账户。请让对方先在个人中心用 NodeLoc 登录一次。
        </p>

        <div class="mt-4 space-y-3">
          <div>
            <label class="label" for="t-amount">金额（NL）</label>
            <input
              id="t-amount"
              v-model.number="amount"
              class="input nums w-36"
              type="number"
              min="1"
              :max="GrantMax"
              step="1"
              :disabled="sending"
            />
            <p class="quiet mt-1 text-xs">单次 1–{{ GrantMax }} NL 的整数；NodeLoc 为支付应用另设的限额会更严格。</p>
          </div>
          <div>
            <label class="label" for="t-note">留言（可选，买家可见）</label>
            <input
              id="t-note"
              v-model="note"
              class="input"
              maxlength="200"
              placeholder="例如：补差价"
              :disabled="sending"
            />
          </div>
        </div>

        <p v-if="transferError" class="alert alert-danger mt-3" role="alert">{{ transferError }}</p>
        <!-- Only a landed 转账 answers with a row, so the status would repeat the
             「已到账」 this sentence already says. -->
        <p v-else-if="sent" class="alert alert-success mt-3">
          已向 {{ transferTarget.username }} 转出 {{ money(sent.amount) }} · 流水号 {{ sent.reference }}
        </p>

        <div v-if="transferHistory.length" class="mt-4 border-t border-[var(--stroke)] pt-3">
          <p class="quiet text-xs">最近转账 · 共 {{ transferTotal }} 笔</p>
          <ul class="mt-2 space-y-1">
            <li v-for="row in transferHistory" :key="row.id" class="flex items-center justify-between gap-2 text-xs">
              <span class="nums">{{ money(row.amount) }}</span>
              <span class="mono quiet truncate">{{ row.note || row.reference }}</span>
              <span class="badge" :class="transferStatus(row.status).badge">{{ transferStatus(row.status).label }}</span>
              <span class="quiet whitespace-nowrap">{{ when(row.created_at) }}</span>
            </li>
          </ul>
        </div>

        <div class="mt-5 flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="sending" @click="transferTarget = null">
            {{ sent ? '关闭' : '取消' }}
          </button>
          <button
            class="btn btn-primary btn-sm"
            :disabled="sending || !recipientBound || !amountValid"
            @click="submitTransfer"
          >
            {{ sending ? '转账中…' : `确认转出 ${amountValid ? amount : 0} NL` }}
          </button>
        </div>
      </div>
    </div>
    <!-- 转账流水 overlay -->
    <div
      v-if="ledgerOpen"
      class="overlay" role="dialog" aria-modal="true" aria-label="转账流水"
      @click.self="ledgerOpen = false"
    >
      <div class="card w-full max-w-3xl !p-5">
        <div class="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h3 class="text-base font-semibold">转账流水</h3>
            <p class="quiet mt-1 text-xs">
              店家通过 NodeLoc 转给买家的每一笔，含未成功的尝试。最近 {{ ledger.length }} 条 · 共 {{ ledgerTotal }} 条。
            </p>
          </div>
          <div class="flex gap-2">
            <button class="btn btn-secondary btn-sm" :disabled="ledgerLoading" @click="loadLedger">刷新</button>
            <button class="btn btn-ghost btn-sm" @click="ledgerOpen = false">关闭</button>
          </div>
        </div>

        <p v-if="ledgerError" class="alert alert-danger mt-3" role="alert">{{ ledgerError }}</p>

        <div class="table-container mt-4 max-h-[60vh] overflow-y-auto">
          <table>
            <thead>
              <tr>
                <th>时间</th>
                <th>收款人</th>
                <th class="text-right">金额</th>
                <th>状态</th>
                <th>流水号 / 留言</th>
                <th>操作者</th>
              </tr>
            </thead>
            <tbody>
              <tr v-if="ledgerLoading && !ledger.length">
                <td colspan="6"><div class="skeleton h-6" /></td>
              </tr>
              <tr v-else-if="!ledger.length">
                <td colspan="6">
                  <div class="empty-state">
                    <p class="empty-glyph" aria-hidden="true">◌</p>
                    <p class="empty-title">还没有转账记录</p>
                    <p class="empty-hint">在用户列表里点「转账」，就会通过 NodeLoc 把 NL 转给那位买家。</p>
                  </div>
                </td>
              </tr>
              <tr v-for="row in ledger" :key="row.id">
                <td class="quiet whitespace-nowrap text-xs">{{ when(row.created_at) }}</td>
                <td class="text-sm">
                  <RouterLink :to="`/users/${row.user_id}`" class="hover:text-[var(--accent)]">{{ row.username }}</RouterLink>
                  <span v-if="row.to_username" class="quiet text-xs mono"> · {{ row.to_username }}</span>
                </td>
                <td class="nums text-right text-sm">{{ money(row.amount) }}</td>
                <td>
                  <span class="badge" :class="transferStatus(row.status).badge">{{ transferStatus(row.status).label }}</span>
                </td>
                <td class="max-w-[240px] truncate text-xs mono quiet" :title="row.detail || row.reference">
                  {{ row.note || row.reference }}
                </td>
                <td class="quiet text-xs">{{ row.operator_name || '—' }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>
  </section>
</template>
