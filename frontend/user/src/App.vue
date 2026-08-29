<script setup lang="ts">
import { onMounted } from 'vue'
import NavBar from './components/NavBar.vue'
import { useAuthStore } from './stores/auth'

const auth = useAuthStore()

onMounted(async () => {
  if (auth.token && !auth.user) {
    try {
      await auth.fetchUser()
    } catch {
      // 认证失败由拦截器和 auth store 统一处理
    }
  }
})
</script>

<template>
  <div class="min-h-screen bg-zinc-950 text-zinc-100">
    <NavBar />
    <main>
      <router-view />
    </main>
  </div>
</template>
