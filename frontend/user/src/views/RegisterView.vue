<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const router = useRouter()

const username = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)

async function submit() {
  if (password.value !== confirmPassword.value) { error.value = '两次输入的密码不一致'; return }
  loading.value = true
  error.value = ''
  try {
    await auth.register(username.value, email.value, password.value)
    await router.push('/')
  } catch {
    error.value = '注册失败，请检查信息后重试'
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
        <div class="pointer-events-none absolute -left-20 bottom-1/4 size-72 rounded-full bg-indigo-500/30 blur-3xl" />
        <div class="pointer-events-none absolute -right-24 top-0 size-64 rounded-full bg-fuchsia-500/20 blur-3xl" />
        <div class="relative text-center">
          <div class="mx-auto mb-8 grid size-20 place-items-center rounded-[26px] border border-white/25 bg-gradient-to-br from-purple-400 via-purple-600 to-indigo-600 text-3xl font-black shadow-[inset_0_2px_0_rgba(255,255,255,0.5),0_18px_44px_-10px_rgba(168,85,247,0.6)]">N</div>
          <h1 class="text-4xl font-black tracking-tight">加入<span class="gradient-text">我们</span></h1>
          <p class="mt-3 text-lg text-[#b4b2c3]">开启数字商品交易新体验</p>
          <div class="mt-10 grid grid-cols-3 gap-3">
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">🎁</p><p class="mt-1 text-xs text-[#b4b2c3]">新用户福利</p>
            </div>
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">💎</p><p class="mt-1 text-xs text-[#b4b2c3]">会员专享</p>
            </div>
            <div class="rounded-2xl border border-white/10 bg-white/[0.05] p-4 backdrop-blur-md transition hover:border-white/20 hover:bg-white/[0.08]">
              <p class="text-2xl">🚀</p><p class="mt-1 text-xs text-[#b4b2c3]">快速开店</p>
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

          <h2 class="text-2xl font-bold tracking-tight">创建账号</h2>
          <p class="mt-2 text-sm text-[#7b7990]">加入 Nodeloc Store，开启数字生活</p>

          <form class="mt-8 space-y-5" @submit.prevent="submit">
            <div>
              <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">用户名</label>
              <input v-model="username" class="input" minlength="3" required autocomplete="username" placeholder="请输入用户名" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">邮箱（选填）</label>
              <input v-model="email" type="email" class="input" autocomplete="email" placeholder="请输入邮箱" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">密码</label>
              <input v-model="password" type="password" class="input" minlength="6" required autocomplete="new-password" placeholder="请输入密码（至少 6 位）" />
            </div>
            <div>
              <label class="mb-1.5 block text-sm font-medium text-[#b4b2c3]">确认密码</label>
              <input v-model="confirmPassword" type="password" class="input" required autocomplete="new-password" placeholder="请再次输入密码" />
            </div>

            <p v-if="error" class="fade-in rounded-2xl border border-rose-400/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{{ error }}</p>

            <button type="submit" class="btn btn-primary w-full py-3.5" :disabled="loading">{{ loading ? '注册中…' : '注册' }}</button>
          </form>

          <p class="mt-7 text-center text-sm text-[#7b7990]">
            已有账号？ <RouterLink to="/login" class="font-medium text-purple-300 transition hover:text-purple-200">前往登录</RouterLink>
          </p>
        </div>
      </div>
    </div>
  </div>
</template>
