<script setup lang="ts">
import { computed } from 'vue'

/**
 * 统一的状态标签。把「订单状态 / 工单状态 / 卡片状态 / 活动状态」的配色
 * 收在一处，避免同一个「已解决」在两张页面上是两种颜色。
 */
const props = defineProps<{ value: string; label?: string; tone?: string }>()

// 各业务的状态 → 语义色调。没登记的按中性处理。
const TONES: Record<string, string> = {
  // 通用
  ok: 'badge-success', success: 'badge-success', done: 'badge-success',
  published: 'badge-success', available: 'badge-success', delivered: 'badge-success',
  completed: 'badge-success', paid: 'badge-success', active: 'badge-success',
  resolved: 'badge-success', closed: 'badge-neutral', used: 'badge-neutral',
  // 进行中
  pending: 'badge-warning', processing: 'badge-info', running: 'badge-success',
  waiting_user: 'badge-warning', human_handling: 'badge-info',
  pending_human: 'badge-warning', user_requested_human: 'badge-warning',
  waiting_confirm: 'badge-warning', manual_pending: 'badge-warning',
  waiting_stock: 'badge-warning', plugin_pending: 'badge-info', scheduled: 'badge-info',
  draft: 'badge-neutral', paused: 'badge-warning', archived: 'badge-neutral',
  // 异常
  failed: 'badge-danger', error: 'badge-danger', rejected: 'badge-danger',
  cancelled: 'badge-neutral', disabled: 'badge-neutral', refunded: 'badge-info',
  offline: 'badge-neutral', ended: 'badge-neutral',
  // 工单优先级
  low: 'badge-neutral', normal: 'badge-neutral', high: 'badge-warning', urgent: 'badge-danger',
}

const tone = computed(() => props.tone ?? TONES[props.value] ?? 'badge-neutral')
</script>

<template>
  <span class="badge" :class="tone">
    <span class="status-dot" aria-hidden="true" />
    {{ label || value }}
  </span>
</template>

<style scoped>
.status-dot {
  width: 5px;
  height: 5px;
  flex-shrink: 0;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.8;
}
</style>
