<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  bindOAuth,
  checkinHistory,
  checkinStatus,
  checkIn,
  myPoints,
  oauthInitiate,
  syncOAuthProfile,
  unbindOAuth,
  updateProfile,
} from '../api/auth'
import { errorMessage } from '../api/client'
import { listNotifications, markNotificationRead } from '../api/notifications'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'
import { oauthErrorText, when } from '../utils/format'
import type { AppNotification, CheckinRecord, CheckinStatus, PointEntry } from '../types'

const POINTS_PAGE = 20

const auth = useAuthStore()
const site = useSiteStore()
const route = useRoute()
const router = useRouter()

const busy = ref(false)
const message = ref('')
const error = ref('')

const checkin = ref<CheckinStatus | null>(null)
const checkins = ref<CheckinRecord[]>([])
const entries = ref<PointEntry[]>([])
const pointsTotal = ref(0)
const pointsLoading = ref(false)
const notifications = ref<AppNotification[]>([])
const unread = ref(0)

const editing = ref(false)
const form = ref({ nickname: '', avatar_url: '', bio: '', email: '' })
const savingProfile = ref(false)

const user = computed(() => auth.user)
const bound = computed(() => Boolean(user.value?.oauth_provider))
const displayName = computed(() => user.value?.nickname || user.value?.oauth_username || user.value?.username || '账户')
const avatar = computed(() => user.value?.avatar_url || user.value?.oauth_avatar || '')
const initials = computed(() => displayName.value.slice(0, 1).toUpperCase())
const emailLocked = computed(() => bound.value && user.value?.oauth_has_email === true)
const profileComplete = computed(() => {
  const current = user.value
  if (!current) return false
  return Boolean((current.nickname || current.username) && current.email && (current.avatar_url || current.oauth_avatar))
})
const morePoints = computed(() => entries.value.length < pointsTotal.value)

function dayLabel(value: string) {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' }).format(date)
}

async function reloadAccountPanels() {
  const [status, history, ledger, inbox] = await Promise.allSettled([
    site.checkinEnabled ? checkinStatus() : Promise.resolve(null),
    site.checkinEnabled ? checkinHistory(7) : Promise.resolve([]),
    myPoints(POINTS_PAGE, 0),
    listNotifications(1, 10),
  ])
  if (status.status === 'fulfilled') checkin.value = status.value
  if (history.status === 'fulfilled') checkins.value = (history.value as CheckinRecord[]) ?? []
  if (ledger.status === 'fulfilled') {
    entries.value = ledger.value.data
    pointsTotal.value = ledger.value.total
  }
  if (inbox.status === 'fulfilled') {
    notifications.value = inbox.value.items
    unread.value = inbox.value.items.filter((item) => !item.is_read).length
  }
}

async function doCheckIn() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const result = await checkIn()
    auth.user = result.user
    checkin.value = {
      enabled: true,
      checked_in_today: true,
      consecutive_days: result.consecutive_days,
      total_checkins: result.total_checkins,
      points: result.points,
    }
    message.value = `签到成功，获得 ${result.reward} 积分，已连续 ${result.consecutive_days} 天。`
    const ledger = await myPoints(POINTS_PAGE, 0)
    entries.value = ledger.data
    pointsTotal.value = ledger.total
    checkins.value = await checkinHistory(7)
  } catch (e) {
    error.value = errorMessage(e, '签到失败，请稍后重试')
  } finally {
    busy.value = false
  }
}

async function loadMorePoints() {
  if (pointsLoading.value) return
  pointsLoading.value = true
  try {
    const ledger = await myPoints(POINTS_PAGE, entries.value.length)
    entries.value = [...entries.value, ...ledger.data]
    pointsTotal.value = ledger.total
  } catch (e) {
    error.value = errorMessage(e, '积分明细加载失败')
  } finally {
    pointsLoading.value = false
  }
}

function startEditing() {
  const current = user.value
  form.value = {
    nickname: current?.nickname || '',
    avatar_url: current?.avatar_url || current?.oauth_avatar || '',
    bio: current?.bio || '',
    email: current?.email || '',
  }
  editing.value = true
  message.value = ''
  error.value = ''
}

