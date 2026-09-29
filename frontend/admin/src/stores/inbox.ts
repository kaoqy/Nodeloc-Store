import { ref } from 'vue'
import { defineStore } from 'pinia'
import { unreadCount } from '../api/notifications'

/**
 * The sidebar dot reads from here, so the 通知中心 entry and the inbox page agree
 * on one number instead of each counting what they happened to load. Restock
 * warnings arrive on a background sweep, so nobody's click produces them: the
 * count is refreshed when the back office is opened and when its own screens
 * change.
 */
export const useInboxStore = defineStore('inbox', () => {
  const unread = ref(0)

  async function refresh() {
    try {
      unread.value = await unreadCount()
    } catch {
      // A missing dot is not worth an error banner; the inbox page itself still
      // reports trouble when the operator opens it.
    }
  }

  function reset(value = 0) {
    unread.value = value
  }

  return { unread, refresh, reset }
})
