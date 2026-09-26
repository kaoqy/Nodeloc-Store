import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'

createApp(App).use(createPinia()).use(router).mount('#app')

// The inline splash in index.html covers the bundle, the lazy route chunk and the
// session check the first guard performs; then it fades out and leaves the document.
router.isReady().finally(() => {
  const boot = document.getElementById('boot')
  if (!boot) return
  boot.classList.add('boot-hide')
  window.setTimeout(() => boot.remove(), 300)
})
