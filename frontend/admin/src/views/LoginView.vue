<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { useThemeStore } from '../stores/theme'

const router = useRouter()
const auth = useAuthStore()
const theme = useThemeStore()

const identifier = ref('')
const password = ref('')
const loading = ref(false)
const error = ref('')

async function submit() {
  loading.value = true
  error.value = ''
  try {
    await auth.login(identifier.value.trim(), password.value)
    await router.push('/')
  } catch (err) {
    error.value = errorMessage(err, '登录失败，请检查账号和密码')
    loading.value = false
  }
}
</script>

<template>
  <div class="relative mx-auto flex min-h-screen w-full max-w-md flex-col justify-center px-5 py-12">
    <button
      class="btn btn-ghost absolute right-4 top-4 !px-2.5"
      :title="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
      :aria-label="theme.theme === 'dark' ? '切换到浅色主题' : '切换到深色主题'"
      @click="theme.toggle"
    >
      <span aria-hidden="true">{{ theme.theme === 'dark' ? '☀' : '☾' }}</span>
    </button>

    <div class="rise-in">
      <div class="brand-mark mb-6">N</div>
      <p class="eyebrow">Nodeloc Store</p>
      <h1 class="mt-2 text-2xl font-bold">管理后台登录</h1>
      <p class="mt-2 text-sm muted">仅管理员账号可进入，普通用户请前往 storefront 前台。</p>

      <form class="card mt-8 space-y-4" @submit.prevent="submit">
        <div>
          <label class="label" for="identifier">用户名或邮箱</label>
          <input
            id="identifier"
            v-model="identifier"
            class="input"
            required
            autocomplete="username"
            placeholder="请输入账号"
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

        <p v-if="error" class="alert alert-danger">{{ error }}</p>

        <button class="btn btn-primary w-full" type="submit" :disabled="loading">
          <span v-if="loading" class="spinner !border-t-[#fffaf7]" />
          {{ loading ? '登录中…' : '登录' }}
        </button>
      </form>

      <p class="hint mt-6 text-center">
        <a href="/" class="hover:text-[var(--text-dim)]">前往商店首页 →</a>
      </p>
    </div>
  </div>
</template>
