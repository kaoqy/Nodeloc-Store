<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { getRuntimeSettings, saveRuntimeSettings, testOAuth, testPayment } from '../api/system'
import type { RuntimeSettings } from '../types'
import { errorMessage } from '../utils/format'

type Probe = { ok: boolean; text: string }

const loading = ref(true)
const saving = ref(false)
const message = ref('')
const messageType = ref<'ok' | 'err'>('ok')
const oauth = ref<Probe | null>(null)
const payment = ref<Probe | null>(null)
const testingOAuth = ref(false)
const testingPayment = ref(false)
const snapshot = ref('')

const settings = reactive<RuntimeSettings>({
  app: { site_name: '', site_slogan: '', site_description: '', site_logo: '', scheme: 'https', domain: '' },
  oauth: { enabled: true, base_url: '', client_id: '', client_secret: '', redirect_uri: '', scopes: '' },
  payment: { enabled: true, payment_id: '', token: '', secret_key: '' },
  features: { enabled_registration: true },
  theme: { theme_primary: '#f2704a', default_locale: 'zh-CN' },
})

const dirty = computed(() => snapshot.value !== '' && JSON.stringify(settings) !== snapshot.value)
const redirectPreview = computed(() => {
  const custom = settings.oauth.redirect_uri.trim()
  if (custom) return custom
  if (!settings.app.domain.trim()) return '填写域名后自动生成'
  return `${settings.app.scheme}://${settings.app.domain.trim()}/api/v1/auth/oauth/callback`
})
const paymentIncomplete = computed(
  () =>
    !settings.payment.payment_id.trim() ||
    settings.payment.token.trim() === '' ||
    settings.payment.secret_key.trim() === '',
)

