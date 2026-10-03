<script setup lang="ts">
/**
 * 后台唯一的图标集。
 *
 * 全部图标共用同一个 24×24 网格、1.7 描边与圆角端点，因此在任何组合里
 * 线条粗细都一致；换成每个调用点自己写 SVG 的话，改一处就会漂移。
 */
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 18 })

const paths: Record<string, string> = {
  dashboard: '<path d="M4 13h6V4H4zM14 20h6v-9h-6zM4 20h6v-4H4zM14 8h6V4h-6z" />',
  orders: '<path d="M6 3h9l4 4v14H6z" /><path d="M15 3v4h4" /><path d="M9 12h7M9 16h5" />',
  products: '<path d="M3 7.5 12 3l9 4.5v9L12 21l-9-4.5z" /><path d="M3 7.5 12 12l9-4.5M12 12v9" />',
  cards: '<rect x="3" y="6" width="18" height="12" rx="2" /><path d="M3 10h18M7 14h4" />',
  categories: '<path d="M4 6h6v6H4zM14 6h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z" />',
  coupons: '<path d="M3 9V7a1 1 0 0 1 1-1h16a1 1 0 0 1 1 1v2a3 3 0 0 0 0 6v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-2a3 3 0 0 0 0-6Z" /><path d="M12 8v8" stroke-dasharray="2 2.5" />',
  activities: '<path d="M4 12h4l2-5 3 10 2-5h5" /><path d="M4 20h16" />',
  plugins: '<path d="M10 4h4v3a2 2 0 1 0 4 0h2v5h-3a2 2 0 1 0 0 4h3v5H4v-5h3a2 2 0 1 0 0-4H4V7h2a2 2 0 1 0 4 0z" />',
  support: '<path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v7a2.5 2.5 0 0 1-2.5 2.5H12l-4.5 3.4V16H6.5A2.5 2.5 0 0 1 4 13.5z" /><path d="M8.5 9.5h7M8.5 12.5h4.5" />',
  knowledge: '<path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H19v15H6.5A2.5 2.5 0 0 0 4 20.5z" /><path d="M4 5.5v15M8 7.5h7M8 11h5" />',
  users: '<path d="M16 20v-1.5a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4V20" /><circle cx="9.5" cy="7.5" r="3.5" /><path d="M17 11a3 3 0 1 0-2-5.2M21 20v-1.4a3.6 3.6 0 0 0-2.6-3.4" />',
  notifications: '<path d="M18 8A6 6 0 1 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" /><path d="M13.7 21a2 2 0 0 1-3.4 0" />',
  settings: '<circle cx="12" cy="12" r="3" /><path d="M12 2.8v2.4M12 18.8v2.4M4.6 7.6l2 1.2M17.4 15.2l2 1.2M4.6 16.4l2-1.2M17.4 8.8l2-1.2" />',
  roles: '<path d="M12 3l7 3v6c0 4.4-3 8-7 9-4-1-7-4.6-7-9V6z" /><path d="M9.3 12.2l1.9 1.9 3.5-3.6" />',
  logs: '<path d="M5 4h11l3 3v13H5z" /><path d="M8 10h8M8 14h8M8 17h5" />',
  search: '<circle cx="11" cy="11" r="6.5" /><path d="M16 16l4 4" />',
  menu: '<path d="M4 7h16M4 12h16M4 17h16" />',
  close: '<path d="M6 6l12 12M18 6L6 18" />',
  chevronDown: '<path d="M6 9l6 6 6-6" />',
  chevronRight: '<path d="M9 6l6 6-6 6" />',
  chevronLeft: '<path d="M15 6l-6 6 6 6" />',
  sun: '<circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />',
  moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />',
  external: '<path d="M7 17 17 7M9 7h8v8" />',
  logout: '<path d="M15 4h3a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-3" /><path d="M10 8l-4 4 4 4M6 12h9" />',
  refresh: '<path d="M20 11a8 8 0 1 0-2.3 5.7" /><path d="M20 4v7h-7" />',
  plus: '<path d="M12 5v14M5 12h14" />',
  check: '<path d="M5 12.5l4.5 4.5L19 7" />',
  alert: '<path d="M12 3l9 16H3z" /><path d="M12 9v5M12 17h.01" />',
  clock: '<circle cx="12" cy="12" r="9" /><path d="M12 7v5l3.5 2" />',
  user: '<circle cx="12" cy="8" r="4" /><path d="M4 20a8 8 0 0 1 16 0" />',
  history: '<path d="M12 8v5l3 2" /><circle cx="12" cy="12" r="9" />',
  download: '<path d="M12 4v11M7.5 11l4.5 4.5 4.5-4.5" /><path d="M5 20h14" />',
  copy: '<rect x="9" y="9" width="11" height="11" rx="2" /><path d="M5 15V5h10" />',
  filter: '<path d="M3 5h18l-7 8v6l-4-2v-4z" />',
}
</script>

<template>
  <svg
    :width="size"
    :height="size"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.7"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
    focusable="false"
    v-html="paths[name] || ''"
  />
</template>
