import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { NAV_ITEMS, titleOf } from '../navigation'

const STORAGE_KEY = 'admin.recent'
const MAX = 6

export interface RecentEntry {
  path: string
  label: string
  at: number
}

/**
 * 最近访问：把店主当天走过的页面记下来，侧栏底部给出快捷路径。
 * 只保留有中文标题的页面，详情页（订单号/用户 ID）不记——那些是某一笔
 * 数据，不是「功能入口」，重复出现只会挤掉真正常用的页面。
 */
export const useRecentStore = defineStore('recent', () => {
  const entries = ref<RecentEntry[]>(read())

  function read(): RecentEntry[] {
    try {
      const raw = localStorage.getItem(STORAGE_KEY)
      if (!raw) return []
      const parsed = JSON.parse(raw) as RecentEntry[]
      return Array.isArray(parsed) ? parsed.slice(0, MAX) : []
    } catch {
      return []
    }
  }

  function persist() {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(entries.value))
  }

  function visit(path: string) {
    // 详情页与子表单不进最近访问，见上方说明。
    const known = NAV_ITEMS.find((item) => item.path === path)
    if (!known) return
    const rest = entries.value.filter((item) => item.path !== path)
    entries.value = [{ path, label: known.label, at: Date.now() }, ...rest].slice(0, MAX)
    persist()
  }

  function clear() {
    entries.value = []
    persist()
  }

  const list = computed(() => entries.value)

  return { entries, list, visit, clear, titleOf }
})
