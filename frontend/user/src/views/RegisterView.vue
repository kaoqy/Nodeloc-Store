<script setup lang="ts">
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useSiteStore } from '../stores/site'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()

const username = ref('')
const email = ref('')
const password = ref('')
const confirmPassword = ref('')
const error = ref('')
const loading = ref(false)

// A destination has to be a path in this shop. `//example.com` starts with a
// slash but is a host, and pushing one walks a freshly logged-in buyer out the
// front door.
const redirect = computed(() => {
  const target = typeof route.query.redirect === 'string' ? route.query.redirect : ''
  return target.startsWith('/') && !target.startsWith('//') ? target : '/'
})

const mismatch = computed(
  () => confirmPassword.value.length > 0 && confirmPassword.value !== password.value
)

async function submit() {
  if (password.value.length < 8) {
    error.value = '密码至少需要 8 位'
    return
  }
  if (password.value !== confirmPassword.value) {
    error.value = '两次输入的密码不一致'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await auth.register(username.value.trim(), email.value.trim(), password.value)
    await router.push(redirect.value)
  } catch (e) {
    error.value = errorMessage(e, '注册失败，请检查信息后重试')
    loading.value = false
  }
}
</script>

<template>
  <div class="mx-auto flex w-full max-w-md flex-1 flex-col justify-center px-4 py-14 sm:px-0">
    <div class="rise-in">
      <div class="brand-mark mb-6">N</div>
      <p class="eyebrow">{{ site.name }}</p>
      <h1 class="mt-2 text-2xl font-bold">创建本地账号</h1>
      <p class="mt-2 text-sm text-[var(--text-dim)]">
        本地账号可独立完成下单与订单查询；注册后可在个人中心绑定 NodeLoc。
      </p>

      <!-- 设置 can close 注册 at any moment, and this page is often already open
           when it does, so the notice replaces the form instead of waiting for
           the server to refuse the submit. -->
      <div v-if="!site.registrationEnabled" class="card mt-7 p-6 text-center">
        <p class="font-semibold">本店已关闭注册</p>
        <p class="mt-2 text-sm text-[var(--text-dim)]">店家只开放 NodeLoc 账号登录，或直接与店家联系购买。</p>
        <RouterLink to="/login" class="btn btn-primary mt-6">前往登录</RouterLink>
      </div>

      <form v-else class="card mt-7 space-y-4" @submit.prevent="submit">
        <div>
          <label class="label" for="username">用户名</label>
          <input
            id="username"
            v-model="username"
            class="input"
            required
            maxlength="64"
            minlength="2"
            autocomplete="username"
            placeholder="2–64 个字符"
          />
        </div>
        <div>
          <label class="label" for="email">邮箱</label>
          <input
            id="email"
            v-model="email"
            type="email"
            class="input"
            autocomplete="email"
            placeholder="选填，用于接收交付通知"
          />
          <p class="hint mt-1.5">留空则不绑定邮箱。</p>
        </div>
        <div>
          <label class="label" for="password">密码</label>
          <input
            id="password"
            v-model="password"
            type="password"
            class="input"
            required
            minlength="8"
            autocomplete="new-password"
            placeholder="至少 8 位"
          />
        </div>
        <div>
          <label class="label" for="confirm">确认密码</label>
          <input
            id="confirm"
            v-model="confirmPassword"
            type="password"
            class="input"
            required
            autocomplete="new-password"
            placeholder="请再次输入密码"
            :class="{ '!border-[var(--danger)]': mismatch }"
          />
          <p v-if="mismatch" class="hint mt-1.5 text-[var(--danger)]">两次输入的密码不一致</p>
        </div>

        <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

        <button class="btn btn-primary w-full" type="submit" :disabled="loading">
          <span v-if="loading" class="spinner spinner-light" />
          {{ loading ? '创建中…' : '创建账号' }}
        </button>
      </form>

      <p class="hint mt-6 text-center">
        已有账号？
        <RouterLink
          :to="{ path: '/login', query: redirect === '/' ? {} : { redirect } }"
          class="accent-text font-semibold underline underline-offset-4"
        >
          前往登录
        </RouterLink>
      </p>
    </div>
  </div>
</template>
