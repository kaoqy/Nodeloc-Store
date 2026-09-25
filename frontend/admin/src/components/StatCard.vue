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
  <div class="sheen card-hover group !p-5">
    <div class="flex items-start justify-between">
      <div>
        <p class="text-sm text-[#7a7890]">{{ title }}</p>
        <p class="mt-2 text-2xl font-black tracking-tight">{{ value }}</p>
        <p v-if="subtitle" class="mt-1 text-xs text-[#7a7890]">{{ subtitle }}</p>
      </div>
      <div
        v-if="icon"
        class="flex h-11 w-11 items-center justify-center rounded-2xl border border-white/[0.14] bg-gradient-to-br from-indigo-500/25 to-purple-500/15 text-lg shadow-[inset_0_1px_0_rgba(255,255,255,0.18)] backdrop-blur-md transition-transform duration-300 group-hover:scale-110 group-hover:-rotate-6"
      >
        {{ icon }}
      </div>
    </div>
    <div v-if="trend" class="mt-3 flex items-center gap-1 text-xs">
      <span v-if="trend === 'up'" class="text-[#34d399]">↑</span>
      <span v-if="trend === 'down'" class="text-[#fb7185]">↓</span>
      <span v-if="trend === 'neutral'" class="text-[#7a7890]">→</span>
      <span :class="trend === 'up' ? 'text-[#34d399]' : trend === 'down' ? 'text-[#fb7185]' : 'text-[#7a7890]'">
        {{ trend === 'up' ? '增长' : trend === 'down' ? '下降' : '持平' }}
      </span>
    </div>
  </div>
</template>
