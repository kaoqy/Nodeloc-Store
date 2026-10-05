<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description?: string
    eyebrow?: string
    /** Optional summary values shown above the working area. */
    metrics?: { label: string; value: string | number; hint?: string; tone?: 'default' | 'accent' | 'warning' | 'danger' | 'success' }[]
  }>(),
  { description: '', eyebrow: '', metrics: () => [] },
)
</script>

<template>
  <section class="management-page">
    <header class="management-page-head">
      <div class="min-w-0">
        <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
        <h1 class="management-page-title">{{ title }}</h1>
        <p v-if="description" class="management-page-description">{{ description }}</p>
        <slot name="meta" />
      </div>
      <div v-if="$slots.actions" class="management-page-actions">
        <slot name="actions" />
      </div>
    </header>

    <div v-if="metrics.length" class="management-metrics">
      <article v-for="metric in metrics" :key="metric.label" class="management-metric" :class="`tone-${metric.tone || 'default'}`">
        <p class="management-metric-label">{{ metric.label }}</p>
        <p class="management-metric-value nums">{{ metric.value }}</p>
        <p v-if="metric.hint" class="management-metric-hint">{{ metric.hint }}</p>
      </article>
    </div>

    <div v-if="$slots.default" class="management-page-body">
      <slot />
    </div>
  </section>
</template>

<style scoped>
.management-page {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
  min-width: 0;
}

.management-page-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4);
  border-bottom: 1px solid var(--stroke-quiet);
  padding-bottom: var(--space-5);
}

.management-page-title {
  margin-top: 4px;
  font-size: var(--text-title);
  font-weight: 760;
  line-height: 1.2;
}

.management-page-description {
  max-width: 72ch;
  margin-top: var(--space-2);
  color: var(--text-dim);
  font-size: var(--text-body);
  line-height: 1.7;
}

.management-page-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

.management-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--space-3);
}

.management-metric {
  min-width: 0;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--surface) 95%, transparent);
  padding: var(--space-4);
}

.management-metric-label,
.management-metric-hint {
  color: var(--text-quiet);
  font-size: var(--text-caption);
}

.management-metric-value {
  margin-top: 3px;
  font-size: 1.45rem;
  font-weight: 750;
  line-height: 1.2;
}

.management-metric.tone-accent .management-metric-value { color: var(--accent); }
.management-metric.tone-warning .management-metric-value { color: var(--warning); }
.management-metric.tone-danger .management-metric-value { color: var(--danger); }
.management-metric.tone-success .management-metric-value { color: var(--success); }

.management-page-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  min-width: 0;
}

@media (max-width: 640px) {
  .management-page {
    gap: var(--space-4);
  }

  .management-page-head {
    align-items: flex-start;
  }

  .management-page-actions {
    width: 100%;
  }

  .management-page-actions :deep(.btn) {
    flex: 1;
  }

  .management-metrics {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
