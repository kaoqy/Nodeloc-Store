<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { bindOAuth, oauthInitiate, unbindOAuth } from '../api/auth'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { oauthErrorText, when } from '../utils/format'

const auth = useAuthStore()
const route = useRoute()
const router = useRouter()

const busy = ref(false)
const message = ref('')
const error = ref('')

const user = computed(() => auth.user)
const bound = computed(() => Boolean(user.value?.oauth_provider))
const displayName = computed(() => user.value?.oauth_username || user.value?.username || '账户')
const avatar = computed(() => user.value?.oauth_avatar || user.value?.avatar_url || '')
const initials = computed(() => displayName.value.slice(0, 1).toUpperCase())

async function unbind() {
  if (busy.value) return
  busy.value = true
  message.value = ''
  error.value = ''
  try {
    const response = await unbindOAuth()
    auth.user = response.user
    message.value = '已解除 NodeLoc 绑定，本地账号与订单不受影响。'
  } catch (e) {
    error.value = errorMessage(e, '解绑失败，请稍后重试')
  } finally {
    busy.value = false
  }
}

async function consumeBindCode() {
  const fragment = new URLSearchParams(window.location.hash.replace(/^#/, ''))
  const code = fragment.get('bind_code') || ''
  const state = fragment.get('state') || ''
  if (!code || !state) return
  history.replaceState(null, '', window.location.pathname + window.location.search)
  busy.value = true
  error.value = ''
  try {
    auth.user = await bindOAuth(code, state)
    message.value = 'NodeLoc 账号已绑定，之后可直接用 NodeLoc 登录。'
  } catch (e) {
    error.value = errorMessage(e, '绑定失败，请重试')
  } finally {
    busy.value = false
  }
}

onMounted(async () => {
  await auth.fetchUser().catch(() => undefined)
  if (!auth.isAuthenticated) {
    await router.replace('/login')
    return
  }
  if (typeof route.query.oauth_error === 'string') {
    error.value = oauthErrorText(route.query.oauth_error, '绑定')
    await router.replace({ path: '/profile' })
    return
  }
  await consumeBindCode()
})
</script>

<template>
  <div class="mx-auto w-full max-w-2xl px-4 py-10 sm:px-6">
    <header class="mb-7">
      <p class="eyebrow">Account</p>
      <h1 class="mt-2 text-2xl font-bold">个人中心</h1>
    </header>

    <section class="card">
      <div class="flex items-center gap-4">
        <div class="grid size-14 shrink-0 place-items-center overflow-hidden rounded-full border border-[var(--stroke)] bg-[var(--surface-hi)] text-lg font-bold">
          <img v-if="avatar" :src="avatar" :alt="displayName" class="size-full object-cover" />
          <span v-else>{{ initials }}</span>
        </div>
        <div class="min-w-0">
          <p class="truncate text-lg font-semibold">{{ displayName }}</p>
          <p class="hint mt-0.5 truncate">
            {{ user?.email || '未绑定邮箱' }}
            <span v-if="user?.is_admin" class="badge badge-accent ml-1">管理员</span>
          </p>
        </div>
      </div>

      <div class="my-5 divider" />

      <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-3">
        <div>
          <dt class="text-[var(--text-quiet)]">用户 ID</dt>
          <dd class="nums mt-1">{{ user?.id ?? '—' }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">注册时间</dt>
          <dd class="mt-1">{{ when(user?.created_at) }}</dd>
        </div>
        <div>
          <dt class="text-[var(--text-quiet)]">上次登录</dt>
          <dd class="mt-1">{{ when(user?.last_login_at) }}</dd>
        </div>
      </dl>
    </section>

    <section class="card mt-5">
      <div class="flex flex-wrap items-start justify-between gap-4">
        <div class="min-w-0 flex-1">
          <h2 class="text-[15px] font-bold">NodeLoc 账号</h2>
          <p class="hint mt-1">
            <template v-if="bound">
              已绑定 <span class="mono">{{ user?.oauth_username || user?.oauth_provider }}</span>，
              之后可直接用 NodeLoc 登录本店。
            </template>
            <template v-else>
              绑定后可使用 NodeLoc 账号一键登录，本地订单仍保留在此账号下。
            </template>
          </p>
        </div>
        <div class="flex shrink-0 gap-2">
          <button v-if="bound" class="btn btn-danger btn-sm" :disabled="busy" @click="unbind">解除绑定</button>
          <button v-else class="btn btn-primary btn-sm" @click="oauthInitiate(true)">绑定 NodeLoc</button>
        </div>
      </div>

      <p v-if="message" class="alert alert-success mt-5" role="status">{{ message }}</p>
      <p v-if="error" class="alert alert-danger mt-5" role="alert">{{ error }}</p>
      <p v-if="!bound && !error" class="alert alert-info mt-5" role="status">
        若该 NodeLoc 账号此前已在本店独立注册过，它将作为另一个账号绑定失败——可先联系管理员合并。
      </p>
    </section>

    <nav class="mt-5 grid gap-3 sm:grid-cols-2">
      <RouterLink to="/orders" class="card-hover flex items-center justify-between gap-3">
        <span>
          <span class="block text-[15px] font-semibold">我的订单</span>
          <span class="hint">支付进度与交付内容</span>
        </span>
        <span aria-hidden="true" class="text-[var(--text-quiet)]">→</span>
      </RouterLink>
      <RouterLink to="/" class="card-hover flex items-center justify-between gap-3">
        <span>
          <span class="block text-[15px] font-semibold">继续挑选</span>
          <span class="hint">浏览在售的数码商品</span>
        </span>
        <span aria-hidden="true" class="text-[var(--text-quiet)]">→</span>
      </RouterLink>
    </nav>
  </div>
</template>
