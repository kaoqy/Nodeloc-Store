<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const router = useRouter()
const auth = useAuthStore()
const identifier = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(identifier.value, password.value)
    await router.push('/')
  } catch (err: any) {
    error.value = err.response?.data?.message || '登录失败，请检查账号和密码'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <main class="flex min-h-screen items-center justify-center bg-[radial-gradient(circle_at_top,_#312e8155,_transparent_45%)] p-4">
    <form class="glass w-full max-w-md rounded-3xl p-7 sm:p-9" @submit.prevent="submit">
      <div class="mb-8 text-center">
        <div class="mx-auto mb-4 w-fit rounded-2xl bg-gradient-to-br from-indigo-500 to-fuchsia-500 px-4 py-3 text-xl font-bold shadow-lg shadow-indigo-500/20">NL</div>
        <h1 class="text-2xl font-bold">管理员登录</h1>
        <p class="mt-2 text-sm text-slate-400">登录 Nodeloc Store 管理后台</p>
      </div>
      <div class="space-y-5">
        <label class="block">
          <span class="mb-2 block text-sm text-slate-300">账号或邮箱</span>
          <input v-model.trim="identifier" class="field" autocomplete="username" placeholder="请输入账号或邮箱" required>
        </label>
        <label class="block">
          <span class="mb-2 block text-sm text-slate-300">密码</span>
          <input v-model="password" class="field" type="password" autocomplete="current-password" placeholder="请输入密码" required>
        </label>
        <p v-if="error" class="rounded-xl border border-rose-500/20 bg-rose-500/10 px-3 py-2 text-sm text-rose-300">{{ error }}</p>
        <button class="btn-primary w-full" :disabled="loading">
          {{ loading ? '登录中…' : '登录' }}
        </button>
      </div>
    </form>
  </main>
</template>
