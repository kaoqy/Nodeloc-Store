<script setup lang="ts">
import { onUnmounted, watch } from 'vue'
import AdminIcon from './AdminIcon.vue'

/**
 * 统一抽屉。用于「不离开列表就能完成的操作」——编辑配置、查看记录、
 * 处理工单。宽度按用途固定，小屏自动占满。
 */
const props = withDefaults(defineProps<{ open: boolean; title: string; width?: 'sm' | 'md' | 'lg' }>(), {
  width: 'md',
})
const emit = defineEmits<{ close: [] }>()

const widthClass = {
  sm: 'sm:max-w-md',
  md: 'sm:max-w-xl',
  lg: 'sm:max-w-3xl',
}[props.width]

// 抽屉打开时锁滚动，并允许 Esc 关闭：长表单里滚到底再关会跳到页面顶部，
// 锁住 body 才是可预期的行为。
function onKey(event: KeyboardEvent) {
  if (event.key === 'Escape') emit('close')
}

watch(
  () => props.open,
  (open) => {
    document.body.style.overflow = open ? 'hidden' : ''
    if (open) window.addEventListener('keydown', onKey)
    else window.removeEventListener('keydown', onKey)
  },
)

onUnmounted(() => {
  document.body.style.overflow = ''
  window.removeEventListener('keydown', onKey)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="drawer">
      <div v-if="open" class="fixed inset-0 z-50 flex justify-end">
        <div class="scrim flex-1" @click="emit('close')" />
        <aside
          class="drawer-panel flex h-full w-full flex-col border-l border-[var(--stroke)] bg-[var(--surface)]"
          :class="widthClass"
          role="dialog"
          aria-modal="true"
          :aria-label="title"
        >
          <header class="flex h-[60px] shrink-0 items-center gap-3 border-b border-[var(--stroke)] px-5">
            <h3 class="min-w-0 flex-1 truncate text-[15px] font-bold">{{ title }}</h3>
            <button class="icon-btn" type="button" aria-label="关闭" @click="emit('close')">
              <AdminIcon name="close" :size="18" />
            </button>
          </header>
          <div class="min-h-0 flex-1 overflow-y-auto p-5">
            <slot />
          </div>
          <footer v-if="$slots.footer" class="shrink-0 border-t border-[var(--stroke)] bg-[var(--surface)] p-4">
            <slot name="footer" />
          </footer>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer-panel { box-shadow: var(--shadow-lg); }
.drawer-enter-active, .drawer-leave-active { transition: opacity 180ms var(--ease); }
.drawer-enter-active .drawer-panel, .drawer-leave-active .drawer-panel {
  transition: transform 220ms var(--spring);
}
.drawer-enter-from, .drawer-leave-to { opacity: 0; }
.drawer-enter-from .drawer-panel, .drawer-leave-to .drawer-panel { transform: translateX(24px); }
</style>
