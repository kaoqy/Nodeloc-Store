<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { oauthInitiate } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const identifier = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(identifier.value, password.value)
    await router.push(typeof route.query.redirect === 'string' ? route.query.redirect : '/')
  } catch {
    error.value = '登录失败，请检查账号和密码'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="flex min-h-screen">
    <!-- Left side - branding -->
    <div class="hidden lg:flex lg:w-1/2 items-center justify-center bg-gradient-to-br from-purple-900/30 via-indigo-900/20 to-[#09090b] p-12">
      <div class="max-w-md text-center">
        <div class="mx-auto mb-8 flex h-20 w-20 items-center justify-center rounded-3xl bg-gradient-to-br from-purple-500 to-indigo-600 text-3xl font-bold shadow-2xl shadow-purple-500/30">N</div>
        <h1 class="text-4xl font-black tracking-tight">Nodeloc Store</h1>
        <p class="mt-3 text-lg text-[#a1a1aa]">安全、便捷的数字商品平台</p>
        <div class="mt-10 grid grid-cols-3 gap-3">
          <div class="card"><p class="text-2xl mb-1">🛡️</p><p class="text-xs text-[#71717a]">安全支付</p></div>
          <div class="card"><p class="text-2xl mb-1">⚡</p><p class="text-xs text-[#71717a]">即时交付</p></div>
          <div class="card"><p class="text-2xl mb-1">🔒</p><p class="text-xs text-[#71717a]">隐私保护</p></div>
        </div>
      </div>
    </div>

    <!-- Right side - login form -->
    <div class="flex w-full lg:w-1/2 items-center justify-center p-8">
      <div class="w-full max-w-sm">
        <div class="mb-8 text-center lg:hidden">
          <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-purple-500 to-indigo-600 text-xl font-bold shadow-lg shadow-purple-500/20">N</div>
          <h1 class="text-2xl font-bold">Nodeloc Store</h1>
        </div>

        <h2 class="text-2xl font-bold tracking-tight">欢迎回来</h2>
        <p class="mt-2 text-sm text-[#71717a]">登录后管理订单与数字商品</p>

        <form class="mt-8 space-y-5" @submit.prevent="submit">
          <div>
            <label class="mb-1.5 block text-sm font-medium">用户名或邮箱</label>
            <input v-model="identifier" class="input" required autocomplete="username" placeholder="请输入用户名或邮箱" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium">密码</label>
            <input v-model="password" type="password" class="input" required autocomplete="current-password" placeholder="请输入密码" />
          </div>

          <p v-if="error" class="rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-300">{{ error }}</p>

          <button type="submit" class="btn-primary w-full py-3" :disabled="loading">{{ loading ? '登录中…' : '登录' }}</button>
        </form>

        <div class="my-6 flex items-center gap-3 text-xs text-[#71717a]">
          <span class="h-px flex-1 bg-white/[0.08]"></span>
          或
          <span class="h-px flex-1 bg-white/[0.08]"></span>
        </div>

        <button class="btn-secondary w-full" @click="oauthInitiate">使用 NodeLoc 登录</button>

        <p class="mt-6 text-center text-sm text-[#71717a]">
          还没有账号？ <RouterLink to="/register" class="text-purple-300 hover:text-purple-200">立即注册</RouterLink>
        </p>
      </div>
    </div>
  </div>
</template>
