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
  // Browser navigation flow: server redirects back with #access_token=…
  const hash = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const hashedToken = hash.get('access_token') || ''
  if (hashedToken) {
    localStorage.setItem('token', hashedToken)
    auth.token = hashedToken
    try {
      await auth.fetchUser()
      await router.replace('/')
      return
    } catch {
      auth.logout()
      error.value = 'NodeLoc 登录失败，请返回后重试'
      return
    }
  }

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
    <div class="glass rise-in w-full p-10 text-center">
      <template v-if="!error">
        <div class="relative mx-auto size-14">
          <div class="absolute inset-0 animate-spin rounded-full border-4 border-white/[0.07] border-t-purple-400" />
          <div class="absolute inset-2 animate-spin rounded-full border-4 border-white/[0.05] border-b-indigo-400 [animation-direction:reverse]" />
        </div>
        <h1 class="mt-7 text-xl font-semibold">正在完成 NodeLoc 登录</h1>
        <p class="mt-2 text-sm text-[#b4b2c3]">请稍候，不要关闭页面…</p>
      </template>
      <template v-else>
        <div class="mx-auto mb-5 grid size-14 place-items-center rounded-full border border-rose-400/30 bg-rose-500/12 text-2xl backdrop-blur-md">⚠️</div>
        <h1 class="text-xl font-semibold text-rose-300">登录未完成</h1>
        <p class="mt-3 text-sm text-[#b4b2c3]">{{ error }}</p>
        <RouterLink to="/login" class="btn btn-primary mt-7 inline-flex px-8">返回登录</RouterLink>
      </template>
    </div>
  </div>
</template>
