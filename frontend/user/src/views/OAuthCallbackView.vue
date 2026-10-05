<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { oauthCallback } from '../api/auth'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { oauthErrorText } from '../utils/format'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const error = ref('')

function query(key: string): string {
  return typeof route.query[key] === 'string' ? String(route.query[key]) : ''
}

function target(): string {
  const stored = sessionStorage.getItem('oauth_redirect')
  sessionStorage.removeItem('oauth_redirect')
  return stored && stored.startsWith('/') && !stored.startsWith('//') ? stored : '/'
}

function fail(reason: string) {
  // A leftover redirect would only be read on the next attempt, where it would
  // look like this one sent the buyer somewhere unexpected.
  sessionStorage.removeItem('oauth_redirect')
  error.value = reason
}

onMounted(async () => {
  if (sessionStorage.getItem('oauth_callback_processing') === window.location.href) {
    fail('这次授权回调已经处理过，请返回登录页重新发起登录。')
    return
  }
  const serverReason = query('oauth_error')
  if (serverReason) {
    fail(oauthErrorText(serverReason))
    return
  }
  // NodeLoc can bounce straight back with its own error= instead of a code.
  const denied = query('error')
  if (denied) {
    fail(oauthErrorText(denied === 'access_denied' ? 'denied' : 'provider'))
    return
  }

  // Browser navigation flow: the server bounces back to /oauth/callback#access_token=…
  // Tokens travel in the fragment so they never reach logs or the Referer header.
  const fragment = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const token = fragment.get('access_token') || ''
  if (token) {
    history.replaceState(null, '', window.location.pathname)
    auth.saveSession(token, null, fragment.get('refresh_token') || undefined)
    try {
      await auth.fetchUser()
      await router.replace(target())
      return
    } catch {
      auth.logout()
      fail('登录状态校验失败，请重新尝试 NodeLoc 登录。')
      return
    }
  }

  const code = query('code')
  const state = query('state')
  if (!code || !state) {
    fail('回调参数不完整，请重新发起 NodeLoc 登录。')
    return
  }
  const callbackURL = window.location.href
  sessionStorage.setItem('oauth_callback_processing', callbackURL)
  history.replaceState(null, '', window.location.pathname)
  try {
    const response = await oauthCallback(code, state)
    auth.saveSession(response.tokens.access_token, response.user, response.tokens.refresh_token)
    sessionStorage.removeItem('oauth_callback_processing')
    await router.replace(target())
  } catch (e) {
    // The server already said which step failed, in Chinese; a bare 「授权未完成」
    // is what left buyers and shop owners arguing about whose fault it was.
    sessionStorage.removeItem('oauth_callback_processing')
    fail(errorMessage(e, 'NodeLoc 授权未完成，请返回登录页重试。'))
  }
})
</script>

<template>
  <div class="page-shell page-shell-narrow flex flex-1 flex-col justify-center py-20">
    <div class="rise-in text-center">
      <template v-if="!error">
        <div class="spinner mx-auto !size-8" />
        <h1 class="mt-6 text-lg font-semibold">正在完成 NodeLoc 登录</h1>
        <p class="hint mt-2">请稍候，无需刷新页面…</p>
      </template>
      <template v-else>
        <div class="eyebrow">登录</div>
        <h1 class="mt-2 text-lg font-bold text-[var(--danger)]">登录未完成</h1>
        <p class="surface-panel mt-4 px-4 py-3 text-left text-sm text-[var(--text-dim)]">{{ error }}</p>
        <RouterLink to="/login" class="btn btn-primary mt-6">返回登录</RouterLink>
      </template>
    </div>
  </div>
</template>
