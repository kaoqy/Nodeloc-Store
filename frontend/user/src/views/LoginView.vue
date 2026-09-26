<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { oauthInitiate } from '../api/auth'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()

const identifier = ref('')
const password = ref('')
const error = ref('')
const loading = ref(false)

const redirect = computed(() =>
  typeof route.query.redirect === 'string' && route.query.redirect.startsWith('/') ? route.query.redirect : '/'
)

const oauthError = computed(() => typeof route.query.oauth_error === 'string')

function startOAuth() {
  // The provider round-trips through the backend, so carry the destination here.
  sessionStorage.setItem('oauth_redirect', redirect.value)
  error.value = ''
  oauthInitiate()
}

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(identifier.value.trim(), password.value)
    await router.push(redirect.value)
  } catch (e) {
    error.value = errorMessage(e, '登录失败，请检查账号和密码')
    loading.value = false
  }
}
</script>

<template>
  <div class="mx-auto flex w-full max-w-md flex-1 flex-col justify-center px-4 py-14 sm:px-0">
    <div class="rise-in">
      <div class="brand-mark mb-6">N</div>
      <p class="eyebrow">{{ site.name }}</p>
      <h1 class="mt-2 text-2xl font-bold">登录</h1>
      <p class="mt-2 text-sm text-[var(--text-dim)]">使用 NodeLoc 账号即可下单，无需重复注册。</p>

      <div v-if="oauthError" class="alert alert-warning mt-6" role="alert">
        NodeLoc 登录未完成，可能是链接过期或授权被拒绝。你可以重试，或改用账号密码登录。
      </div>

      <button class="btn btn-primary btn-lg mt-7 w-full" type="button" @click="startOAuth">
        使用 NodeLoc 账号继续
      </button>

      <div class="my-6 flex items-center gap-3">
        <span class="h-px flex-1 bg-[var(--stroke)]" />
        <span class="hint">或使用账号密码</span>
        <span class="h-px flex-1 bg-[var(--stroke)]" />
      </div>

      <form class="card space-y-4" @submit.prevent="submit">
        <div>
          <label class="label" for="identifier">用户名或邮箱</label>
          <input
            id="identifier"
            v-model="identifier"
            class="input"
            required
            autocomplete="username"
            placeholder="请输入用户名或邮箱"
          />
        </div>
        <div>
          <label class="label" for="password">密码</label>
          <input
            id="password"
            v-model="password"
            type="password"
            class="input"
            required
            autocomplete="current-password"
            placeholder="请输入密码"
          />
        </div>

        <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

        <button class="btn btn-secondary w-full" type="submit" :disabled="loading">
          <span v-if="loading" class="spinner !border-t-[var(--text)]" />
          {{ loading ? '登录中…' : '登录' }}
        </button>
      </form>

      <p class="hint mt-6 text-center">
        还没有账号？
        <RouterLink :to="{ path: '/register', query: redirect === '/' ? {} : { redirect } }" class="accent-text font-semibold underline underline-offset-4">注册本地账号</RouterLink>
      </p>
    </div>
  </div>
</template>
