<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  checkinHistory,
  checkinStatus,
  checkIn,
  myPoints,
  startOAuthBind,
  syncOAuthProfile,
  unbindOAuth,
  updateProfile,
  uploadAvatar,
} from '../api/auth'
import { errorMessage } from '../api/client'
import { listNotifications, markAllRead, markNotificationRead } from '../api/notifications'
import type { InboxKind, InboxQuery } from '../api/notifications'
import { useAuthStore } from '../stores/auth'
import { useInboxStore } from '../stores/inbox'
import { useSiteStore } from '../stores/site'
import { dayLabel, oauthErrorText, roleLabel, when } from '../utils/format'
import type { AppNotification, CheckinRecord, CheckinStatus, PointEntry } from '../types'

const POINTS_PAGE = 20
const NOTES_PAGE = 10

const auth = useAuthStore()
const site = useSiteStore()
const inbox = useInboxStore()
const route = useRoute()
const router = useRouter()

const busy = ref(false)
const oauthStarting = ref(false)
const message = ref('')
const error = ref('')

const checkin = ref<CheckinStatus | null>(null)
const checkins = ref<CheckinRecord[]>([])
const entries = ref<PointEntry[]>([])
const pointsTotal = ref(0)
const pointsLoading = ref(false)
const notifications = ref<AppNotification[]>([])
const markingAll = ref(false)
const notesTotal = ref(0)
const notesLoading = ref(false)
const noteKind = ref('')
const notesUnreadOnly = ref(false)
const inboxKinds = ref<InboxKind[]>([])

const editing = ref(false)
const form = ref({ nickname: '', avatar_url: '', bio: '', email: '' })
const savingProfile = ref(false)
const avatarInput = ref<HTMLInputElement | null>(null)
const uploadingAvatar = ref(false)

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

async function reloadAccountPanels() {
  const [status, history, ledger, messages] = await Promise.allSettled([
    site.checkinEnabled ? checkinStatus() : Promise.resolve(null),
    site.checkinEnabled ? checkinHistory(7) : Promise.resolve([]),
    myPoints(POINTS_PAGE, 0),
    listNotifications(1, NOTES_PAGE, noteQuery()),
  ])
  if (status.status === 'fulfilled') checkin.value = status.value
  if (history.status === 'fulfilled') checkins.value = (history.value as CheckinRecord[]) ?? []
  if (ledger.status === 'fulfilled') {
    entries.value = ledger.value.data
    pointsTotal.value = ledger.value.total
  }
  if (messages.status === 'fulfilled') {
    notifications.value = messages.value.items
    notesTotal.value = messages.value.total
    inboxKinds.value = messages.value.kinds
  }
  // The count comes from the server because the panel only lists one page:
  // counting that page would under-report a longer inbox to the header badge.
  await inbox.refresh()
}

/** 一键清空未读：通知一条条点太慢，尤其是店家刚群发过公告。 */
async function readAllNotifications() {
  if (markingAll.value) return
  markingAll.value = true
  error.value = ''
  try {
    const marked = await markAllRead()
    notifications.value = notifications.value.map((item) => ({ ...item, is_read: true }))
    inbox.reset(0)
    // The tabs carry unread counts, and they all go to zero together.
    inboxKinds.value = inboxKinds.value.map((kind) => ({ ...kind, unread: 0 }))
    if (notesUnreadOnly.value) await loadNotes()
    message.value = marked ? `已把 ${marked} 条通知标为已读。` : '这些通知本来就是已读状态。'
  } catch (e) {
    error.value = errorMessage(e, '全部标为已读失败，请稍后重试')
  } finally {
    markingAll.value = false
  }
}

function noteQuery(): InboxQuery {
  return { kind: noteKind.value, unreadOnly: notesUnreadOnly.value }
}

/** One page of the inbox, either fresh or appended by 加载更多. */
async function loadNotes(append = false) {
  if (notesLoading.value) return
  notesLoading.value = true
  error.value = ''
  try {
    const page = append ? Math.floor(notifications.value.length / NOTES_PAGE) + 1 : 1
    const result = await listNotifications(page, NOTES_PAGE, noteQuery())
    notifications.value = append ? [...notifications.value, ...result.items] : result.items
    notesTotal.value = result.total
    inboxKinds.value = result.kinds
  } catch (e) {
    error.value = errorMessage(e, '通知读取失败，请稍后重试')
  } finally {
    notesLoading.value = false
  }
}