async function saveProfile() {
  if (savingProfile.value) return
  savingProfile.value = true
  error.value = ''
  message.value = ''
  try {
    const payload: Record<string, string> = {
      nickname: form.value.nickname.trim(),
      avatar_url: form.value.avatar_url.trim(),
      bio: form.value.bio.trim(),
    }
    // An address owned by NodeLoc is not sent at all: the server would refuse
    // it, and the field is already disabled in the form.
    if (!emailLocked.value && form.value.email.trim()) payload.email = form.value.email.trim()
    auth.user = await updateProfile(payload)
    editing.value = false
    message.value = '资料已更新。'
  } catch (e) {
    error.value = errorMessage(e, '资料保存失败，请稍后重试')
  } finally {
    savingProfile.value = false
  }
}

async function syncProfile() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    auth.user = await syncOAuthProfile()
    message.value = '已从 NodeLoc 同步最新的昵称、头像与邮箱。'
  } catch (e) {
    error.value = errorMessage(e, '同步失败，请重新绑定 NodeLoc 账号')
  } finally {
    busy.value = false
  }
}

async function unbind() {
  if (busy.value) return
  if (!window.confirm('解绑后将无法用 NodeLoc 一键登录，确认继续？')) return
  busy.value = true
  message.value = ''
  error.value = ''
  try {
    const response = await unbindOAuth()
    auth.user = response.user
    message.value = '已解除 NodeLoc 绑定，本地账号与订单不受影响。'
  } catch (e) {
    error.value = errorMessage(e, '解绑失败，请稍后重试')
  } finally {
    busy.value = false
  }
}

