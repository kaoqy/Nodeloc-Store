import { ref } from 'vue'
import { defineStore } from 'pinia'
import { unreadCount } from '../api/notifications'

/**
 * The header badge reads from here so 顶栏 and 个人中心 agree on one number
 * instead of each counting the page of messages they happened to load.
 */
export const useInboxStore = defineStore('inbox', () => {
  const unread = ref(0)

  async function refresh() {
    try {
      unread.value = await unreadCount()
    } catch {
      // A missing badge is not worth an error banner; the inbox itself still
      // reports trouble when the buyer opens it.
    }
  }

  function reset(value = 0) {
    unread.value = value
  }

  return { unread, refresh, reset }
})
