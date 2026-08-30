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
    <h1 class="text-2xl font-bold">个人中心</h1>
    <p class="mt-2 text-sm text-[#71717a]">管理账号资料与第三方登录绑定</p>

    <!-- Profile Card -->
    <section class="mt-8 rounded-2xl border border-white/[0.08] bg-[#111114] p-7 sm:p-9">
      <div class="flex items-center gap-5">
        <div class="grid size-16 shrink-0 place-items-center overflow-hidden rounded-2xl bg-gradient-to-br from-purple-500 to-indigo-600 text-2xl font-bold">
          <img v-if="auth.user?.avatar" :src="auth.user.avatar" alt="头像" class="h-full w-full object-cover" />
          <span v-else>{{ auth.user?.username?.slice(0, 1).toUpperCase() }}</span>
        </div>
        <div>
          <h2 class="text-xl font-semibold">{{ auth.user?.username }}</h2>
          <p class="mt-1 text-sm text-[#a1a1aa]">{{ auth.user?.email || '未设置邮箱' }}</p>
        </div>
      </div>

      <div class="mt-6 h-px bg-white/[0.08]" />

      <dl class="mt-6 grid gap-5 sm:grid-cols-2">
        <div>
          <dt class="text-sm text-[#71717a]">用户 ID</dt>
          <dd class="mt-1 font-medium">{{ auth.user?.id }}</dd>
        </div>
        <div>
          <dt class="text-sm text-[#71717a]">注册时间</dt>
          <dd class="mt-1 font-medium">{{ auth.user?.created_at ? new Date(auth.user.created_at).toLocaleDateString('zh-CN') : '暂无' }}</dd>
        </div>
      </dl>
    </section>

    <!-- OAuth Card -->
    <section class="mt-6 rounded-2xl border border-white/[0.08] bg-[#111114] p-7 sm:p-9">
      <div class="flex flex-col gap-5 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h2 class="text-lg font-semibold">NodeLoc 账号</h2>
          <p class="mt-1 text-sm text-[#a1a1aa]">绑定后可使用 NodeLoc 快速登录</p>
        </div>
        <button
          v-if="auth.user?.oauth_bound"
          class="btn-danger"
          :disabled="loading"
          @click="unbind"
        >{{ loading ? '解绑中…' : '解除绑定' }}</button>
        <button v-else class="btn-primary" @click="bindOAuth">绑定 NodeLoc</button>
      </div>
      <p v-if="message" class="mt-4 rounded-lg border border-emerald-500/20 bg-emerald-500/10 px-3 py-2 text-sm text-emerald-300">{{ message }}</p>
      <p v-if="error" class="mt-4 rounded-lg border border-red-500/20 bg-red-500/10 px-3 py-2 text-sm text-red-300">{{ error }}</p>
    </section>
  </div>
</template>
