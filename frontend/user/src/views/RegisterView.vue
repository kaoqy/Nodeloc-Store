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
  <div class="flex min-h-screen">
    <!-- Left side - branding -->
    <div class="hidden lg:flex lg:w-1/2 items-center justify-center bg-gradient-to-br from-purple-900/30 via-indigo-900/20 to-[#09090b] p-12">
      <div class="max-w-md text-center">
        <div class="mx-auto mb-8 flex h-20 w-20 items-center justify-center rounded-3xl bg-gradient-to-br from-purple-500 to-indigo-600 text-3xl font-bold shadow-2xl shadow-purple-500/30">N</div>
        <h1 class="text-4xl font-black tracking-tight">加入我们</h1>
        <p class="mt-3 text-lg text-[#a1a1aa]">开启数字商品交易新体验</p>
        <div class="mt-10 grid grid-cols-3 gap-3">
          <div class="card"><p class="text-2xl mb-1">🎁</p><p class="text-xs text-[#71717a]">新用户福利</p></div>
          <div class="card"><p class="text-2xl mb-1">💎</p><p class="text-xs text-[#71717a]">会员专享</p></div>
          <div class="card"><p class="text-2xl mb-1">🚀</p><p class="text-xs text-[#71717a]">快速开店</p></div>
        </div>
      </div>
    </div>

    <!-- Right side - register form -->
    <div class="flex w-full lg:w-1/2 items-center justify-center p-8">
      <div class="w-full max-w-sm">
        <div class="mb-8 text-center lg:hidden">
          <div class="mx-auto mb-4 flex h-14 w-14 items-center justify-center rounded-2xl bg-gradient-to-br from-purple-500 to-indigo-600 text-xl font-bold shadow-lg shadow-purple-500/20">N</div>
          <h1 class="text-2xl font-bold">Nodeloc Store</h1>
        </div>

        <h2 class="text-2xl font-bold tracking-tight">创建账号</h2>
        <p class="mt-2 text-sm text-[#71717a]">加入 Nodeloc Store，开启数字生活</p>

        <form class="mt-8 space-y-5" @submit.prevent="submit">
          <div>
            <label class="mb-1.5 block text-sm font-medium">用户名</label>
            <input v-model="username" class="input" minlength="3" required autocomplete="username" placeholder="请输入用户名" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium">邮箱（选填）</label>
            <input v-model="email" type="email" class="input" autocomplete="email" placeholder="请输入邮箱" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium">密码</label>
            <input v-model="password" type="password" class="input" minlength="6" required autocomplete="new-password" placeholder="请输入密码（至少 6 位）" />
          </div>
          <div>
            <label class="mb-1.5 block text-sm font-medium">确认密码</label>
            <input v-model="confirmPassword" type="password" class="input" required autocomplete="new-password" placeholder="请再次输入密码" />
          </div>

          <p v-if="error" class="rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-300">{{ error }}</p>

          <button type="submit" class="btn-primary w-full py-3" :disabled="loading">{{ loading ? '注册中…' : '注册' }}</button>
        </form>

        <p class="mt-6 text-center text-sm text-[#71717a]">
          已有账号？ <RouterLink to="/login" class="text-purple-300 hover:text-purple-200">前往登录</RouterLink>
        </p>
      </div>
    </div>
  </div>
</template>
