<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listNotifications, markAsRead } from '../api/notifications'
import type { Notification } from '../types'

const loading = ref(true)
const notifications = ref<Notification[]>([])

async function load() {
  loading.value = true
  try {
    const result = await listNotifications()
    notifications.value = result.data
  } finally {
    loading.value = false
  }
}

async function markRead(id: number) {
  await markAsRead(id)
  await load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <p class="text-sm text-[#6b6b80]">系统通知</p>

    <div class="space-y-2">
      <div v-if="loading" class="space-y-2">
        <div v-for="i in 5" :key="i" class="skeleton h-16" />
      </div>
      <div v-else-if="!notifications.length" class="py-12 text-center text-[#6b6b80]">暂无通知</div>
      <div
        v-for="notif in notifications"
        :key="notif.id"
        :class="['card-hover flex items-start gap-4', !notif.is_read && 'border-indigo-500/30 bg-indigo-500/5']"
      >
        <div class="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-[#1a1a25] text-lg">
          🔔
        </div>
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <h4 class="text-sm font-medium">{{ notif.title }}</h4>
            <span v-if="!notif.is_read" class="h-2 w-2 rounded-full bg-indigo-500" />
          </div>
          <p class="mt-1 text-sm text-[#a1a1b5]">{{ notif.content }}</p>
          <p class="mt-2 text-xs text-[#6b6b80]">{{ notif.created_at }}</p>
        </div>
        <button v-if="!notif.is_read" class="btn-ghost text-xs" @click="markRead(notif.id)">标为已读</button>
      </div>
    </div>
  </section>
</template>
