import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import './style.css'

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.mount('#app')

// The inline splash in index.html covers bundle download plus the first route;
// once that resolves it fades out and leaves the document.
router.isReady().finally(() => {
  const boot = document.getElementById('boot')
  if (!boot) return
  boot.classList.add('boot-hide')
  window.setTimeout(() => boot.remove(), 300)
})
