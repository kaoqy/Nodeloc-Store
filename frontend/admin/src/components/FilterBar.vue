<script setup lang="ts">
/**
 * 统一的筛选区。搜索框、下拉与查询按钮在同一行，小屏自动换行；
 * 右侧留给「共 N 条」这类计数与主操作按钮。
 */
defineProps<{ count?: string }>()
</script>

<template>
  <div class="card flex flex-wrap items-center gap-2 !p-3">
    <slot />
    <span v-if="count" class="quiet nums ml-auto text-xs">{{ count }}</span>
    <div v-if="$slots.actions" class="flex flex-wrap gap-2" :class="count ? '' : 'ml-auto'">
      <slot name="actions" />
    </div>
  </div>
</template>
