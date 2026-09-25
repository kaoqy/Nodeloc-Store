<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { getRuntimeSettings, saveRuntimeSettings, testOAuth, testPayment } from '../api/system'
import type { RuntimeSettings } from '../types'

const loading = ref(true)
const saving = ref(false)
const message = ref('')
const messageType = ref<'ok' | 'err'>('ok')
const oauthResult = ref('')
const paymentResult = ref('')
const testingOAuth = ref(false)
const testingPayment = ref(false)

const settings = reactive<RuntimeSettings>({
  app: { site_name: '', site_slogan: '', site_description: '', site_logo: '', scheme: 'https', domain: '' },
  oauth: { enabled: true, base_url: '', client_id: '', client_secret: '', redirect_uri: '', scopes: '' },
  payment: { enabled: true, payment_id: '', secret_key: '' },
  features: { enabled_registration: true },
  theme: { theme_primary: '#6366f1', default_locale: 'zh-CN' },
})

async function load() {
  loading.value = true
  try {
    const result = await getRuntimeSettings()
    Object.assign(settings.app, result.app)
    Object.assign(settings.oauth, result.oauth)
    Object.assign(settings.payment, result.payment)
    Object.assign(settings.features, result.features)
    Object.assign(settings.theme, result.theme)
  } catch (err: any) {
    message.value = err.response?.data?.error || '加载配置失败'
    messageType.value = 'err'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  message.value = ''
  try {
    await saveRuntimeSettings({ ...settings, app: { ...settings.app }, oauth: { ...settings.oauth }, payment: { ...settings.payment }, features: { ...settings.features }, theme: { ...settings.theme } })
    message.value = '已保存，新配置立即生效'
    messageType.value = 'ok'
  } catch (err: any) {
    message.value = err.response?.data?.error || '保存失败'
    messageType.value = 'err'
  } finally {
    saving.value = false
  }
}

async function runOAuthTest() {
  testingOAuth.value = true
  oauthResult.value = ''
  try {
    const r = await testOAuth()
    oauthResult.value = r.ok ? '配置有效，可跳转授权页' : `失败：${r.msg || '请检查 OAuth 参数'}`
  } catch (err: any) {
    oauthResult.value = `失败：${err.response?.data?.error || err.message}`
  } finally {
    testingOAuth.value = false
  }
}

async function runPaymentTest() {
  testingPayment.value = true
  paymentResult.value = ''
  try {
    const r = await testPayment()
    paymentResult.value = `${r.ok ? '✓ ' : '✗ '}${r.msg}`
  } catch (err: any) {
    paymentResult.value = `失败：${err.response?.data?.error || err.message}`
  } finally {
    testingPayment.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-48" />
    <div class="grid gap-4"><div v-for="i in 5" :key="i" class="skeleton h-12" /></div>
  </section>

  <section v-else class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <h2 class="text-xl font-bold">系统设置</h2>
        <p class="text-sm text-[#7a7890]">初始化时配置的所有参数都可以在这里修改，保存后立即生效</p>
      </div>
      <button class="btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中...' : '保存设置' }}
      </button>
    </div>

    <p v-if="message" :class="['rounded-xl border px-4 py-2.5 text-sm', messageType === 'ok' ? 'border-emerald-400/30 bg-emerald-500/10 text-emerald-200' : 'border-rose-400/30 bg-rose-500/10 text-rose-200']">{{ message }}</p>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="space-y-4 lg:col-span-2">
        <div class="card">
          <h3 class="mb-4 font-semibold">基本信息</h3>
          <div class="space-y-3">
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">网站名称</label>
                <input v-model="settings.app.site_name" class="input" />
              </div>
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">标语</label>
                <input v-model="settings.app.site_slogan" class="input" />
              </div>
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">站点域名</label>
              <input v-model="settings.app.domain" class="input" placeholder="store.example.com" />
              <p class="mt-1 text-xs text-[#7a7890]">修改后请同步更新 NodeLoc OAuth 应用的重定向 URI</p>
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">协议</label>
                <select v-model="settings.app.scheme" class="input">
                  <option value="https">https</option>
                  <option value="http">http</option>
                </select>
              </div>
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">Logo URL</label>
                <input v-model="settings.app.site_logo" class="input" />
              </div>
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">网站描述</label>
              <textarea v-model="settings.app.site_description" class="input min-h-20 resize-none" />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="mb-4 flex items-center justify-between">
            <h3 class="font-semibold">NodeLoc OAuth 登录</h3>
            <button
              :class="['btn', settings.oauth.enabled ? 'btn-primary' : 'btn-secondary']"
              @click="settings.oauth.enabled = !settings.oauth.enabled"
            >
              {{ settings.oauth.enabled ? '已启用' : '已禁用' }}
            </button>
          </div>
          <div class="space-y-3">
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">NodeLoc 站点地址</label>
              <input v-model="settings.oauth.base_url" class="input" placeholder="https://www.nodeloc.com" />
            </div>
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">Client ID</label>
                <input v-model="settings.oauth.client_id" class="input" />
              </div>
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">Client Secret</label>
                <input v-model="settings.oauth.client_secret" type="password" class="input" placeholder="保持 ******** 则不修改" autocomplete="off" />
              </div>
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">回调地址（留空自动生成）</label>
              <input v-model="settings.oauth.redirect_uri" class="input" placeholder="自动生成：协议://域名/api/v1/auth/oauth/callback" />
            </div>
            <div class="flex items-center gap-3">
              <button class="btn btn-secondary" :disabled="testingOAuth" @click="runOAuthTest">{{ testingOAuth ? '测试中…' : '测试 OAuth 配置' }}</button>
              <span v-if="oauthResult" class="text-xs text-[#b3b1c4]">{{ oauthResult }}</span>
            </div>
          </div>
        </div>

        <div class="card">
          <div class="mb-4 flex items-center justify-between">
            <h3 class="font-semibold">NodeLoc Payments 支付</h3>
            <button
              :class="['btn', settings.payment.enabled ? 'btn-primary' : 'btn-secondary']"
              @click="settings.payment.enabled = !settings.payment.enabled"
            >
              {{ settings.payment.enabled ? '已启用' : '已禁用' }}
            </button>
          </div>
          <div class="space-y-3">
            <div class="grid gap-3 sm:grid-cols-2">
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">支付 ID（payment_id）</label>
                <input v-model="settings.payment.payment_id" class="input" />
              </div>
              <div>
                <label class="mb-1 block text-sm text-[#b3b1c4]">Secret Key</label>
                <input v-model="settings.payment.secret_key" type="password" class="input" placeholder="保持 ******** 则不修改" autocomplete="off" />
              </div>
            </div>
            <div class="flex items-center gap-3">
              <button class="btn btn-secondary" :disabled="testingPayment" @click="runPaymentTest">{{ testingPayment ? '测试中…' : '测试支付网关' }}</button>
              <span v-if="paymentResult" class="text-xs text-[#b3b1c4]">{{ paymentResult }}</span>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card">
          <h3 class="mb-4 font-semibold">功能开关</h3>
          <div class="space-y-3">
            <label class="flex items-center justify-between">
              <span class="text-sm">开放注册</span>
              <button
                :class="['btn', settings.features.enabled_registration ? 'btn-primary' : 'btn-secondary']"
                @click="settings.features.enabled_registration = !settings.features.enabled_registration"
              >
                {{ settings.features.enabled_registration ? '已启用' : '已禁用' }}
              </button>
            </label>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">外观</h3>
          <div class="space-y-3">
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">主题色</label>
              <input v-model="settings.theme.theme_primary" type="color" class="h-10 w-20 cursor-pointer rounded-lg border-0 bg-transparent" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">默认语言</label>
              <select v-model="settings.theme.default_locale" class="input">
                <option value="zh-CN">简体中文</option>
                <option value="zh-TW">繁體中文</option>
                <option value="en">English</option>
              </select>
            </div>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">系统信息</h3>
          <div class="space-y-2 text-sm">
            <div class="flex justify-between">
              <span class="text-[#7a7890]">版本</span>
              <span>v1.0.0</span>
            </div>
            <div class="flex justify-between">
              <span class="text-[#7a7890]">配置方式</span>
              <span>应用内初始化</span>
            </div>
            <div class="flex justify-between">
              <span class="text-[#7a7890]">配置文件</span>
              <span>无需 yml</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