function pickKind(kind: string) {
  if (noteKind.value === kind) return
  noteKind.value = kind
  void loadNotes()
}

function toggleUnreadOnly() {
  notesUnreadOnly.value = !notesUnreadOnly.value
  void loadNotes()
}

/** One click back to the whole inbox, whichever tabs were on. */
function clearNoteFilter() {
  if (!notesFiltered.value) return
  noteKind.value = ''
  notesUnreadOnly.value = false
  void loadNotes()
}

const notesFiltered = computed(() => Boolean(noteKind.value) || notesUnreadOnly.value)
const notesMore = computed(() => notifications.value.length < notesTotal.value)
const notesAllCount = computed(() => inboxKinds.value.reduce((sum, kind) => sum + (kind.total || 0), 0))
const notesUnreadCount = computed(() => inboxKinds.value.reduce((sum, kind) => sum + (kind.unread || 0), 0))
// Only kinds the buyer actually has mail in become a tab, so the row never shows
// a 促销 chip that is guaranteed to answer "这里还没有通知".
const noteTabs = computed(() => inboxKinds.value.filter((kind) => kind.total > 0))
const kindLabels: Record<string, string> = {
  order: '订单',
  system: '系统',
  promo: '促销',
  announcement: '公告',
  stock: '库存预警',
  transfer: '店家转账',
}

