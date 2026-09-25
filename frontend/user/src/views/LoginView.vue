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
  <div class="mx-auto flex min-h-[calc(100vh-96px)] max-w-7xl items-center justify-center px-4 py-10">
    <div class="grid w-full max-w-5xl overflow-hidden rounded-[30px] border border-white/[0.14] bg-gradient-to-b from-white/[0.08] to-white/[0.02] shadow-[inset_0_1px_0_rgba(255,255,255,0.16),0_30px_80px_-20px_rgba(0,0,0,0.65)] backdrop-blur-2xl lg:grid-cols-2">
      <!-- Branding -->
      <div class="relative hidden flex-col justify-center overflow-hidden p-12 lg:flex">
        <div class="pointer-events-none absolute -left-24 top-1/4 size-72 rounded-full bg-purple-500/30 blur-3xl" />
        <div class="pointer-events-none absolute -right-16 bottom-0 size-64 rounded-full bg-indigo-500/25 blur-3xl" />
        <div class="relative text-center">
          <div class="mx-auto mb-8 grid size-20 place-items-center rounded-[26px] border border-white/25 bg-gradient-to-br from-purple-400 via-purple-600 to-indigo-600 text-3xl font-black shadow-[inset_0_2px_0_rgba(255,255,255,0.5),0_18px_44px_-10px_rgba(168,85,247,0.6)]">N</div>
          <h1 class="text-4xl font-black tracking-tight">Nodeloc <span class="gradient-text">Store</span></h1>
          <p class="mt-3 text-lg text-[#b4b2c3]">安全、便捷的数字商品平台</p>
          <div class="mt-10 grid grid-cols-3 gap-3">
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">🛡️</p><p class="mt-1 text-xs text-[#b4b2c3]">安全支付</p>
            </div>
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">⚡</p><p class="mt-1 text-xs text-[#b4b2c3]">即时交付</p>
            </div>
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">🔒</p><p class="mt-1 text-xs text-[#b4b2c3]">隐私保护</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Form -->
      <div class="flex items-center justify-center bg-white/[0.03] p-8 sm:p-12">
        <div class="w-full max-w-sm">
          <div class="mb-8 text-center lg:hidden">
            <div class="mx-auto mb-4 grid size-14 place-items-center rounded-2xl border border-white/25 bg-gradient-to-br from-purple-400 via-purple-600 to-indigo-600 text-xl font-black shadow-[inset_0_2px_0_rgba(255,255,255,0.45),0_10px_30px_-6px_rgba(168,85,247,0.55)]">N</div>
            <h1 class="text-2xl font-bold">Nodeloc <span class="gradient-text">Store</span></h1>
          </div>

          <h2 class="text-2xl font-bold tracking-tight">欢迎回来 <span class="text-lg">👋</span></h2>
          <p class="mt-2 text-sm text-[#7b7990]">登录后管理订单与数字商品</p>

          <form class="mt-8 space-y-5" @submit.prevent="submit">
            <div>
              <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">用户名或邮箱</label>
              <input v-model="identifier" class="input" required autocomplete="username" placeholder="请输入用户名或邮箱" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">密码</label>
              <input v-model="password" type="password" class="input" required autocomplete="current-password" placeholder="请输入密码" />
            </div>

            <p v-if="error" class="fade-in rounded-2xl border border-rose-400/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{{ error }}</p>

            <button type="submit" class="btn btn-primary w-full py-3.5" :disabled="loading">{{ loading ? '登录中…' : '登录' }}</button>
          </form>

          <div class="my-6 flex items-center gap-4 text-xs text-[#7b7990]">
            <span class="h-px flex-1 bg-gradient-to-r from-transparent via-white/15 to-transparent"></span>
            或
            <span class="h-px flex-1 bg-gradient-to-r from-transparent via-white/15 to-transparent"></span>
          </div>

          <button class="btn btn-secondary w-full py-3.5" @click="oauthInitiate">
            <span class="grid size-5 place-items-center rounded-full bg-gradient-to-br from-purple-400 to-indigo-500 text-[10px] font-black">N</span>
            使用 NodeLoc 登录
          </button>

          <p class="mt-7 text-center text-sm text-[#7b7990]">
            还没有账号？ <RouterLink to="/register" class="font-medium text-purple-300 transition hover:text-purple-200">立即注册</RouterLink>
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
