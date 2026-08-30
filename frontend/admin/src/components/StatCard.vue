<script setup lang="ts">
defineProps<{
  title: string
  value: string | number
  subtitle?: string
  icon?: string
  trend?: 'up' | 'down' | 'neutral'
}>()
</script>

<template>
  <div class="card-hover group">
    <div class="flex items-start justify-between">
      <div>
        <p class="text-sm text-[#6b6b80]">{{ title }}</p>
        <p class="mt-2 text-2xl font-bold tracking-tight">{{ value }}</p>
        <p v-if="subtitle" class="mt-1 text-xs text-[#6b6b80]">{{ subtitle }}</p>
      </div>
      <div
        v-if="icon"
        class="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-indigo-500/20 to-purple-500/20 text-lg text-indigo-300 shadow-sm"
      >
        {{ icon }}
      </div>
    </div>
    <div v-if="trend" class="mt-3 flex items-center gap-1 text-xs">
      <span v-if="trend === 'up'" class="text-[#22c55e]">↑</span>
      <span v-if="trend === 'down'" class="text-[#ef4444]">↓</span>
      <span v-if="trend === 'neutral'" class="text-[#6b6b80]">→</span>
      <span :class="trend === 'up' ? 'text-[#22c55e]' : trend === 'down' ? 'text-[#ef4444]' : 'text-[#6b6b80]'">
        {{ trend === 'up' ? '增长' : trend === 'down' ? '下降' : '持平' }}
      </span>
    </div>
  </div>
</template>
