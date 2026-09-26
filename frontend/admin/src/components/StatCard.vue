<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    value: string | number
    hint?: string
    accent?: boolean
  }>(),
  { accent: false },
)
</script>

<template>
  <div class="card !p-5">
    <p class="eyebrow">{{ label }}</p>
    <p class="nums mt-3 text-[28px] font-bold leading-none tracking-tight" :class="accent ? 'accent-text' : ''">
      {{ value }}
    </p>
    <p v-if="hint" class="quiet mt-2.5 text-xs">{{ hint }}</p>
  </div>
</template>
