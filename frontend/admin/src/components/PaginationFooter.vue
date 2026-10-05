<script setup lang="ts">
withDefaults(
  defineProps<{
    page: number
    pages: number
    loading?: boolean
    // Free-form range text, e.g. 第 1–20 条 · 共 47 条. Omitted when empty.
    summary?: string
  }>(),
  { loading: false, summary: '' },
)

const emit = defineEmits<{ change: [page: number] }>()
</script>

<template>
  <div class="pagination-footer flex flex-wrap items-center justify-between gap-3">
    <p v-if="summary" class="quiet mono text-xs">{{ summary }}</p>
    <div class="flex items-center gap-2">
      <button
        class="btn btn-secondary btn-sm"
        :disabled="page <= 1 || loading"
        aria-label="上一页"
        @click="emit('change', page - 1)"
      >
        ← 上一页
      </button>
      <span class="muted mono text-sm nums" aria-current="page">{{ page }} / {{ pages }}</span>
      <button
        class="btn btn-secondary btn-sm"
        :disabled="page >= pages || loading"
        aria-label="下一页"
        @click="emit('change', page + 1)"
      >
        下一页 →
      </button>
    </div>
  </div>
</template>

<style scoped>
.pagination-footer {
  border-top: 1px solid var(--stroke-quiet);
  padding-top: 12px;
}

@media (max-width: 640px) {
  .pagination-footer {
    align-items: stretch;
  }

  .pagination-footer > div {
    width: 100%;
  }

  .pagination-footer .btn {
    flex: 1;
  }
}
</style>
