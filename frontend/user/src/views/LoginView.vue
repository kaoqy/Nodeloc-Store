<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { oauthInitiate } from '../api/auth'
import { useAuthStore } from '../stores/auth'
const auth = useAuthStore(), router = useRouter(), route = useRoute()
const identifier = ref(''), password = ref(''), error = ref(''), loading = ref(false)
async function submit() { loading.value = true; error.value = ''; try { await auth.login(identifier.value, password.value); await router.push(typeof route.query.redirect === 'string' ? route.query.redirect : '/') } catch { error.value = '登录失败，请检查账号和密码' } finally { loading.value = false } }
</script>
<template><div class="mx-auto max-w-md px-4 py-16"><div class="rounded-3xl p-7 glass sm:p-9"><h1 class="text-3xl font-bold">欢迎回来</h1><p class="mt-2 text-zinc-400">登录后管理订单与数字商品</p><form class="mt-8 space-y-5" @submit.prevent="submit"><label class="block text-sm text-zinc-300">用户名或邮箱<input v-model="identifier" class="field mt-2" required autocomplete="username" /></label><label class="block text-sm text-zinc-300">密码<input v-model="password" type="password" class="field mt-2" required autocomplete="current-password" /></label><p v-if="error" class="text-sm text-red-300">{{ error }}</p><button class="btn-primary w-full" :disabled="loading">{{ loading ? '登录中…' : '登录' }}</button></form><div class="my-6 flex items-center gap-3 text-xs text-zinc-600"><span class="h-px flex-1 bg-white/10"></span>或<span class="h-px flex-1 bg-white/10"></span></div><button class="w-full rounded-xl border border-white/10 px-4 py-3 font-medium hover:bg-white/5" @click="oauthInitiate">使用 NodeLoc 登录</button><p class="mt-6 text-center text-sm text-zinc-400">还没有账号？ <RouterLink to="/register" class="text-fuchsia-300">立即注册</RouterLink></p></div></div></template>
