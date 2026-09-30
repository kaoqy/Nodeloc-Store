<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useAuthStore } from '../stores/auth'
import { getRuntimeSettings, saveRuntimeSettings, testOAuth, testPayment } from '../api/system'
import type { FooterLink, RuntimeSettings } from '../types'
import ImageField from '../components/ImageField.vue'
import { errorMessage } from '../utils/format'
import { applyBrand } from '../utils/brand'
import { applyShopIdentity } from '../utils/identity'

type Probe = { ok: boolean; text: string }

// The storefront renders the link rows straight into its footer, so the same
// ceiling the server applies is shown here instead of failing on save.
const MaxFooterLinks = 8

const auth = useAuthStore()
const loading = ref(true)
const saving = ref(false)
const message = ref('')
// 'warn' is for a save that landed but cannot take effect yet, which is neither
// a success nor a failure.
const messageType = ref<'ok' | 'err' | 'warn'>('ok')
const oauth = ref<Probe | null>(null)
const payment = ref<Probe | null>(null)
const testingOAuth = ref(false)
const testingPayment = ref(false)
const snapshot = ref('')

const settings = reactive<RuntimeSettings>({
  app: {
    site_name: '',
    site_slogan: '',
    site_description: '',
    site_logo: '',
    scheme: 'https',
    domain: '',
    footer_text: '',
    footer_note: '',
    footer_links: [],
    announcement: '',
  },
  oauth: { enabled: true, base_url: '', client_id: '', client_secret: '', redirect_uri: '', scopes: '' },
  payment: { enabled: true, payment_id: '', token: '', secret_key: '', base_url: '' },
  features: { enabled_registration: true, enabled_checkin: true, enabled_coupons: true, stock_alert_threshold: 5 },
  theme: { theme_primary: '#f2704a', default_locale: 'zh-CN' },
})

const canManage = computed(() => auth.allows('settings', 'manage'))

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

// What the server says is missing, as opposed to what this half-typed form looks
// like. The two disagree exactly when a secret is stored and masked as ********,
// and the owner deserves the store's own reading, not a guess from the inputs.
const MISSING_LABELS: Record<string, string> = {
  payment_id: 'Payment ID',
  token: 'Payment Token',
  secret_key: 'Secret Key',
  base_url: 'NodeLoc 域名',
}
const serverMissing = ref<string[]>([])
const missingLabels = computed(() => serverMissing.value.map((name) => MISSING_LABELS[name] || name))
const paymentBlocked = computed(
  () => settings.payment.enabled && (paymentIncomplete.value || serverMissing.value.length > 0),
)
const paymentBasePreview = computed(
  () =>
    (settings.payment.base_url || '').trim() ||
    (settings.oauth.base_url || '').trim() ||
    '未设置：请先填 NodeLoc 域名或这里的支付地址',
)

async function refreshReadiness() {
  try {
    const doc = await getRuntimeSettings()
    serverMissing.value = doc.payment_missing ?? []
  } catch {
    // A failed diagnostic read must not block the page that fixes the problem.
    serverMissing.value = []
  }
}

const footerLinks = computed(() => settings.app.footer_links ?? [])
const linksFull = computed(() => footerLinks.value.length >= MaxFooterLinks)

function addFooterLink() {
  if (linksFull.value) return
  const list = (settings.app.footer_links ||= [])
  const link: FooterLink = { label: '', url: '' }
  list.push(link)
}

function removeFooterLink(index: number) {
  settings.app.footer_links?.splice(index, 1)
}

// A number input goes through an empty state while it is being retyped, so the
// field keeps its own string and only the committed value reaches the settings.
const stockThreshold = ref('5')
watch(stockThreshold, (raw) => {
  const parsed = Number.parseInt(raw, 10)
  settings.features.stock_alert_threshold = Number.isNaN(parsed) ? 5 : Math.min(Math.max(parsed, 0), 999)
})
watch(
  () => settings.features.stock_alert_threshold,
  (value) => {
    const text = String(value ?? 5)
    if (text !== stockThreshold.value) stockThreshold.value = text
  },
)

// The picker repaints this page while it is being dragged. persist=false keeps
// an unsaved preview out of the shared storage key, which is what the
// storefront's boot splash reads; only a saved colour is allowed to claim that.
watch(
  () => settings.theme.theme_primary,
  (value) => applyBrand(value, false),
)

const savedBrand = ref(settings.theme.theme_primary)

function linkTargetValid(url: string) {
  const value = url.trim()
  if (!value) return false
  if (value.startsWith('/') && !value.startsWith('//')) return true
  return /^https?:\/\//i.test(value)
}

