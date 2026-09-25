<script setup lang="ts">
import { ref } from 'vue'
import { bindOAuth, unbindOAuth } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const loading = ref(false)
const message = ref('')
const error = ref('')

async function unbind() {
  loading.value = true
  message.value = ''
  error.value = ''
  try {
    const response = await unbindOAuth()
    auth.user = response.user
    message.value = 'NodeLoc 账号已解绑'
  } catch {
    error.value = '解绑失败，请稍后重试'
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="mx-auto max-w-3xl px-4 py-10 sm:px-6">
    <h1 class="text-3xl font-black tracking-tight">个人<span class="gradient-text">中心</span></h1>
    <p class="mt-2 text-sm text-[#7b7990]">管理账号资料与第三方登录绑定</p>

    <!-- Profile Card -->
    <section class="glass rise-in mt-8 p-7 sm:p-9">
      <div class="flex items-center gap-5">
        <div class="grid size-18 shrink-0 place-items-center overflow-hidden rounded-full border border-white/25 bg-gradient-to-br from-purple-400 via-purple-600 to-indigo-600 text-2xl font-black shadow-[inset_0_2px_0_rgba(255,255,255,0.4),0_12px_32px_-8px_rgba(168,85,247,0.55)]">
          <img v-if="auth.user?.avatar" :src="auth.user.avatar" alt="头像" class="h-full w-full object-cover" />
          <span v-else>{{ auth.user?.username?.slice(0, 1).toUpperCase() }}</span>
        </div>
        <div>
          <h2 class="text-xl font-semibold">{{ auth.user?.username }}</h2>
          <p class="mt-1 text-sm text-[#b4b2c3]">{{ auth.user?.email || '未设置邮箱' }}</p>
        </div>
      </div>

      <div class="mt-7 h-px bg-gradient-to-r from-transparent via-white/15 to-transparent" />

      <dl class="mt-7 grid gap-5 sm:grid-cols-2">
        <div class="rounded-2xl border border-white/[0.08] bg-white/[0.04] p-4">
          <dt class="text-xs text-[#7b7990]">用户 ID</dt>
          <dd class="mt-1.5 font-medium">{{ auth.user?.id }}</dd>
        </div>
        <div class="rounded-2xl border border-white/[0.08] bg-white/[0.04] p-4">
          <dt class="text-xs text-[#7b7990]">注册时间</dt>
          <dd class="mt-1.5 font-medium">{{ auth.user?.created_at ? new Date(auth.user.created_at).toLocaleDateString('zh-CN') : '暂无' }}</dd>
        </div>
      </dl>
    </section>

    <!-- OAuth Card -->
    <section class="glass fade-in mt-6 p-7 sm:p-9">
      <div class="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-center gap-4">
          <span class="grid size-11 place-items-center rounded-2xl border border-white/15 bg-gradient-to-br from-purple-500/25 to-indigo-500/15 text-lg backdrop-blur-md">🔗</span>
          <div>
            <h2 class="text-lg font-semibold">NodeLoc 账号</h2>
            <p class="mt-1 text-sm text-[#b4b2c3]">绑定后可使用 NodeLoc 快速登录</p>
          </div>
        </div>
        <button
          v-if="auth.user?.oauth_bound"
          class="btn btn-danger"
          :disabled="loading"
          @click="unbind"
        >{{ loading ? '解绑中…' : '解除绑定' }}</button>
        <button v-else class="btn btn-primary" @click="bindOAuth">绑定 NodeLoc</button>
      </div>
      <p v-if="message" class="fade-in mt-5 rounded-2xl border border-emerald-400/25 bg-emerald-500/10 px-4 py-3 text-sm text-emerald-300">{{ message }}</p>
      <p v-if="error" class="fade-in mt-5 rounded-2xl border border-rose-400/25 bg-rose-500/10 px-4 py-3 text-sm text-rose-300">{{ error }}</p>
    </section>
  </div>
</template>
