<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import PageHeader from '../components/PageHeader.vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import { broadcastNotification, listNotifications, markAllRead, markAsRead, sendNotification } from '../api/notifications'
import { errorMessage, notificationKind, when } from '../utils/format'
import { useInboxStore } from '../stores/inbox'
import type { Notification } from '../types'

/** 通知中心：管理员的收件箱 + 向用户发通知 / 广播。 */

const PAGE_SIZE = 20

const router = useRouter()
const inbox = useInboxStore()

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const items = ref<Notification[]>([])
const total = ref(0)
const page = ref(1)
const filter = ref('all')
const showSend = ref(false)
const draft = ref({ target: 'broadcast', user_id: '', type: 'system', title: '', content: '', link: '' })

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(() => {
  if (filter.value === 'all') return items.value
  if (filter.value === 'unread') return items.value.filter((n) => !n.is_read)
  return items.value.filter((n) => n.type === filter.value)
})
const kinds = computed(() => ['all', 'unread', ...new Set(items.value.map((n) => n.type))])

async function load() {
  loading.value = true
  error.value = ''
  try {
    const result = await listNotifications(page.value, PAGE_SIZE)
    items.value = result.items
    total.value = result.total
  } catch (err) {
    error.value = errorMessage(err, '加载通知失败')
  } finally {
    loading.value = false
  }
}

async function read(item: Notification) {
  try {
    await markAsRead(item.id)
    items.value = items.value.map((n) => (n.id === item.id ? { ...n, is_read: true } : n))
    await inbox.refresh()
  } catch (err) {
    error.value = errorMessage(err, '标记已读失败')
  }
}

async function readAll() {
  busy.value = true
  try {
    const marked = await markAllRead()
    notice.value = '已把 ' + marked + ' 条通知标为已读。'
    await load()
    await inbox.refresh()
  } catch (err) {
    error.value = errorMessage(err, '操作失败')
  } finally {
    busy.value = false
  }
}

function open(item: Notification) {
  if (!item.is_read) void read(item)
  if (item.link) {
    if (/^https?:\/\//i.test(item.link)) window.open(item.link, '_blank', 'noopener')
    else void router.push(item.link)
  }
}

async function send() {
  if (!draft.value.title.trim()) {
    error.value = '请填写通知标题。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    if (draft.value.target === 'broadcast') {
      const result = await broadcastNotification({
        type: draft.value.type, title: draft.value.title,
        content: draft.value.content || undefined, link: draft.value.link || undefined,
      })
      notice.value = '已向 ' + result.sent + ' 位用户发出通知。'
    } else {
      const userId = Number(draft.value.user_id)
      if (!userId) {
        error.value = '请填写收件人的用户 ID。'
        busy.value = false
        return
      }
      await sendNotification({
        user_id: userId, type: draft.value.type, title: draft.value.title,
        content: draft.value.content || undefined, link: draft.value.link || undefined,
      })
      notice.value = '已发送给用户 #' + userId + '。'
    }
    showSend.value = false
    draft.value = { target: 'broadcast', user_id: '', type: 'system', title: '', content: '', link: '' }
    await load()
  } catch (err) {
    error.value = errorMessage(err, '发送失败')
  } finally {
    busy.value = false
  }
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <PageHeader title="通知中心" description="你的站内信，以及向买家发送通知与广播。" bordered>
      <template #actions>
        <button class="btn btn-primary btn-sm" @click="showSend = true">
          <AdminIcon name="plus" :size="14" />
          发送通知
        </button>
        <button class="btn btn-quiet btn-sm" :disabled="busy" @click="readAll">全部标为已读</button>
        <button class="btn btn-quiet btn-sm" :disabled="loading" @click="load">
          <AdminIcon name="refresh" :size="14" />
          刷新
        </button>
      </template>
    </PageHeader>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="kind in kinds"
        :key="kind"
        class="chip"
        :class="filter === kind ? 'chip-active' : ''"
        @click="filter = kind"
      >
        {{ kind === 'all' ? '全部' : kind === 'unread' ? '未读' : notificationKind(kind) }}
      </button>
    </div>

    <div v-if="loading" class="card space-y-2.5">
      <div v-for="i in 5" :key="i" class="skeleton h-12 w-full" />
    </div>
    <div v-else-if="!filtered.length" class="card py-16 text-center">
      <p class="font-semibold">没有通知</p>
      <p class="quiet mt-1.5 text-sm">订单、工单与库存提醒都会出现在这里。</p>
    </div>
    <template v-else>
      <ul class="space-y-2">
        <li v-for="item in filtered" :key="item.id">
          <button
            class="card-quiet flex w-full items-start gap-3 text-left transition-colors hover:border-[var(--stroke-hi)]"
            @click="open(item)"
          >
            <span class="mt-0.5 shrink-0">
              <span v-if="!item.is_read" class="badge-accent">未读</span>
              <span v-else class="badge-neutral">{{ notificationKind(item.type) }}</span>
            </span>
            <span class="min-w-0 flex-1">
              <span class="block text-[13.5px] font-semibold">{{ item.title }}</span>
              <span v-if="item.content" class="quiet mt-1 block whitespace-pre-line text-xs">{{ item.content }}</span>
            </span>
            <span class="quiet shrink-0 text-[11px]">{{ when(item.created_at) }}</span>
          </button>
        </li>
      </ul>
      <PaginationFooter :page="page" :pages="pageCount" :loading="loading" :summary="'共 ' + total + ' 条'" @change="goPage" />
    </template>

    <AppDrawer :open="showSend" title="发送通知" @close="showSend = false">
      <div class="space-y-3">
        <div>
          <label class="label" for="n-target">发送对象</label>
          <select id="n-target" v-model="draft.target" class="input">
            <option value="broadcast">全体用户（广播）</option>
            <option value="single">指定用户</option>
          </select>
        </div>
        <div v-if="draft.target === 'single'">
          <label class="label" for="n-user">用户 ID</label>
          <input id="n-user" v-model="draft.user_id" class="input nums" placeholder="例如 12" />
        </div>
        <div>
          <label class="label" for="n-type">类型</label>
          <select id="n-type" v-model="draft.type" class="input">
            <option value="system">系统</option>
            <option value="order">订单</option>
            <option value="ticket">工单</option>
            <option value="activity">活动</option>
            <option value="coupon">优惠券</option>
            <option value="stock">库存</option>
          </select>
        </div>
        <div>
          <label class="label" for="n-title">标题</label>
          <input id="n-title" v-model="draft.title" class="input" maxlength="200" />
        </div>
        <div>
          <label class="label" for="n-content">内容</label>
          <textarea id="n-content" v-model="draft.content" class="input min-h-[100px]" />
        </div>
        <div>
          <label class="label" for="n-link">跳转链接（可选）</label>
          <input id="n-link" v-model="draft.link" class="input mono text-xs" placeholder="/orders 或 https://…" />
        </div>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="showSend = false">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="send">{{ busy ? '发送中…' : '发送' }}</button>
        </div>
      </template>
    </AppDrawer>
  </section>
</template>
