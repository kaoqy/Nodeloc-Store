<script setup lang="ts">
import { progressing } from '../utils/progress'
</script>

<template>
  <Transition name="bar">
    <div v-if="progressing" class="bar" role="progressbar" aria-label="正在加载" aria-busy="true">
      <span class="bar-fill" />
    </div>
  </Transition>
</template>

<style scoped>
.bar {
  position: fixed;
  inset: 0 0 auto;
  height: 2px;
  z-index: 80;
  overflow: hidden;
  background: color-mix(in srgb, var(--accent) 14%, transparent);
}
.bar-fill {
  position: absolute;
  inset: 0;
  width: 35%;
  background: var(--accent);
  animation: bar-slide 1s ease-in-out infinite;
}
@keyframes bar-slide {
  0% { transform: translateX(-100%); }
  100% { transform: translateX(320%); }
}
.bar-enter-active,
.bar-leave-active { transition: opacity 180ms var(--ease); }
.bar-enter-from,
.bar-leave-to { opacity: 0; }

@media (prefers-reduced-motion: reduce) {
  .bar-fill { animation: none; width: 100%; opacity: 0.55; }
}
</style>
