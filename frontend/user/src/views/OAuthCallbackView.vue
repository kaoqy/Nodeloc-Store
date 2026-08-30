<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { oauthCallback } from '../api/auth'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const error = ref('')

onMounted(async () => {
  const code = typeof route.query.code === 'string' ? route.query.code : ''
  const state = typeof route.query.state === 'string' ? route.query.state : ''
  if (!code || !state) { error.value = 'OAuth 回调参数不完整'; return }
  try {
    const response = await oauthCallback(code, state)
    localStorage.setItem('token', response.tokens.access_token)
    auth.token = response.tokens.access_token
    auth.user = response.user
    await router.replace('/')
  } catch {
    error.value = 'NodeLoc 登录失败，请返回后重试'
  }
})
</script>

<template>
  <div class="mx-auto grid min-h-[65vh] max-w-md place-items-center px-4">
    <div class="w-full rounded-2xl border border-white/[0.08] bg-[#111114] p-8 text-center">
      <template v-if="!error">
        <div class="mx-auto size-12 animate-spin rounded-full border-4 border-white/10 border-t-purple-500" />
        <h1 class="mt-6 text-xl font-semibold">正在完成 NodeLoc 登录</h1>
        <p class="mt-2 text-sm text-[#a1a1aa]">请稍候，不要关闭页面…</p>
      </template>
      <template v-else>
        <h1 class="text-xl font-semibold text-red-300">登录未完成</h1>
        <p class="mt-3 text-sm text-[#a1a1aa]">{{ error }}</p>
        <RouterLink to="/login" class="btn-primary mt-6 inline-block">返回登录</RouterLink>
      </template>
    </div>
  </div>
</template>
