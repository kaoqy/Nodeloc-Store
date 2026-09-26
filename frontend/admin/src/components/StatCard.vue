<script setup lang="ts">
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    label: string
    value: string | number
    hint?: string
    accent?: boolean
    // Percentage change against the previous period. null hides the pill, and
    // a zero delta reads as flat rather than as a drop.
    delta?: number | null
  }>(),
  { accent: false, delta: null },
)

const deltaMeta = computed(() => {
  if (props.delta === null || !Number.isFinite(props.delta)) return null
  const rounded = Math.round(props.delta * 10) / 10
  const abs = Math.abs(rounded)
  if (!rounded) return { text: '持平', tone: 'badge-neutral' }
  const text = `${rounded > 0 ? '↑' : '↓'} ${abs}%`
  return { text, tone: rounded > 0 ? 'badge-success' : 'badge-danger' }
})
</script>

<template>
  <div class="card !p-5">
    <div class="flex items-start justify-between gap-2">
      <p class="eyebrow">{{ label }}</p>
      <span v-if="deltaMeta" class="badge nums" :class="deltaMeta.tone">{{ deltaMeta.text }}</span>
    </div>
    <p class="nums mt-3 text-[28px] font-bold leading-none tracking-tight" :class="accent ? 'accent-text' : ''">
      {{ value }}
    </p>
    <p v-if="hint" class="quiet mt-2.5 text-xs">{{ hint }}</p>
  </div>
</template>
