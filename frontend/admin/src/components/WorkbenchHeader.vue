<script setup lang="ts">
withDefaults(
  defineProps<{
    title: string
    description?: string
    eyebrow?: string
    backTo?: string
    backLabel?: string
  }>(),
  { description: '', eyebrow: '', backTo: '', backLabel: '返回列表' },
)
</script>

<template>
  <header class="workbench-header">
    <div class="workbench-header-main">
      <RouterLink v-if="backTo" :to="backTo" class="workbench-back">
        ← {{ backLabel }}
      </RouterLink>
      <p v-if="eyebrow" class="eyebrow">{{ eyebrow }}</p>
      <h1 class="workbench-title">{{ title }}</h1>
      <p v-if="description" class="workbench-description">{{ description }}</p>
      <slot name="meta" />
    </div>
    <div v-if="$slots.actions" class="workbench-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<style scoped>
.workbench-header {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4);
  border-bottom: 1px solid var(--stroke-quiet);
  padding-bottom: var(--space-5);
}

.workbench-back {
  display: inline-flex;
  margin-bottom: var(--space-2);
  color: var(--text-quiet);
  font-size: var(--text-caption);
  transition: color var(--fast);
}

.workbench-back:hover {
  color: var(--text);
}

.workbench-title {
  margin-top: 4px;
  overflow: hidden;
  font-size: var(--text-title);
  font-weight: 760;
  line-height: 1.2;
  text-overflow: ellipsis;
}

.workbench-description {
  max-width: 72ch;
  margin-top: var(--space-2);
  color: var(--text-dim);
  font-size: var(--text-body);
  line-height: 1.7;
}

.workbench-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2);
}

@media (max-width: 640px) {
  .workbench-header {
    align-items: flex-start;
  }

  .workbench-actions {
    width: 100%;
  }

  .workbench-actions :deep(.btn) {
    flex: 1;
  }
}
</style>