function kindLabel(kind: string) {
  return kindLabels[kind] || kind
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

// An avatar is applied as it is uploaded, not when the form is saved: the shop
// drops the picture this one replaces, so a half-finished form must not leave a
// file on disk that no account points at.
async function chooseAvatar(event: Event) {
  const input = event.target as HTMLInputElement
  const picked = input.files?.[0]
  // Cleared so choosing the same file again after a refusal still counts as a
  // change, and so the field never shows a name it did not keep.
  input.value = ''
  if (!picked || uploadingAvatar.value) return

  uploadingAvatar.value = true
  error.value = ''
  message.value = ''
  try {
    const result = await uploadAvatar(picked)
    auth.user = result.user
    form.value.avatar_url = result.url
    message.value = '头像已更新。'
  } catch (e) {
    error.value = errorMessage(e, '头像上传失败，请稍后重试')
  } finally {
    uploadingAvatar.value = false
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
    inbox.reset(Math.max(0, inbox.unread - 1))
    inboxKinds.value = inboxKinds.value.map((kind) =>
      kind.type === item.type && kind.unread > 0 ? { ...kind, unread: kind.unread - 1 } : kind,
    )
    // Under the 未读 filter the row just stopped belonging to the list.
    if (notesUnreadOnly.value) await loadNotes()
  } catch (e) {
    error.value = errorMessage(e, '标记已读失败')
  }
}

async function startOAuthBinding() {
  if (oauthStarting.value) return
  oauthStarting.value = true
  error.value = ''
  message.value = ''
  try {
    // 绑定 leaves this page for NodeLoc and comes back to /profile with the
    // reason in the query string, so the outcome is reported by onMounted.
    await startOAuthBind('/profile')
  } catch (e) {
    error.value = errorMessage(e, '无法开始绑定，请稍后重试')
    oauthStarting.value = false
  }
}

onMounted(async () => {
  await auth.fetchUser().catch(() => undefined)
  if (!auth.isAuthenticated) {
    await router.replace('/login')
    return
  }
  if (typeof route.query.oauth_error === 'string') {
    // A bind that failed before a code was exchanged is reported here, in the
    // address bar; a bind that succeeded is reported through oauth_bind below.
    error.value = oauthErrorText(route.query.oauth_error, '绑定')
    await router.replace({ path: '/profile' })
  }
  // The 绑定 callback lands here with the outcome rather than a fragment: the
  // server already exchanged the code and attached the identity.
  if (typeof route.query.oauth_bind === 'string') {
    const outcome = route.query.oauth_bind
    await router.replace({ path: '/profile' })
    if (outcome === 'ok') {
      await auth.fetchUser().catch(() => undefined)
      message.value = 'NodeLoc 账号已绑定，之后可直接用 NodeLoc 登录。'
    } else {
      error.value = oauthErrorText(outcome, '绑定')
    }
    await reloadAccountPanels()
    return
  }
  await reloadAccountPanels()
})
</script>

<template>
  <div class="profile-page mx-auto w-full max-w-5xl px-4 py-9 sm:px-6">
    <header class="mb-6 flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="eyebrow">账号、积分与通知</p>
        <h1 class="mt-2 text-2xl font-bold sm:text-3xl">个人中心</h1>
      </div>
      <p class="hint max-w-md">资料、订单、积分和站内通知都在这里集中管理。</p>
    </header>

    <p v-if="message" class="alert alert-success mb-5" role="status">{{ message }}</p>
    <p v-if="error" class="alert alert-danger mb-5" role="alert">{{ error }}</p>

    <section class="card profile-summary">
      <div class="profile-hero flex items-start gap-4">
        <div class="profile-avatar grid size-[72px] shrink-0 place-items-center overflow-hidden rounded-full border border-[var(--stroke-hi)] bg-[var(--surface-hi)] text-xl font-bold">
          <img v-if="avatar" :src="avatar" :alt="displayName" class="size-full object-cover" />
          <span v-else>{{ initials }}</span>
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex flex-wrap items-center gap-2">
            <p class="truncate text-lg font-semibold">{{ displayName }}</p>
            <span v-if="auth.isStaff" :class="['badge', roleLabel(auth.accountRole).badge]">{{ roleLabel(auth.accountRole).label }}</span>
            <span v-else-if="user?.is_admin" class="badge badge-accent">管理员</span>
            <span v-if="profileComplete" class="badge badge-success">资料完整</span>
            <span v-else class="badge badge-neutral">资料待完善</span>
          </div>
          <p class="hint mt-1 truncate">
            {{ user?.email || '未绑定邮箱' }}
            <span v-if="emailLocked" class="opacity-70">· 来自 NodeLoc</span>
          </p>
          <p v-if="user?.bio" class="mt-2 break-words whitespace-pre-line text-[13px] leading-relaxed text-[var(--text-dim)]">
            {{ user.bio }}
          </p>
        </div>
        <div v-if="!editing" class="flex shrink-0 flex-wrap items-end justify-end gap-2">
          <button class="btn btn-quiet btn-sm" @click="startEditing">编辑资料</button>
          <button class="btn btn-quiet btn-sm" :disabled="uploadingAvatar" @click="avatarInput?.click()">
            {{ uploadingAvatar ? '上传中…' : '换头像' }}
          </button>
        </div>
      </div>

      <input ref="avatarInput" type="file" accept="image/png,image/jpeg,image/gif" class="hidden" @change="chooseAvatar" />

      <div class="my-5 divider" />

      <dl class="profile-stats mt-5 grid grid-cols-2 gap-2.5 sm:grid-cols-4">
        <div class="profile-stat">
          <dt class="text-[var(--text-quiet)]">用户 ID</dt>
          <dd class="nums mt-1">{{ user?.id ?? '—' }}</dd>
        </div>
        <div class="profile-stat">
          <dt class="text-[var(--text-quiet)]">积分</dt>
          <dd class="nums mt-1">{{ user?.points ?? 0 }}</dd>
        </div>
        <div class="profile-stat">
          <dt class="text-[var(--text-quiet)]">注册时间</dt>
          <dd class="mt-1">{{ when(user?.created_at) }}</dd>
        </div>
        <div class="profile-stat">
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
          <div class="profile-avatar-input flex items-center gap-2">
            <input
              id="avatar"
              v-model="form.avatar_url"
              class="input min-w-0"
              maxlength="255"
              placeholder="https://…（留空则使用 NodeLoc 头像）"
            />
            <button class="btn btn-secondary btn-sm shrink-0" :disabled="uploadingAvatar" @click="avatarInput?.click()">
              {{ uploadingAvatar ? '上传中…' : '上传' }}
            </button>
          </div>
          <p class="hint mt-1">上传的图片会立刻换上，旧的头像随即丢弃；这里也可以填一个外部图片地址，保存后生效。</p>
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
              已绑定 <span class="mono">{{ user?.oauth_username || 'NodeLoc 账号' }}</span>
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
          <button v-else class="btn btn-primary btn-sm" :disabled="oauthStarting" @click="startOAuthBinding">
            <span v-if="oauthStarting" class="spinner spinner-light" />
            {{ oauthStarting ? '正在连接 NodeLoc…' : '绑定 NodeLoc' }}
          </button>
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
      <div class="flex flex-wrap items-baseline justify-between gap-3">
        <h2 class="text-[15px] font-bold">通知</h2>
        <div class="flex items-center gap-2">
          <span v-if="inbox.unread" class="badge badge-accent nums">{{ inbox.unread }} 未读</span>
          <button
            v-if="inbox.unread"
            class="btn btn-quiet btn-sm"
            :disabled="markingAll"
            @click="readAllNotifications"
          >
            {{ markingAll ? '处理中…' : '全部标为已读' }}
          </button>
        </div>
      </div>

      <div v-if="noteTabs.length" class="mt-4 flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="chip"
          :class="!notesFiltered ? 'chip-active' : ''"
          @click="clearNoteFilter"
        >
          全部 <span class="nums">{{ notesAllCount }}</span>
        </button>
        <button
          v-if="notesUnreadCount"
          type="button"
          class="chip"
          :class="notesUnreadOnly ? 'chip-active' : ''"
          @click="toggleUnreadOnly"
        >
          未读 <span class="nums">{{ notesUnreadCount }}</span>
        </button>
        <button
          v-for="kind in noteTabs"
          :key="kind.type"
          type="button"
          class="chip"
          :class="noteKind === kind.type ? 'chip-active' : ''"
          @click="pickKind(kind.type)"
        >
          {{ kindLabel(kind.type) }} <span class="nums">{{ kind.total }}</span>
          <span v-if="kind.unread" class="nums hint">· {{ kind.unread }} 未读</span>
        </button>
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
              <span v-if="item.content" class="hint mt-1 block line-clamp-2 break-words whitespace-pre-line">{{ item.content }}</span>
              <span class="hint mt-1 block">{{ when(item.created_at) }}</span>
            </span>
            <span v-if="item.link" aria-hidden="true" class="hint shrink-0">→</span>
          </button>
        </li>
      </ul>
      <p v-else-if="notesFiltered" class="hint mt-4">
        {{ notesUnreadOnly ? '这一类没有未读的了。' : '这个类别还没有通知。' }}
        <button v-if="noteTabs.length" type="button" class="chip ml-1" @click="clearNoteFilter">看全部</button>
      </p>
      <p v-else class="hint mt-4">暂时没有通知。</p>

      <button
        v-if="notesMore"
        type="button"
        class="btn btn-quiet btn-sm mt-4"
        :disabled="notesLoading"
        @click="loadNotes(true)"
      >
        {{ notesLoading ? '加载中…' : `加载更多（还有 ${notesTotal - notifications.length} 条）` }}
      </button>
    </section>

    <nav class="profile-actions mt-5 grid gap-3 sm:grid-cols-3">
      <RouterLink to="/orders" class="card-hover flex items-center justify-between gap-3">
        <span>
          <span class="block text-[15px] font-semibold">我的订单</span>
          <span class="hint">支付进度与交付内容</span>
        </span>
        <span aria-hidden="true" class="text-[var(--text-quiet)]">→</span>
      </RouterLink>
      <RouterLink to="/support" class="card-hover flex items-center justify-between gap-3">
        <span>
          <span class="block text-[15px] font-semibold">工单中心</span>
          <span class="hint">提交问题、查看进度与回复</span>
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

<style scoped>
.profile-actions > a {
  min-width: 0;
}

.profile-actions > a > span:first-child {
  min-width: 0;
}

.profile-actions .hint {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.profile-summary {
  overflow: hidden;
}

.profile-avatar {
  box-shadow: 0 0 0 4px var(--surface-hi);
}

.profile-stats {
  border-top: 1px solid var(--stroke-quiet);
  padding-top: 18px;
}

.profile-stat {
  min-width: 0;
  border-radius: var(--radius-sm);
  background: var(--surface-sunken);
  padding: 10px 12px;
}

.profile-stat dd {
  overflow: hidden;
  color: var(--text);
  font-size: 13px;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .profile-hero {
    flex-wrap: wrap;
  }
}
</style>
