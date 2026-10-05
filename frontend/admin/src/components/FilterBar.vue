<script setup lang="ts">
/**
 * 统一的筛选区。搜索框、下拉与查询按钮在同一行，小屏自动换行；
 * 右侧留给「共 N 条」这类计数与主操作按钮。
 */
defineProps<{ count?: string }>()
</script>

<template>
  <div class="card filter-bar !p-3">
    <div class="filter-bar-fields">
      <slot />
    </div>
    <div class="filter-bar-tail">
      <span v-if="count" class="quiet nums text-xs">{{ count }}</span>
      <div v-if="$slots.actions" class="flex flex-wrap gap-2">
        <slot name="actions" />
      </div>
    </div>
  </div>
</template>

<style scoped>
.filter-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  background: color-mix(in srgb, var(--surface) 94%, transparent);
  backdrop-filter: blur(8px);
}

.filter-bar-fields,
.filter-bar-tail {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-2);
}

.filter-bar-tail {
  margin-left: auto;
}

@media (max-width: 640px) {
  .filter-bar,
  .filter-bar-fields,
  .filter-bar-tail {
    width: 100%;
  }

  .filter-bar-tail {
    margin-left: 0;
    justify-content: space-between;
  }

  .filter-bar :deep(.input) {
    max-width: 100%;
  }

  .filter-bar-fields > * {
    flex: 1 1 10rem;
  }

  .filter-bar-fields > .btn {
    flex: 0 0 auto;
  }

  .filter-bar-tail {
    gap: var(--space-2);
  }

  .filter-bar-tail :deep(.btn) {
    flex: 1;
  }
}
</style>
