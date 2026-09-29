<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import { broadcastNotification, listNotifications, markAllRead, markAsRead, sendNotification } from '../api/notifications'
import { errorMessage, notificationKind, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { useInboxStore } from '../stores/inbox'
import type { Notification } from '../types'

const PageSize = 20

const auth = useAuthStore()
const inbox = useInboxStore()
// Sending and broadcasting are one guarded route pair on the server; the form
// stays visible so a 客服 account sees what they are missing, but nothing in it
// is pressable without the grant.
const canManage = computed(() => auth.allows('notifications', 'manage'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
// The list confirms its own reads; the composer confirms what it sent. One
// shared sentence would show the same words on both sides of the page.
const listNotice = ref('')
const notifications = ref<Notification[]>([])
const total = ref(0)
const page = ref(1)

const form = ref({ type: 'system', title: '', content: '', link: '', user_id: '' })

const unread = computed(() => notifications.value.filter((item) => !item.is_read).length)
const totalPages = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listNotifications(page.value, PageSize)
    notifications.value = result.items
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载通知失败')
  } finally {
    loading.value = false
  }
  // The sidebar dot is the server's count, not this page's, so a restock warning
  // that arrived while this screen was open still shows up.
  void inbox.refresh()
}

async function read(item: Notification) {
  if (item.is_read) return
  try {
    await markAsRead(item.id)
    item.is_read = true
    if (inbox.unread > 0) inbox.reset(inbox.unread - 1)
  } catch (err) {
    error.value = errorMessage(err, '标记已读失败')
  }
}

async function readAll() {
  if (busy.value) return
  busy.value = true
  error.value = ''
  listNotice.value = ''
  try {
    const marked = await markAllRead()
    notifications.value = notifications.value.map((item) => ({ ...item, is_read: true }))
    inbox.reset(0)
    listNotice.value = marked ? `已把 ${marked} 条标为已读。` : '这里已经没有未读消息了。'
  } catch (err) {
    error.value = errorMessage(err, '标记已读失败')
  } finally {
    busy.value = false
  }
}

async function publish() {
  const title = form.value.title.trim()
  if (!title) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload = {
      type: form.value.type,
      title,
      content: form.value.content.trim() || undefined,
      link: form.value.link.trim() || undefined,
    }
    const userId = Number(form.value.user_id)
    if (userId > 0) {
      await sendNotification({ ...payload, user_id: userId })
      notice.value = `已发送给用户 #${userId}`
    } else {
      const result = await broadcastNotification(payload)
      notice.value = `已广播给 ${result.sent ?? 0} 位用户`
    }
    form.value = { ...form.value, title: '', content: '', link: '', user_id: '' }
  } catch (err) {
    error.value = errorMessage(err, '发送失败')
  } finally {
    busy.value = false
  }
}

function go(next: number) {
  page.value = Math.max(1, next)
  load()
}

/**
 * Where the message points inside the back office. A restock warning carries the
 * card page of the shelf it is about, so reading it and acting on it are one
 * click. An absolute link a shop broadcast instead stays a normal page open: it
 * is not a router path this app owns.
 */
function routeOf(link?: string | null): string | null {
  if (!link) return null
  return link.startsWith('/') && !link.startsWith('//') ? link : null
}

onMounted(load)
</script>

<template>
  <section class="grid gap-5 lg:grid-cols-3">
    <div class="space-y-4 lg:col-span-2">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <h2 class="text-base font-semibold">我的通知</h2>
          <span v-if="unread" class="badge badge-accent">{{ unread }} 未读</span>
          <button
            v-if="inbox.unread || unread"
            class="btn btn-quiet btn-sm"
            :disabled="busy"
            title="把这一页和后面的未读消息一次读完"
            @click="readAll"
          >
            全部标为已读
          </button>
        </div>
        <PaginationFooter
          class="!justify-end"
          :page="page"
          :pages="totalPages"
          :loading="loading"
          summary=""
          @change="go"
        />
      </div>

      <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
      <p v-if="listNotice" class="alert alert-info" role="status">{{ listNotice }}</p>

      <div v-if="loading" class="space-y-2">
        <div v-for="i in 5" :key="i" class="skeleton h-20" />
      </div>
      <p v-else-if="!notifications.length" class="card py-14 text-center text-sm quiet">
        {{ canManage ? '还没有通知。右侧广播一条试试。' : '还没有通知。' }}
      </p>
      <div
        v-else
        class="card !p-0 divide-y divide-[var(--stroke-quiet)]"
      >
        <div v-for="item in notifications" :key="item.id" class="flex items-start gap-4 px-5 py-4">
          <span
            class="mt-1.5 h-2 w-2 shrink-0 rounded-full"
            :class="item.is_read ? 'bg-[var(--stroke-hi)]' : 'bg-[var(--accent)]'"
          />
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="text-sm font-medium">{{ item.title }}</h3>
              <span class="badge badge-neutral">{{ notificationKind(item.type) }}</span>
              <RouterLink
                v-if="routeOf(item.link)"
                :to="routeOf(item.link) as string"
                class="text-xs accent-text underline-offset-2 hover:underline"
                >去处理 →</RouterLink
              >
              <a
                v-else-if="item.link"
                :href="item.link"
                target="_blank"
                rel="noopener noreferrer"
                class="text-xs accent-text underline-offset-2 hover:underline"
                >打开链接 ↗</a
              >
            </div>
            <p v-if="item.content" class="mt-1 text-sm muted">{{ item.content }}</p>
            <p class="quiet mt-1.5 text-xs">{{ when(item.created_at) }}</p>
          </div>
          <button v-if="!item.is_read" class="btn btn-ghost btn-sm" :disabled="busy" @click="read(item)">标为已读</button>
        </div>
      </div>
    </div>

    <div class="card h-fit">
      <h2 class="text-base font-semibold">发送通知</h2>
      <p class="quiet mt-1 mb-4 text-xs">填写用户 ID 只通知单个用户，留空则广播给全部用户。</p>
      <p v-if="!canManage" class="alert alert-info mb-4 text-xs" role="status">
        当前账号没有「通知管理」权限，只能查看自己的通知。
      </p>

      <fieldset class="m-0 space-y-3 border-0 p-0" :disabled="!canManage">
        <div>
          <label class="label" for="n-type">类型</label>
          <select id="n-type" v-model="form.type" class="input">
            <option value="system">system</option>
            <option value="order">order</option>
            <option value="promo">promo</option>
          </select>
        </div>
        <div>
          <label class="label" for="n-user">用户 ID（可选）</label>
          <input id="n-user" v-model="form.user_id" class="input nums" placeholder="留空表示广播" />
        </div>
        <div>
          <label class="label" for="n-title">标题</label>
          <input id="n-title" v-model="form.title" class="input" placeholder="必填" />
        </div>
        <div>
          <label class="label" for="n-content">正文</label>
          <textarea id="n-content" v-model="form.content" class="input h-28 resize-none" />
        </div>
        <div>
          <label class="label" for="n-link">链接</label>
          <input id="n-link" v-model="form.link" class="input mono text-xs" placeholder="/orders 或 https://…" />
        </div>

        <p v-if="notice" class="alert alert-success mt-4" role="status">{{ notice }}</p>

        <button class="btn btn-primary mt-4 w-full" :disabled="busy || !form.title.trim()" @click="publish">
          {{ busy ? '发送中…' : '发送' }}
        </button>
      </fieldset>
    </div>
  </section>
</template>
