<script setup lang="ts">
/**
 * One outline icon set for the back office.
 *
 * A single component with a name prop keeps the sidebar and the top bar on the
 * same stroke weight and grid; the alternative — an SVG per call site — drifts
 * into three different line weights the moment anyone edits one of them.
 */
withDefaults(defineProps<{ name: string; size?: number }>(), { size: 18 })

// Every entry is the inner geometry of a 24x24 viewBox drawn with a 1.7 stroke,
// round caps and round joins (set on the <svg>), so none of them repeat that.
const paths: Record<string, string> = {
  dashboard: '<path d="M4 13h6V4H4zM14 20h6v-9h-6zM4 20h6v-4H4zM14 8h6V4h-6z" />',
  products: '<path d="M3 7.5 12 3l9 4.5v9L12 21l-9-4.5z" /><path d="M3 7.5 12 12l9-4.5M12 12v9" />',
  orders: '<path d="M6 3h9l4 4v14H6z" /><path d="M15 3v4h4" /><path d="M9 12h7M9 16h5" />',
  cards: '<rect x="3" y="6" width="18" height="12" rx="2" /><path d="M3 10h18M7 14h4" />',
  categories: '<path d="M4 6h6v6H4zM14 6h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z" />',
  coupons: '<path d="M3 9V7a1 1 0 0 1 1-1h16a1 1 0 0 1 1 1v2a3 3 0 0 0 0 6v2a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-2a3 3 0 0 0 0-6Z" /><path d="M12 8v8" stroke-dasharray="2 2.5" />',
  plugins: '<path d="M10 4h4v3a2 2 0 1 0 4 0h2v5h-3a2 2 0 1 0 0 4h3v5H4v-5h3a2 2 0 1 0 0-4H4V7h2a2 2 0 1 0 4 0z" />',
  users: '<path d="M16 20v-1.5a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4V20" /><circle cx="9.5" cy="7.5" r="3.5" /><path d="M17 11a3 3 0 1 0-2-5.2M21 20v-1.4a3.6 3.6 0 0 0-2.6-3.4" />',
  notifications: '<path d="M18 8A6 6 0 1 0 6 8c0 7-3 9-3 9h18s-3-2-3-9" /><path d="M13.7 21a2 2 0 0 1-3.4 0" />',
  roles: '<path d="M12 3l7 3v6c0 4.4-3 8-7 9-4-1-7-4.6-7-9V6z" /><path d="M9.3 12.2l1.9 1.9 3.5-3.6" />',
  logs: '<path d="M5 4h11l3 3v13H5z" /><path d="M8 10h8M8 14h8M8 17h5" />',
  settings: '<circle cx="12" cy="12" r="3" /><path d="M12 2.8v2.4M12 18.8v2.4M4.6 7.6l2 1.2M17.4 15.2l2 1.2M4.6 16.4l2-1.2M17.4 8.8l2-1.2" />',
  external: '<path d="M7 17 17 7M9 7h8v8" />',
  logout: '<path d="M15 4h3a1 1 0 0 1 1 1v14a1 1 0 0 1-1 1h-3" /><path d="M10 8l-4 4 4 4M6 12h9" />',
  chevron: '<path d="M9 6l6 6-6 6" />',
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
