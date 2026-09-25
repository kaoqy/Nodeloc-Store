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
  <div class="mx-auto flex min-h-screen max-w-5xl items-center justify-center px-4 py-10">
    <div class="grid w-full overflow-hidden rounded-[30px] border border-white/[0.14] bg-gradient-to-b from-white/[0.08] to-white/[0.02] shadow-[inset_0_1px_0_rgba(255,255,255,0.16),0_30px_80px_-20px_rgba(0,0,0,0.65)] backdrop-blur-2xl lg:grid-cols-2">
      <!-- Branding -->
      <div class="relative hidden flex-col justify-center overflow-hidden p-12 lg:flex">
        <div class="pointer-events-none absolute -left-24 top-1/4 size-72 rounded-full bg-indigo-500/30 blur-3xl" />
        <div class="pointer-events-none absolute -right-16 bottom-0 size-64 rounded-full bg-purple-500/25 blur-3xl" />
        <div class="relative text-center">
          <div class="mx-auto mb-8 flex h-20 w-20 items-center justify-center rounded-[26px] border border-white/25 bg-gradient-to-br from-indigo-400 via-indigo-600 to-purple-600 text-3xl font-black text-white shadow-[inset_0_2px_0_rgba(255,255,255,0.5),0_18px_44px_-10px_rgba(99,102,241,0.6)]">
            NL
          </div>
          <h1 class="text-4xl font-black tracking-tight">Nodeloc <span class="gradient-text">Store</span></h1>
          <p class="mt-4 text-lg text-[#b3b1c4]">数字商品交易平台 · 管理后台</p>
          <div class="mt-8 grid grid-cols-3 gap-4 text-center">
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">🛍️</p>
              <p class="mt-1 text-xs text-[#b3b1c4]">商品管理</p>
            </div>
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">📦</p>
              <p class="mt-1 text-xs text-[#b3b1c4]">订单处理</p>
            </div>
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">🔑</p>
              <p class="mt-1 text-xs text-[#b3b1c4]">卡密交付</p>
            </div>
          </div>
        </div>
      </div>

      <!-- Form -->
      <div class="flex items-center justify-center bg-white/[0.03] p-8 sm:p-12">
        <div class="w-full max-w-sm">
          <div class="mb-8 text-center lg:hidden">
            <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl border border-white/25 bg-gradient-to-br from-indigo-400 via-indigo-600 to-purple-600 text-xl font-black text-white shadow-[inset_0_2px_0_rgba(255,255,255,0.45),0_10px_30px_-6px_rgba(99,102,241,0.55)]">
              NL
            </div>
            <h1 class="text-2xl font-bold">Nodeloc <span class="gradient-text">Store</span></h1>
          </div>

          <h2 class="text-2xl font-bold tracking-tight">管理员登录</h2>
          <p class="mt-2 text-sm text-[#7a7890]">登录管理后台</p>

          <form class="mt-8 space-y-5" @submit.prevent="submit">
            <div>
              <label class="mb-2 block text-sm font-medium text-[#b3b1c4]">账号或邮箱</label>
              <input v-model.trim="identifier" class="input" autocomplete="username" placeholder="请输入账号或邮箱" required />
            </div>
            <div>
              <label class="mb-2 block text-sm font-medium text-[#b3b1c4]">密码</label>
              <input v-model="password" type="password" class="input" autocomplete="current-password" placeholder="请输入密码" required />
            </div>

            <p v-if="error" class="fade-in rounded-2xl border border-rose-400/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">
              {{ error }}
            </p>

            <button type="submit" class="btn btn-primary w-full py-3.5" :disabled="loading">
              {{ loading ? '登录中…' : '登录' }}
            </button>
          </form>

          <p class="mt-7 text-center text-sm text-[#7a7890]">
            使用 NodeLoc 账号登录
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
