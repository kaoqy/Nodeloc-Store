<script setup lang="ts">
import { progressing, requestPending } from '../utils/progress'
</script>

<template>
  <Transition name="bar">
    <div v-if="progressing" class="bar" role="progressbar" aria-label="正在加载" aria-busy="true">
      <span class="bar-fill" />
    </div>
  </Transition>
  <Transition name="request">
    <div v-if="requestPending" class="request-indicator" role="status" aria-live="polite">
      <span class="spinner !size-3.5" />
      正在加载…
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

.request-indicator {
  position: fixed;
  right: 18px;
  bottom: 18px;
  z-index: 70;
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-pill);
  background: var(--glass);
  padding: 7px 12px;
  color: var(--text-dim);
  font-size: 12px;
  box-shadow: var(--shadow-md);
  backdrop-filter: blur(12px);
}
.request-enter-active,
.request-leave-active { transition: opacity 160ms var(--ease), transform 160ms var(--ease); }
.request-enter-from,
.request-leave-to { opacity: 0; transform: translateY(6px); }

@media (prefers-reduced-motion: reduce) {
  .bar-fill { animation: none; width: 100%; opacity: 0.55; }
}
</style>
