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
    // Daily values for the mini trend line; fewer than two points hides it.
    spark?: number[] | null
  }>(),
  { accent: false, delta: null, spark: null },
)

const deltaMeta = computed(() => {
  if (props.delta === null || !Number.isFinite(props.delta)) return null
  const rounded = Math.round(props.delta * 10) / 10
  const abs = Math.abs(rounded)
  if (!rounded) return { text: '持平', tone: 'badge-neutral' }
  const text = `${rounded > 0 ? '↑' : '↓'} ${abs}%`
  return { text, tone: rounded > 0 ? 'badge-success' : 'badge-danger' }
})

const spark = computed(() => {
  const values = (props.spark ?? []).filter((item) => Number.isFinite(item))
  if (values.length < 2) return null
  const max = Math.max(...values)
  const min = Math.min(...values)
  const span = max - min || 1
  const step = 100 / (values.length - 1)
  const points = values.map(
    (value, index) => `${(index * step).toFixed(2)},${(26 - ((value - min) / span) * 22).toFixed(2)}`,
  )
  return { line: points.join(' '), area: `${points.join(' ')} 100,28 0,28` }
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
    <svg
      v-if="spark"
      class="mt-3 block h-7 w-full"
      :class="accent ? 'accent-text' : 'muted'"
      viewBox="0 0 100 28"
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
    >
      <polygon :points="spark.area" fill="currentColor" opacity="0.1" />
      <polyline
        :points="spark.line"
        fill="none"
        stroke="currentColor"
        stroke-width="1.5"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
        opacity="0.75"
      />
    </svg>
    <p v-if="hint" class="quiet mt-2.5 text-xs">{{ hint }}</p>
  </div>
</template>