async function load() {
  loading.value = true
  try {
    const result = await getRuntimeSettings()
    Object.assign(settings.app, result.app)
    Object.assign(settings.oauth, result.oauth)
    Object.assign(settings.payment, result.payment)
    Object.assign(settings.features, result.features)
    Object.assign(settings.theme, result.theme)
    snapshot.value = JSON.stringify(settings)
  } catch (err) {
    message.value = errorMessage(err, '加载配置失败')
    messageType.value = 'err'
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  message.value = ''
  try {
    await saveRuntimeSettings(JSON.parse(JSON.stringify(settings)))
    snapshot.value = JSON.stringify(settings)
    message.value = '已保存，运行时配置已重建并立即生效'
    messageType.value = 'ok'
  } catch (err) {
    message.value = errorMessage(err, '保存失败')
    messageType.value = 'err'
  } finally {
    saving.value = false
  }
}

async function runOAuthTest() {
  testingOAuth.value = true
  oauth.value = null
  try {
    const result = await testOAuth()
    oauth.value = { ok: result.ok, text: result.ok ? result.authorize_url || '配置有效，可跳转授权页' : result.msg || '请检查 OAuth 参数' }
  } catch (err) {
    oauth.value = { ok: false, text: `失败：${errorMessage(err, '无法读取 OAuth 配置')}` }
  } finally {
    testingOAuth.value = false
  }
}

async function runPaymentTest() {
  testingPayment.value = true
  payment.value = null
  try {
    const result = await testPayment()
    payment.value = { ok: result.ok, text: result.msg || (result.ok ? '支付网关连通正常' : '支付网关不可用') }
  } catch (err) {
    payment.value = { ok: false, text: `失败：${errorMessage(err, '无法连接支付网关')}` }
  } finally {
    testingPayment.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-9 w-52" />
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="space-y-4 lg:col-span-2">
        <div v-for="i in 3" :key="i" class="skeleton h-52" />
      </div>
      <div class="space-y-4">
        <div v-for="i in 2" :key="i" class="skeleton h-40" />
      </div>
    </div>
  </section>

  <section v-else class="space-y-6">
    <div class="flex flex-wrap items-end justify-between gap-4">
      <div>
        <p class="eyebrow">Runtime configuration</p>
        <h2 class="mt-1 text-xl font-bold">系统设置</h2>
        <p class="mt-1 text-sm text-[var(--text-quiet)]">保存后运行时配置立即重建，无需重启容器</p>
      </div>
      <div class="flex items-center gap-3">
        <span v-if="dirty" class="badge badge-warning">有未保存的更改</span>
        <button class="btn btn-primary" :disabled="saving || !dirty" @click="save">
          <span v-if="saving" class="spinner spinner-light !size-4" />
          {{ saving ? '正在保存…' : '保存设置' }}
        </button>
      </div>
    </div>

    <p v-if="message" :class="['alert', messageType === 'ok' ? 'alert-success' : 'alert-danger']">{{ message }}</p>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="space-y-5 lg:col-span-2">
        <!-- 站点信息 -->
        <div class="card">
          <div class="mb-5 flex items-baseline justify-between gap-4">
            <h3 class="font-semibold">站点信息</h3>
            <span class="hint">展示在商店前台与管理后台</span>
          </div>
          <div class="space-y-4">
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="label" for="site-name">网站名称</label>
                <input id="site-name" v-model="settings.app.site_name" class="input" />
              </div>
              <div>
                <label class="label" for="site-slogan">标语</label>
                <input id="site-slogan" v-model="settings.app.site_slogan" class="input" />
              </div>
            </div>
            <div class="grid gap-4 sm:grid-cols-[1fr_120px]">
              <div>
                <label class="label" for="site-domain">站点域名</label>
                <input id="site-domain" v-model="settings.app.domain" class="input mono" placeholder="store.example.com" />
                <p class="hint mt-1">修改后请同步更新 NodeLoc OAuth 应用的重定向 URI</p>
              </div>
              <div>
                <label class="label" for="site-scheme">协议</label>
                <select id="site-scheme" v-model="settings.app.scheme" class="input">
                  <option value="https">https</option>
                  <option value="http">http</option>
                </select>
              </div>
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="label" for="site-logo">Logo 地址</label>
                <input id="site-logo" v-model="settings.app.site_logo" class="input" placeholder="留空则使用内置标识" />
              </div>
              <div>
                <label class="label" for="site-desc">网站描述</label>
                <input id="site-desc" v-model="settings.app.site_description" class="input" />
              </div>
            </div>
          </div>
        </div>

        <!-- OAuth -->
        <div class="card">
          <div class="mb-5 flex items-center justify-between gap-4">
            <div>
              <h3 class="font-semibold">NodeLoc OAuth 登录</h3>
              <p class="hint mt-0.5">买家与管理员均可使用 NodeLoc 账号登录</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="hint">{{ settings.oauth.enabled ? '已启用' : '已禁用' }}</span>
              <button
                class="switch"
                :class="{ 'switch-on': settings.oauth.enabled }"
                type="button"
                role="switch"
                :aria-checked="settings.oauth.enabled"
                aria-label="启用 NodeLoc OAuth 登录"
                @click="settings.oauth.enabled = !settings.oauth.enabled"
              />
            </div>
          </div>
          <div class="space-y-4">
            <div>
              <label class="label" for="oauth-base">NodeLoc 站点地址</label>
              <input id="oauth-base" v-model="settings.oauth.base_url" class="input mono" placeholder="https://www.nodeloc.com" />
            </div>
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="label" for="oauth-id">Client ID</label>
                <input id="oauth-id" v-model="settings.oauth.client_id" class="input mono" />
              </div>
              <div>
                <label class="label" for="oauth-secret">Client Secret</label>
                <input id="oauth-secret" v-model="settings.oauth.client_secret" type="password" class="input mono" placeholder="保持 ******** 则不修改" autocomplete="off" />
              </div>
            </div>
            <div>
              <label class="label" for="oauth-redirect">重定向 URI</label>
              <input id="oauth-redirect" v-model="settings.oauth.redirect_uri" class="input mono" :placeholder="redirectPreview" />
              <p class="hint mt-1">留空则自动生成：<span class="mono">{{ redirectPreview }}</span></p>
            </div>
            <div>
              <label class="label" for="oauth-scopes">授权范围 scope</label>
              <input id="oauth-scopes" v-model="settings.oauth.scopes" class="input mono" placeholder="openid profile" />
              <p class="hint mt-1">
                必须包含 <span class="mono">openid</span>（NodeLoc 强制，缺失会被自动补上）。
                <span class="mono">email</span> 需要 NodeLoc 管理员审批，应用没获批时填写会导致授权被拒。
              </p>
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <button class="btn btn-secondary btn-sm" type="button" :disabled="testingOAuth" @click="runOAuthTest">
                {{ testingOAuth ? '测试中…' : '测试 OAuth 配置' }}
              </button>
              <span v-if="oauth" :class="['badge', oauth.ok ? 'badge-success' : 'badge-danger']">{{ oauth.ok ? '通过' : '未通过' }}</span>
            </div>
            <p v-if="oauth" class="codebox text-xs">{{ oauth.text }}</p>
          </div>
        </div>

        <!-- Payments -->
        <div class="card">
          <div class="mb-5 flex items-center justify-between gap-4">
            <div>
              <h3 class="font-semibold">NodeLoc Payments 支付</h3>
              <p class="hint mt-0.5">下单扣减积分，回调地址由订单号自动拼接</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="hint">{{ settings.payment.enabled ? '已启用' : '已禁用' }}</span>
              <button
                class="switch"
                :class="{ 'switch-on': settings.payment.enabled }"
                type="button"
                role="switch"
                :aria-checked="settings.payment.enabled"
                aria-label="启用 NodeLoc Payments 支付"
                @click="settings.payment.enabled = !settings.payment.enabled"
              />
            </div>
          </div>
          <div class="space-y-4">
            <div class="grid gap-4 sm:grid-cols-2">
              <div>
                <label class="label" for="payment-id">Payment ID（payment_id）</label>
                <input id="payment-id" v-model="settings.payment.payment_id" class="input mono" placeholder="pay_xxx" />
                <p class="hint mt-1">写在接口地址里，决定款项进哪个应用。</p>
              </div>
              <div>
                <label class="label" for="payment-token">Payment Token（tk_xxx）</label>
                <input id="payment-token" v-model="settings.payment.token" type="password" class="input mono" placeholder="tk_xxx；保持 ******** 则不修改" autocomplete="off" />
                <p class="hint mt-1">买家下单时用它签名（SHA-256 后再作 HMAC 密钥），缺失就无法创建支付。</p>
              </div>
              <div class="sm:col-span-2">
                <label class="label" for="payment-secret">Secret Key（商户密钥）</label>
                <input id="payment-secret" v-model="settings.payment.secret_key" type="password" class="input mono" placeholder="保持 ******** 则不修改" autocomplete="off" />
                <p class="hint mt-1">原样用于查单签名与回调验签，不要填成 Token。</p>
              </div>
            </div>
            <p v-if="paymentIncomplete" class="alert alert-warning" role="alert">
              三项都要填写：Payment ID 决定收款应用，Payment Token 用于下单，Secret Key 用于查单和回调验签。缺任何一项，买家下单都会失败。
            </p>
            <div class="flex flex-wrap items-center gap-3">
              <button class="btn btn-secondary btn-sm" type="button" :disabled="testingPayment" @click="runPaymentTest">
                {{ testingPayment ? '测试中…' : '测试支付网关' }}
              </button>
              <span v-if="payment" :class="['badge', payment.ok ? 'badge-success' : 'badge-danger']">{{ payment.ok ? '通过' : '未通过' }}</span>
            </div>
            <p v-if="payment" class="codebox text-xs">{{ payment.text }}</p>
          </div>
        </div>
      </div>

      <!-- 右侧栏 -->
      <div class="space-y-5">
        <div class="card">
          <h3 class="mb-4 font-semibold">功能开关</h3>
          <div class="flex items-center justify-between gap-4">
            <div>
              <p class="text-sm">开放本地账号注册</p>
              <p class="hint mt-0.5">关闭后仅能通过 NodeLoc 登录</p>
            </div>
            <button
              class="switch"
              :class="{ 'switch-on': settings.features.enabled_registration }"
              type="button"
              role="switch"
              :aria-checked="settings.features.enabled_registration"
              aria-label="开放本地账号注册"
              @click="settings.features.enabled_registration = !settings.features.enabled_registration"
            />
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">外观</h3>
          <div class="space-y-4">
            <div class="flex items-center justify-between gap-4">
              <div>
                <label class="label mb-0" for="theme-primary">主题色</label>
                <p class="hint mt-0.5 mono">{{ settings.theme.theme_primary }}</p>
              </div>
              <input id="theme-primary" v-model="settings.theme.theme_primary" type="color" class="h-9 w-16 cursor-pointer rounded-lg border border-[var(--stroke)] bg-transparent" />
            </div>
            <div>
              <label class="label" for="theme-locale">默认语言</label>
              <select id="theme-locale" v-model="settings.theme.default_locale" class="input">
                <option value="zh-CN">简体中文</option>
                <option value="zh-TW">繁體中文</option>
                <option value="en">English</option>
              </select>
            </div>
          </div>
        </div>

        <div class="card card-quiet">
          <h3 class="mb-4 font-semibold">运行时</h3>
          <dl class="space-y-2.5 text-sm">
            <div class="flex items-center justify-between gap-3">
              <dt class="quiet">版本</dt>
              <dd class="mono">v1.0.0</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="quiet">数据库</dt>
              <dd class="mono">SQLite / MySQL</dd>
            </div>
            <div class="flex items-center justify-between gap-3">
              <dt class="quiet">配置来源</dt>
              <dd>应用内设置</dd>
            </div>
          </dl>
          <p class="hint mt-4 leading-relaxed">
            配置写入数据库中的运行时记录，不依赖任何 yml 文件；备份数据库即备份全部设置。
          </p>
        </div>
      </div>
    </div>
  </section>
</template>
