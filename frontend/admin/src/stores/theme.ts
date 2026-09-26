import { ref } from 'vue'
import { defineStore } from 'pinia'

export type Theme = 'dark' | 'light'

const stored = localStorage.getItem('store-theme') as Theme | null

export const useThemeStore = defineStore('theme', () => {
  const theme = ref<Theme>(stored || (window.matchMedia('(prefers-color-scheme: light)').matches ? 'light' : 'dark'))

  function apply(value: Theme) {
    document.documentElement.dataset.theme = value
    const meta = document.querySelector('meta[name="theme-color"]')
    if (meta) meta.setAttribute('content', value === 'light' ? '#f6f4f1' : '#0a0b0d')
  }

  function toggle() {
    theme.value = theme.value === 'dark' ? 'light' : 'dark'
    localStorage.setItem('store-theme', theme.value)
    apply(theme.value)
  }

  apply(theme.value)

  return { theme, toggle }
})
