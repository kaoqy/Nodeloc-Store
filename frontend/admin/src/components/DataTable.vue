<script setup lang="ts">
import AdminIcon from './AdminIcon.vue'
import PaginationFooter from './PaginationFooter.vue'

/**
 * 后台通用数据表格。
 *
 * 重构前 8 个列表页各写一份 table，导致：空状态文案不同、加载样式不同、
 * 小屏溢出处理不同、分页参数命名不同。这里把「加载中 / 出错 / 空 / 有数据」
 * 四种状态与分页收进一个组件，页面只负责列定义与行内容。
 */

export interface Column {
  /** 列标题；空字符串表示纯操作列 */
  label: string
  /** 数字列右对齐并使用等宽字体 */
  numeric?: boolean
  /** 操作列右对齐 */
  actions?: boolean
  /** 自定义宽度，例如 '180px' */
  width?: string
  /** 小屏隐藏的次要列 */
  hideOnMobile?: boolean
}

withDefaults(
  defineProps<{
    columns: Column[]
    loading?: boolean
    error?: string
    /** 空状态标题 */
    emptyTitle?: string
    /** 空状态说明 */
    emptyHint?: string
    /** 当前是否处于筛选状态（决定空状态是否给「清除筛选」） */
    filtered?: boolean
    /** 分页 */
    page?: number
    pages?: number
    total?: number
    summary?: string
  }>(),
  {
    loading: false,
    error: '',
    emptyTitle: '暂无数据',
    emptyHint: '',
    filtered: false,
    page: 1,
    pages: 1,
    total: 0,
    summary: '',
  },
)

const emit = defineEmits<{ retry: []; clearFilters: []; change: [page: number] }>()
</script>

<template>
  <div class="space-y-3">
    <!-- 出错：给出原因与重试，而不是留一片空白 -->
    <div v-if="error" class="card text-center">
      <p class="alert alert-danger text-left" role="alert">{{ error }}</p>
      <button class="btn btn-secondary mt-4" :disabled="loading" @click="emit('retry')">
        <span v-if="loading" class="spinner" />
        {{ loading ? '重试中…' : '重新加载' }}
      </button>
    </div>

    <div v-else-if="loading" class="card data-loading" role="status" aria-live="polite">
      <div class="data-loading-head">
        <span class="spinner" />
        <span>正在加载数据…</span>
      </div>
      <div class="space-y-2">
        <div v-for="i in 6" :key="i" class="skeleton h-9 w-full" />
      </div>
    </div>

    <div v-else-if="!total" class="card py-16 text-center">
      <span class="empty-state-glyph" aria-hidden="true">⌕</span>
      <p class="mt-4 font-semibold">{{ emptyTitle }}</p>
      <p v-if="emptyHint" class="mt-1.5 text-sm text-[var(--text-quiet)]">{{ emptyHint }}</p>
      <button v-if="filtered" class="btn btn-secondary btn-sm mt-6" @click="emit('clearFilters')">
        清除筛选
      </button>
      <slot name="empty-action" />
    </div>

    <template v-else>
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th
                v-for="(column, index) in columns"
                :key="index"
                :style="column.width ? { width: column.width } : undefined"
                :class="[
                  column.numeric ? 'nums' : '',
                  column.actions ? 'text-right' : '',
                  column.hideOnMobile ? 'hide-on-mobile' : '',
                ]"
              >
                {{ column.label }}
              </th>
            </tr>
          </thead>
          <tbody>
            <slot />
          </tbody>
        </table>
      </div>

      <PaginationFooter
        v-if="pages > 1"
        :page="page"
        :pages="pages"
        :loading="loading"
        :summary="summary || ('共 ' + total + ' 条')"
        @change="emit('change', $event)"
      />
      <p v-else-if="summary || total" class="quiet nums text-xs">{{ summary || ('共 ' + total + ' 条') }}</p>
    </template>
  </div>
</template>

<style scoped>
.data-loading {
  padding: 14px;
}

.data-loading-head {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  color: var(--text-dim);
  font-size: 12.5px;
}

.empty-state-glyph {
  display: grid;
  place-items: center;
  width: 44px;
  height: 44px;
  margin-inline: auto;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
  color: var(--text-quiet);
  font-size: 18px;
}
</style>