async function load() {
  loading.value = true
  try {
    const document = await getRuntimeSettings()
    const result = document.settings
    Object.assign(settings.app, result.app)
    settings.app.footer_links = (result.app.footer_links ?? []).map((link) => ({ ...link }))
    Object.assign(settings.oauth, result.oauth)
    Object.assign(settings.payment, result.payment)
    Object.assign(settings.features, result.features)
    Object.assign(settings.theme, result.theme)
    stockThreshold.value = String(settings.features.stock_alert_threshold ?? 5)
    snapshot.value = JSON.stringify(settings)
    serverMissing.value = document.payment_missing ?? []
    // Remembered as the colour on file, so a save that fails can put it back.
    savedBrand.value = settings.theme.theme_primary
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
    const result = await saveRuntimeSettings(JSON.parse(JSON.stringify(settings)))
    snapshot.value = JSON.stringify(settings)
    // Committing writes the shared key, so even the storefront's boot splash
    // wears the new colour before it has asked the API anything.
    savedBrand.value = settings.theme.theme_primary
    applyBrand(savedBrand.value)
    // Renaming the shop renames its browser tabs and its door plate, here and
    // in the storefront.
    applyShopIdentity(settings.app.site_name, settings.app.site_logo)
    // The server wrote the row but could not rebuild the runtime, so claiming
    // 「立即生效」 here would point the owner at a change that is not live.
    if (result?.restart_pending) {
      message.value = result.message || '设置已保存，但这一项要等容器重启后才生效'
      messageType.value = 'warn'
    } else {
      message.value = '已保存，运行时配置已重建并立即生效'
      messageType.value = 'ok'
    }
    // Ask the store what it now believes is missing, rather than letting a stale
    // warning sit under a form that was just fixed.
    refreshReadiness()
  } catch (err) {
    // The saved colour is still the truth, so the page puts on it again. The
    // picker keeps the rejected choice: the save usually fails for a different
    // field, and making the owner re-pick a hex would only lose their work.
    applyBrand(savedBrand.value, false)
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
        <span v-if="!canManage" class="badge badge-neutral">只读</span>
        <span v-else-if="dirty" class="badge badge-warning">有未保存的更改</span>
        <button class="btn btn-primary" :disabled="saving || !dirty || !canManage" @click="save">
          <span v-if="saving" class="spinner spinner-light !size-4" />
          {{ saving ? '正在保存…' : '保存设置' }}
        </button>
      </div>
    </div>

    <p
      v-if="message"
      :class="[
        'alert',
        messageType === 'ok' ? 'alert-success' : messageType === 'warn' ? 'alert-warning' : 'alert-danger',
      ]"
      >{{ message }}</p
    >
    <p v-if="!canManage" class="alert alert-warning" role="alert">
      当前账号只有查看系统设置的权限，所有字段均为只读。需要改动时请联系超级管理员授予「设置 · 管理」。
    </p>

    <div class="grid gap-6 lg:grid-cols-3">
      <!-- A disabled fieldset turns off every control inside it at once, so the
           read-only view cannot be edited even where an input has no :disabled. -->
      <fieldset class="m-0 min-w-0 space-y-5 border-0 p-0 lg:col-span-2" :disabled="!canManage">
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
              <ImageField
                v-model="settings.app.site_logo"
                input-id="site-logo"
                label="Logo 地址"
                placeholder="留空则使用内置标识"
                scope="site"
                preview
                :disabled="!canManage"
              />
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
              <span v-if="paymentBlocked" class="badge badge-danger" title="开关是开的，但凭据不完整，买家下单仍会失败">还收不了款</span>
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
              <div class="sm:col-span-2">
                <label class="label" for="payment-base">支付 API 地址（可选）</label>
                <input
                  id="payment-base"
                  v-model="settings.payment.base_url"
                  class="input mono"
                  placeholder="留空则使用 NodeLoc 域名"
                />
                <p class="hint mt-1">
                  下单、查单、转账都发往这里。只有当你的论坛域名与支付应用不在同一个域时才需要填，
                  例如登录走镜像站而收款走主站；填错会让每个买家都收不到付款页。
                </p>
                <p class="quiet mt-1 text-xs mono">当前发往：{{ paymentBasePreview }}</p>
              </div>
            </div>
            <p v-if="paymentIncomplete" class="alert alert-warning" role="alert">
              三项都要填写：Payment ID 决定收款应用，Payment Token 用于下单，Secret Key 用于查单和回调验签。缺任何一项，买家下单都会失败。
            </p>
            <p v-else-if="serverMissing.length" class="alert alert-warning" role="alert">
              开关是开着的，但商店还收不了钱：服务端认为缺少
              <strong>{{ missingLabels.join('、') }}</strong>。
              填好后点「保存配置」，再用下面的「测试支付网关」复核。
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

        <!-- 公告与页脚 -->
        <div class="card">
          <div class="mb-5 flex items-baseline justify-between gap-4">
            <div>
              <h3 class="font-semibold">公告与页脚</h3>
              <p class="hint mt-0.5">商店前台的首页横幅与页脚文案</p>
            </div>
            <span class="hint">纯展示，不参与下单</span>
          </div>
          <div class="space-y-4">
            <div>
              <label class="label" for="footer-announcement">首页公告</label>
              <input
                id="footer-announcement"
                v-model="settings.app.announcement"
                class="input"
                maxlength="200"
                placeholder="例如：国庆期间 24 小时自动发货"
              />
              <p class="hint mt-1">单行横幅，买家可以在前台自行关闭；清空即不再展示。</p>
            </div>
            <div>
              <label class="label" for="footer-text">页脚主文案</label>
              <textarea
                id="footer-text"
                v-model="settings.app.footer_text"
                class="input"
                maxlength="300"
                rows="2"
                placeholder="店铺简介、联系方式、备案号等"
              ></textarea>
              <p class="hint mt-1">留空则显示站点名称与标语。</p>
            </div>
            <div>
              <label class="label" for="footer-note">页脚小字</label>
              <input
                id="footer-note"
                v-model="settings.app.footer_note"
                class="input"
                maxlength="120"
                placeholder="例如：© 2026 Your Shop · 保留所有权利"
              />
              <p class="hint mt-1">展示在主文案下方的一行说明。</p>
            </div>
            <div>
              <div class="flex items-baseline justify-between gap-3">
                <span class="label mb-0">页脚链接</span>
                <span class="hint mono">{{ footerLinks.length }} / {{ MaxFooterLinks }}</span>
              </div>
              <p class="hint mt-1">支持 <span class="mono">https://</span>、<span class="mono">http://</span> 或以 <span class="mono">/</span> 开头的站内路径。</p>
              <div class="mt-3 space-y-2.5">
                <div v-for="(link, index) in footerLinks" :key="index">
                  <div class="flex items-center gap-2">
                    <input
                      v-model="link.label"
                      class="input w-32 shrink-0"
                      maxlength="40"
                      placeholder="名称"
                      :aria-label="`第 ${index + 1} 条页脚链接的名称`"
                    />
                    <input
                      v-model="link.url"
                      class="input mono min-w-0 flex-1"
                      maxlength="300"
                      placeholder="https://example.com"
                      :aria-label="`第 ${index + 1} 条页脚链接的地址`"
                    />
                    <button class="btn btn-quiet btn-sm shrink-0" type="button" @click="removeFooterLink(index)">移除</button>
                  </div>
                  <p v-if="link.url.trim() && !linkTargetValid(link.url)" class="mt-1 text-xs text-[var(--danger)]">
                    地址需要以 http://、https:// 或 / 开头，这一条在保存时会被忽略。
                  </p>
                </div>
                <div class="flex items-center gap-3">
                  <button v-if="!linksFull" class="btn btn-secondary btn-sm" type="button" @click="addFooterLink">添加链接</button>
                  <span v-else class="hint">最多 {{ MaxFooterLinks }} 条链接。</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </fieldset>

      <!-- 右侧栏 -->
      <fieldset class="m-0 min-w-0 space-y-5 border-0 p-0" :disabled="!canManage">
        <div class="card">
          <h3 class="mb-4 font-semibold">功能开关</h3>
          <div class="space-y-4">
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
            <div class="divider" />
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm">每日签到</p>
                <p class="hint mt-0.5">关闭后买家中心不再显示签到入口</p>
              </div>
              <button
                class="switch"
                :class="{ 'switch-on': settings.features.enabled_checkin }"
                type="button"
                role="switch"
                :aria-checked="settings.features.enabled_checkin"
                aria-label="开放每日签到"
                @click="settings.features.enabled_checkin = !settings.features.enabled_checkin"
              />
            </div>
            <div class="flex items-center justify-between gap-4">
              <div>
                <p class="text-sm">优惠码</p>
                <p class="hint mt-0.5">关闭后下单不再接受任何优惠码</p>
              </div>
              <button
                class="switch"
                :class="{ 'switch-on': settings.features.enabled_coupons }"
                type="button"
                role="switch"
                :aria-checked="settings.features.enabled_coupons"
                aria-label="开放优惠码"
                @click="settings.features.enabled_coupons = !settings.features.enabled_coupons"
              />
            </div>
            <div class="divider" />
            <div>
              <label class="label" for="stock-threshold">库存预警阈值</label>
              <input
                id="stock-threshold"
                v-model="stockThreshold"
                class="input mono w-28"
                inputmode="numeric"
                autocomplete="off"
                @blur="stockThreshold = String(settings.features.stock_alert_threshold ?? 5)"
              />
              <p class="hint mt-1">可用卡密少于这个数就进入看板的「等待补货」，0 表示只在完全缺货时提醒。</p>
            </div>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">外观</h3>
          <div class="space-y-4">
            <div class="flex items-center justify-between gap-4">
              <div>
                <label class="label mb-0" for="theme-primary">主题色</label>
                <p class="hint mt-0.5 mono">{{ settings.theme.theme_primary }}</p>
                <p class="hint mt-1">
                  前台、后台与登录页的按钮、链接、徽标都会跟着变色。拖动即可预览，保存后才对买家生效。
                </p>
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
              <p class="hint mt-1.5">
                写进前台页面的 lang 属性：读屏软件与中日韩字形回退会跟着变，后台界面本身仍是简体中文。
              </p>
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
      </fieldset>
    </div>
  </section>
</template>
