<script setup lang="ts">
/**
 * 统一的页面标题区。
 *
 * 之前每个页面自己写一遍标题 + 说明 + 按钮，字号、间距、按钮位置各不相同，
 * 有的页面干脆没有说明。现在所有后台页面都从这里开始，保证同一套层级。
 */
withDefaults(
  defineProps<{
    title: string
    description?: string
    /** 是否显示底部细分隔线 */
    bordered?: boolean
  }>(),
  { description: '', bordered: false },
)
</script>

<template>
  <div class="page-head" :class="bordered ? 'page-head-bordered' : ''">
    <div class="min-w-0">
      <h2 class="truncate">{{ title }}</h2>
      <p v-if="description" class="page-head-description quiet mt-1 max-w-3xl text-[12.5px] leading-relaxed">{{ description }}</p>
      <slot name="meta" />
    </div>
    <div v-if="$slots.actions" class="page-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.page-head-bordered {
  border-bottom: 1px solid var(--stroke-quiet);
  padding-bottom: 16px;
}

.page-head :deep(.page-head-description) {
  line-height: 1.7;
}
</style>
