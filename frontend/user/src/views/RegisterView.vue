<script setup lang="ts">
import { ref } from 'vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '../stores/auth'
const auth = useAuthStore(), router = useRouter()
const username = ref(''), email = ref(''), password = ref(''), confirmPassword = ref(''), error = ref(''), loading = ref(false)
async function submit() { if (password.value !== confirmPassword.value) { error.value = '两次输入的密码不一致'; return } loading.value = true; error.value = ''; try { await auth.register(username.value, email.value, password.value); await router.push('/') } catch { error.value = '注册失败，请检查信息后重试' } finally { loading.value = false } }
</script>
<template><div class="mx-auto max-w-md px-4 py-16"><div class="rounded-3xl p-7 glass sm:p-9"><h1 class="text-3xl font-bold">创建账号</h1><p class="mt-2 text-zinc-400">加入 Nodeloc Store，开启�数字生活</p><form class="mt-8 space-y-5" @submit.prevent="submit"><label class="block text-sm">用户名<input v-model="username" class="field mt-2" minlength="3" required autocomplete="username" /></label><label class="block text-sm">邮箱（选填）<input v-model="email" type="email" class="field mt-2" autocomplete="email" /></label><label class="block text-sm">密码<input v-model="password" type="password" class="field mt-2" minlength="6" required autocomplete="new-password" /></label><label class="block text-sm">确认密码<input v-model="confirmPassword" type="password" class="field mt-2" required autocomplete="new-password" /></label><p v-if="error" class="text-sm text-red-300">{{ error }}</p><button class="btn-primary w-full" :disabled="loading">{{ loading ? '注册中…' : '注册' }}</button></form><p class="mt-6 text-center text-sm text-zinc-400">已有账号？ <RouterLink to="/login" class="text-fuchsia-300">前往登录</RouterLink></p></div></div></template>
