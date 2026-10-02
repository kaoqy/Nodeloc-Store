<script setup lang="ts">
import { computed } from 'vue'
import AdminIcon from './AdminIcon.vue'

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
    // Icon name from AdminIcon, drawn in the corner tile.
    icon?: string
  }>(),
  { accent: false, delta: null, spark: null, icon: '' },
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
    (value, index) => `${(index * step).toFixed(2)},${(27 - ((value - min) / span) * 22).toFixed(2)}`,
  )
  return { line: points.join(' '), area: `${points.join(' ')} 100,30 0,30` }
})
</script>

<template>
  <div class="card stat-card !p-5">
    <div class="flex items-start justify-between gap-3">
      <div class="min-w-0">
        <p class="eyebrow">{{ label }}</p>
        <p class="nums mt-2.5 text-[27px] font-bold leading-none tracking-tight" :class="accent ? 'accent-text' : ''">
          {{ value }}
        </p>
      </div>
      <span v-if="icon" class="stat-icon" :class="accent ? 'stat-icon-accent' : ''">
        <AdminIcon :name="icon" :size="17" />
      </span>
    </div>

    <svg
      v-if="spark"
      class="mt-3.5 block h-8 w-full"
      :class="accent ? 'accent-text' : 'muted'"
      viewBox="0 0 100 30"
      preserveAspectRatio="none"
      aria-hidden="true"
      focusable="false"
    >
      <polygon :points="spark.area" fill="currentColor" opacity="0.12" />
      <polyline
        :points="spark.line"
        fill="none"
        stroke="currentColor"
        stroke-width="1.6"
        stroke-linejoin="round"
        stroke-linecap="round"
        vector-effect="non-scaling-stroke"
        opacity="0.8"
      />
    </svg>

    <div class="mt-3 flex flex-wrap items-center gap-2">
      <span v-if="deltaMeta" class="badge nums" :class="deltaMeta.tone">{{ deltaMeta.text }}</span>
      <span v-if="hint" class="quiet text-xs">{{ hint }}</span>
    </div>
  </div>
</template>

<style scoped>
/* The sparkline was measured against a 28-unit box; the card now draws 30. */
.stat-card { display: flex; flex-direction: column; position: relative; }
.stat-icon {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  border: 1px solid var(--stroke);
  background: var(--surface-sunken);
  color: var(--text-dim);
}
.stat-icon-accent {
  border-color: var(--accent-line);
  background: var(--accent-soft);
  color: var(--accent);
}
</style>
