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
  <div class="flex min-h-screen">
    <!-- Left side - branding -->
    <div class="hidden lg:flex lg:w-1/2 items-center justify-center bg-gradient-to-br from-indigo-900/40 via-purple-900/30 to-[#0a0a0f] p-12">
      <div class="max-w-md text-center">
        <div class="mx-auto mb-8 flex h-20 w-20 items-center justify-center rounded-3xl bg-gradient-to-br from-indigo-500 to-purple-600 text-3xl font-bold shadow-2xl shadow-indigo-500/30">
          NL
        </div>
        <h1 class="text-4xl font-bold tracking-tight">Nodeloc Store</h1>
        <p class="mt-4 text-lg text-[#a1a1b5]">数字商品交易平台</p>
        <div class="mt-8 grid grid-cols-3 gap-4 text-center">
          <div class="card">
            <p class="text-2xl font-bold">🛍️</p>
            <p class="mt-1 text-xs text-[#6b6b80]">商品管理</p>
          </div>
          <div class="card">
            <p class="text-2xl font-bold">📦</p>
            <p class="mt-1 text-xs text-[#6b6b80]">订单处理</p>
          </div>
          <div class="card">
            <p class="text-2xl font-bold">🔑</p>
            <p class="mt-1 text-xs text-[#6b6b80]">卡密交付</p>
          </div>
        </div>
      </div>
    </div>

    <!-- Right side - login form -->
    <div class="flex w-full lg:w-1/2 items-center justify-center p-8">
      <div class="w-full max-w-sm">
        <div class="mb-8 text-center lg:hidden">
          <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-500 to-purple-600 text-xl font-bold shadow-lg shadow-indigo-500/20">
            NL
          </div>
          <h1 class="text-2xl font-bold">Nodeloc Store</h1>
        </div>

        <h2 class="text-2xl font-bold tracking-tight">管理员登录</h2>
        <p class="mt-2 text-sm text-[#6b6b80]">登录管理后台</p>

        <form class="mt-8 space-y-5" @submit.prevent="submit">
          <div>
            <label class="mb-2 block text-sm font-medium">账号或邮箱</label>
            <input v-model.trim="identifier" class="input" autocomplete="username" placeholder="请输入账号或邮箱" required />
          </div>
          <div>
            <label class="mb-2 block text-sm font-medium">密码</label>
            <input v-model="password" type="password" class="input" autocomplete="current-password" placeholder="请输入密码" required />
          </div>

          <p v-if="error" class="rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-300">
            {{ error }}
          </p>

          <button type="submit" class="btn-primary w-full py-3" :disabled="loading">
            {{ loading ? '登录中…' : '登录' }}
          </button>
        </form>

        <p class="mt-6 text-center text-sm text-[#6b6b80]">
          使用 NodeLoc 账号登录
        </p>
      </div>
    </div>
  </div>
</template>