async function openNotification(item: AppNotification) {
  await readNotification(item)
  const link = (item.link || '').trim()
  if (!link) return
  // Only this storefront or a plain http(s) address: an injected
  // javascript: URL in a notification would otherwise run on click.
  if (link.startsWith('/')) {
    await router.push(link)
    return
  }
  if (/^https?:\/\//i.test(link)) window.open(link, '_blank', 'noopener')
}

async function readNotification(item: AppNotification) {
  if (item.is_read) return
  try {
    await markNotificationRead(item.id)
    item.is_read = true
    unread.value = Math.max(0, unread.value - 1)
  } catch (e) {
    error.value = errorMessage(e, '标记已读失败')
  }
}

async function consumeBindCode() {
  const fragment = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const code = fragment.get('bind_code') || ''
  const state = fragment.get('state') || ''
  if (!code || !state) return
  history.replaceState(null, '', window.location.pathname + window.location.search)
  busy.value = true
  error.value = ''
  try {
    auth.user = await bindOAuth(code, state)
    message.value = 'NodeLoc 账号已绑定，之后可直接用 NodeLoc 登录。'
    await reloadAccountPanels()
  } catch (e) {
    error.value = errorMessage(e, '绑定失败，请重试')
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  await auth.fetchUser().catch(() => undefined)
  if (!auth.isAuthenticated) {
    await router.replace('/login')
    return
  }
  if (typeof route.query.oauth_error === 'string') {
    error.value = oauthErrorText(route.query.oauth_error, '绑定')
    await router.replace({ path: '/profile' })
  }
  await consumeBindCode()
  await reloadAccountPanels()
})
</script>

<template>
  <div class="mx-auto w-full max-w-3xl px-4 py-10 sm:px-6">
    <header class="mb-7">
      <p class="eyebrow">Account</p>
      <h1 class="mt-2 text-2xl font-bold">个人中心</h1>
    </header>

    <p v-if="message" class="alert alert-success mb-5" role="status">{{ message }}</p>
    <p v-if="error" class="alert alert-danger mb-5" role="alert">{{ error }}</p>

    <section class="card">
      <div class="flex items-start gap-4">
        <div class="grid size-16 shrink-0 place-items-center overflow-hidden rounded-full border border-[var(--stroke)] bg-[var(--surface-hi)] text-xl font-bold">
          <img v-if="avatar" :src="avatar" :alt="displayName" class="size-full object-cover" />
          <span v-else>{{ initials }}</span>
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <p class="truncate text-lg font-semibold">{{ displayName }}</p>
            <span v-if="auth.isStaff" class="badge badge-accent">{{ auth.accountRole || '工作人员' }}</span>
            <span v-else-if="user?.is_admin" class="badge badge-accent">管理员</span>
            <span v-if="profileComplete" class="badge badge-success">资料完整</span>
            <span v-else class="badge badge-neutral">资料待完善</span>
          </div>
          <p class="hint mt-1 truncate">
            {{ user?.email || '未绑定邮箱' }}
            <span v-if="emailLocked" class="opacity-70">· 来自 NodeLoc</span>
          </p>
          <p v-if="user?.bio" class="mt-2 whitespace-pre-line text-[13px] leading-relaxed text-[var(--text-dim)]">
            {{ user.bio }}
          </p>
        </div>
        <button v-if="!editing" class="btn btn-quiet btn-sm shrink-0" @click="startEditing">编辑资料</button>
      </div>

      <div class="my-5 divider" />

      <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-4">
        <div>
          <dt class="text-[var(--text-quiet)]">用户 ID</dt>
          <dd class="nums mt-1">{{ user?.id ?? '—' }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">积分</dt>
          <dd class="nums mt-1">{{ user?.points ?? 0 }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">注册时间</dt>
          <dd class="mt-1">{{ when(user?.created_at) }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">上次登录</dt>
          <dd class="mt-1">{{ when(user?.last_login_at) }}</dd>
        </div>
      </dl>

      <form v-if="editing" class="mt-6 space-y-4 border-t border-[var(--stroke-quiet)] pt-6" @submit.prevent="saveProfile">
        <div>
          <label class="label" for="nickname">昵称</label>
          <input id="nickname" v-model="form.nickname" class="input" maxlength="32" placeholder="展示在个人中心与订单中" />
        </div>
        <div>
          <label class="label" for="avatar">头像地址</label>
          <input id="avatar" v-model="form.avatar_url" class="input" maxlength="255" placeholder="https://…（留空则使用 NodeLoc 头像）" />
        </div>
        <div>
          <label class="label" for="email">邮箱</label>
          <input
            id="email"
            v-model="form.email"
            class="input"
            type="email"
            maxlength="190"
            :disabled="emailLocked"
            :placeholder="emailLocked ? '由 NodeLoc 管理，请在论坛修改后同步' : '用于接收订单与交付通知'"
          />
        </div>
        <div>
          <label class="label" for="bio">个人简介</label>
          <textarea id="bio" v-model="form.bio" class="input" maxlength="500" placeholder="选填"></textarea>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-primary btn-sm" type="submit" :disabled="savingProfile">
            <span v-if="savingProfile" class="spinner spinner-light" />
            {{ savingProfile ? '保存中…' : '保存' }}
          </button>
          <button class="btn btn-quiet btn-sm" type="button" @click="editing = false">取消</button>
        </div>
      </form>
    </section>

    <!-- 签到: the daily loop the shop owner turned on in 设置. -->
    <section v-if="site.checkinEnabled && checkin" class="card mt-5">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 class="text-[15px] font-bold">每日签到</h2>
          <p class="hint mt-1">
            连续签到有额外积分，中断则从第 1 天重新开始。
          </p>
        </div>
        <button
          class="btn btn-sm shrink-0"
          :class="checkin.checked_in_today ? 'btn-quiet' : 'btn-primary'"
          :disabled="busy || checkin.checked_in_today || !checkin.enabled"
          @click="doCheckIn"
        >
          {{ checkin.checked_in_today ? '今日已签到' : checkin.enabled ? '立即签到' : '签到未开启' }}
        </button>
      </div>

      <dl class="mt-5 grid grid-cols-3 gap-4 text-center">
        <div class="card-quiet">
          <dt class="text-[var(--text-quiet)]">连续天数</dt>
          <dd class="nums mt-1 text-xl font-bold">{{ checkin.consecutive_days }}</dd>
        </div>
        <div class="card-quiet">
          <dt class="text-[var(--text-quiet)]">累计签到</dt>
          <dd class="nums mt-1 text-xl font-bold">{{ checkin.total_checkins }}</dd>
        </div>
        <div class="card-quiet">
          <dt class="text-[var(--text-quiet)]">当前积分</dt>
          <dd class="nums mt-1 text-xl font-bold">{{ checkin.points }}</dd>
        </div>
      </dl>

      <div v-if="checkins.length" class="mt-5 flex flex-wrap gap-2">
        <span
          v-for="item in checkins"
          :key="item.id"
          class="badge"
          :class="item.consecutive_days >= 7 ? 'badge-teal' : 'badge-neutral'"
        >
          {{ dayLabel(item.checkin_date) }} · +{{ item.reward_points }}
        </span>
      </div>
    </section>

    <section class="card mt-5">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
          <h2 class="text-[15px] font-bold">NodeLoc 账号</h2>
          <p class="hint mt-1">
            <template v-if="bound">
              已绑定 <span class="mono">{{ user?.oauth_username || user?.oauth_provider }}</span>
              <span v-if="user?.oauth_uid" class="mono"> · ID {{ user.oauth_uid }}</span>
              <span v-if="user?.oauth_trust_level !== null && user?.oauth_trust_level !== undefined">
                · 信任等级 {{ user.oauth_trust_level }}
              </span>
            </template>
            <template v-else>绑定后可使用 NodeLoc 账号一键登录，本地订单仍保留在此账号下。</template>
          </p>
        </div>
        <div class="flex shrink-0 flex-wrap gap-2">
          <button v-if="bound" class="btn btn-secondary btn-sm" :disabled="busy" @click="syncProfile">同步资料</button>
          <button v-if="bound" class="btn btn-danger btn-sm" :disabled="busy" @click="unbind">解除绑定</button>
          <button v-else class="btn btn-primary btn-sm" @click="oauthInitiate(true)">绑定 NodeLoc</button>
        </div>
      </div>
      <p v-if="!bound" class="alert alert-info mt-5" role="status">
        若该 NodeLoc 账号此前已在本店独立注册过，它将作为另一个账号绑定失败——可先联系管理员合并。
      </p>
    </section>

    <section class="card mt-5">
      <div class="flex items-baseline justify-between gap-3">
        <h2 class="text-[15px] font-bold">积分明细</h2>
        <span class="hint nums">{{ pointsTotal }} 条</span>
      </div>
      <p class="hint mt-1">签到与店家调整都会记账，余额一栏可以逐条对上。</p>

      <ul v-if="entries.length" class="mt-4 divide-y divide-[var(--stroke-quiet)]">
        <li v-for="item in entries" :key="item.id" class="flex items-center justify-between gap-4 py-3 text-sm">
          <div class="min-w-0">
            <p class="truncate">{{ item.reason || '积分变动' }}</p>
            <p class="hint mt-0.5">{{ when(item.created_at) }}</p>
          </div>
          <div class="shrink-0 text-right">
            <p class="nums font-semibold" :class="item.delta >= 0 ? 'accent-text' : 'text-[var(--danger)]'">
              {{ item.delta >= 0 ? '+' : '' }}{{ item.delta }}
            </p>
            <p class="hint nums">余额 {{ item.balance_after }}</p>
          </div>
        </li>
      </ul>
      <p v-else class="hint mt-4">还没有积分记录，签到或店家的调整都会出现在这里。</p>

      <button v-if="morePoints" class="btn btn-quiet btn-sm mt-4" :disabled="pointsLoading" @click="loadMorePoints">
        {{ pointsLoading ? '加载中…' : '加载更多' }}
      </button>
    </section>

    <section class="card mt-5">
      <div class="flex items-baseline justify-between gap-3">
        <h2 class="text-[15px] font-bold">通知</h2>
        <span v-if="unread" class="badge badge-accent nums">{{ unread }} 未读</span>
      </div>

      <ul v-if="notifications.length" class="mt-4 divide-y divide-[var(--stroke-quiet)]">
        <li v-for="item in notifications" :key="item.id" class="py-3 text-sm">
          <button
            type="button"
            class="flex w-full cursor-pointer items-start gap-3 text-left transition-colors hover:text-[var(--accent)]"
            @click="openNotification(item)"
          >
            <span class="mt-1.5 size-2 shrink-0 rounded-full" :class="item.is_read ? 'bg-[var(--stroke-hi)]' : 'bg-[var(--accent)]'" />
            <span class="min-w-0 flex-1">
              <span class="block truncate font-medium">{{ item.title }}</span>
              <span v-if="item.content" class="hint mt-1 block line-clamp-2 whitespace-pre-line">{{ item.content }}</span>
              <span class="hint mt-1 block">{{ when(item.created_at) }}</span>
            </span>
            <span v-if="item.link" aria-hidden="true" class="hint shrink-0">→</span>
          </button>
        </li>
      </ul>
      <p v-else class="hint mt-4">暂时没有通知。</p>
    </section>

    <nav class="mt-5 grid gap-3 sm:grid-cols-2">
      <RouterLink to="/orders" class="card-hover flex items-center justify-between gap-3">
        <span>
          <span class="block text-[15px] font-semibold">我的订单</span>
          <span class="hint">支付进度与交付内容</span>
        </span>
        <span aria-hidden="true" class="text-[var(--text-quiet)]">→</span>
      </RouterLink>
      <RouterLink to="/" class="card-hover flex items-center justify-between gap-3">
        <span>
          <span class="block text-[15px] font-semibold">继续挑选</span>
          <span class="hint">浏览在售的数码商品</span>
        </span>
        <span aria-hidden="true" class="text-[var(--text-quiet)]">→</span>
      </RouterLink>
    </nav>
  </div>
</template>
