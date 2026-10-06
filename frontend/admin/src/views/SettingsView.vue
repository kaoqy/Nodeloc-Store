<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { onBeforeRouteLeave } from 'vue-router'
import { useAuthStore } from '../stores/auth'
import { getOAuthAttempts, getRuntimeSettings, saveRuntimeSettings, testMail, testOAuth, testPayment, type OAuthAttempt } from '../api/system'
import type { FooterLink, RuntimeSettings } from '../types'
import ImageField from '../components/ImageField.vue'
import { errorMessage, oauthOutcome, oauthReason, oauthStep, when } from '../utils/format'
import { applyBrand } from '../utils/brand'
import { applyShopIdentity } from '../utils/identity'

type Probe = { ok: boolean; text: string }

// The storefront renders the link rows straight into its footer, so the same
// ceiling the server applies is shown here instead of failing on save.
const MaxFooterLinks = 8

const auth = useAuthStore()

// 设置分组：站点与品牌、NodeLoc 登录、支付、邮件、公告、功能开关、外观、运行时。
// 每一组都是同一份表单的一部分，切换分组不会丢改动，保存仍然是一次提交。
// 五个分组按「店家要配置什么」划分。
// 原来 8 个标签里有「运行时信息」这种纯展示页，还有把公告和页脚拆出去、
// 让店主在两个标签之间来回跳的写法；现在同类内容放在同一屏。
const SETTINGS_TABS = [
  { key: 'site', label: '站点与品牌', hint: '站名、域名、Logo 与外观' },
  { key: 'oauth', label: 'NodeLoc 登录', hint: 'OAuth 凭据与登录记录' },
  { key: 'payment', label: '支付设置', hint: 'Nodeloc Payments 凭据' },
  { key: 'new_api', label: 'New-API 发货', hint: '兑换码渠道与 quota 换算' },
  { key: 'content', label: '内容与功能', hint: '公告、页脚与功能开关' },
  { key: 'smtp', label: '邮件通知', hint: 'SMTP 与发信测试' },
] as const

const settingsTab = ref<(typeof SETTINGS_TABS)[number]['key']>('site')

// 打开页面时若地址带 ?tab=payment，就直接落到那一组，方便从别处跳过来。
const initialTab = typeof window !== 'undefined' ? new URLSearchParams(window.location.search).get('tab') : ''
if (initialTab && SETTINGS_TABS.some((item) => item.key === initialTab)) {
  settingsTab.value = initialTab as (typeof SETTINGS_TABS)[number]['key']
}

const activeTabHint = computed(() => {
  const found = SETTINGS_TABS.find((item) => item.key === settingsTab.value)
  return found ? found.hint : '保存后运行时配置立即重建，无需重启容器'
})

// 放弃更改：把表单恢复成上一次成功保存的快照。
function discard() {
  if (!snapshot.value) return
  try {
    const saved = JSON.parse(snapshot.value) as RuntimeSettings
    Object.assign(settings.app, saved.app)
    Object.assign(settings.oauth, saved.oauth)
    Object.assign(settings.payment, saved.payment)
    Object.assign(settings.new_api, saved.new_api)
    Object.assign(settings.smtp, saved.smtp)
    Object.assign(settings.features, saved.features)
    Object.assign(settings.theme, saved.theme)
    applyBrand(savedBrand.value, false)
    message.value = '已放弃未保存的更改'
    messageType.value = 'ok'
  } catch {
    message.value = '恢复上一次保存的内容失败，请刷新页面'
    messageType.value = 'err'
  }
}

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
const mailRecipient = ref('')
const testingMail = ref(false)
const mailMessage = ref('')
const snapshot = ref('')
const attempts = ref<OAuthAttempt[]>([])
const loadingAttempts = ref(false)
const attemptsError = ref('')

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
  smtp: { enabled: false, host: '', port: 587, username: '', password: '', secure: 'starttls', from: '' },
  payment: { enabled: true, payment_id: '', token: '', secret_key: '', base_url: '' },
  new_api: {
    enabled: false,
    base_url: '',
    admin_access_token: '',
    admin_user_id: '',
    nl_usd_rate: '',
  },
  features: { enabled_registration: true, enabled_checkin: true, enabled_coupons: true, stock_alert_threshold: 5 },
  theme: { theme_primary: '#f2704a', default_locale: 'zh-CN' },
})

const canManage = computed(() => auth.allows('settings', 'manage'))
// 「测试 OAuth 配置」要能改配置才给按；登录记录只是读，客服之外的只读账号也该看到。
const canViewAttempts = computed(() => auth.allows('settings', 'view') || canManage.value)

const dirty = computed(() => snapshot.value !== '' && JSON.stringify(settings) !== snapshot.value)
const redirectPreview = computed(() => {
  const custom = settings.oauth.redirect_uri.trim()
  if (custom) return custom
  if (!settings.app.domain.trim()) return '填写域名后自动生成'
  return `${settings.app.scheme}://${settings.app.domain.trim()}/api/v1/auth/oauth/callback`
})
// NodeLoc's payment application may hand out one secret or two, and either box is
// enough to sign with, so the form is incomplete only when neither is filled.
const paymentIncomplete = computed(
  () =>
    !settings.payment.payment_id.trim() ||
    (settings.payment.token.trim() === '' && settings.payment.secret_key.trim() === ''),
)

// What the server says is missing, as opposed to what this half-typed form looks
// like. The two disagree exactly when a secret is stored and masked as ********,
// and the owner deserves the store's own reading, not a guess from the inputs.
const MISSING_LABELS: Record<string, string> = {
  payment_id: 'Payment ID',
  token: 'Payment Token',
  secret_key: 'Secret Key',
  base_url: 'NodeLoc 域名',
  client_id: 'Client ID',
  client_secret: 'Client Secret',
  domain: '站点域名',
}
const serverMissing = ref<string[]>([])
const missingLabels = computed(() => serverMissing.value.map((name) => MISSING_LABELS[name] || name))
// Credentials that are filled in but cannot work — Token and Secret Key swapped,
// an OAuth Client ID typed into the Payment ID box. The store checks the shape on
// save; guessing it here would only drift from what the server actually reads.
const serverWarnings = ref<string[]>([])
const serverNewAPIMissing = ref<string[]>([])
const paymentBlocked = computed(
  () => settings.payment.enabled && (paymentIncomplete.value || serverMissing.value.length > 0),
)
// 登录读的是同一套：商店发现空在哪一格，以及哪一格填了却不可能工作。缺了这两句，
// 「登录不了」就只剩买家的一句抱怨。
const oauthMissing = ref<string[]>([])
const oauthWarnings = ref<string[]>([])
const oauthMissingLabels = computed(() => oauthMissing.value.map((name) => MISSING_LABELS[name] || name))
const oauthIncomplete = computed(
  () => !settings.oauth.client_id.trim() || !settings.oauth.client_secret.trim(),
)
const oauthBlocked = computed(
  () => settings.oauth.enabled && (oauthIncomplete.value || oauthMissing.value.length > 0),
)
const paymentBasePreview = computed(
  () =>
    (settings.payment.base_url || '').trim() ||
    (settings.oauth.base_url || '').trim() ||
    '未设置：请先填 NodeLoc 域名或这里的支付地址',
)

// The payment gateway note is what 「测试支付网关」 learned the last time somebody
// pressed it, including the signing convention NodeLoc turned out to accept. The
// probe is not free — it is one POST at the provider — so it is remembered here
// instead of re-run every time the page opens. It is remembered *with* the
// credentials it was about: an answer about a Token the owner has since retyped
// would read 「签名已被接受」 about a setting nobody has tested.
const probeKey = 'nodeloc-store.payment-probe'

type StoredProbe = { at: string; ok: boolean; msg: string; stamp: string }

const lastProbe = ref<StoredProbe | null>(null)

function credentialStamp(): string {
  return [
    settings.payment.payment_id,
    settings.payment.token,
    settings.payment.secret_key,
    settings.payment.base_url,
    settings.oauth.base_url,
  ]
    .map((value) => (value || '').trim())
    .join('|')
}

function readStoredProbe(): StoredProbe | null {
  try {
    const raw = localStorage.getItem(probeKey)
    if (!raw) return null
    const parsed = JSON.parse(raw) as StoredProbe
    return parsed && typeof parsed.msg === 'string' ? parsed : null
  } catch {
    return null
  }
}

function probeTime(value: string): string {
  const at = new Date(value)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleString('zh-CN', { hour12: false })
}

// The still-valid note: the one just pressed, or the stored one as long as none of
// the five fields it was measured against has moved.
const paymentProbeNote = computed(() => {
  if (payment.value) return { ok: payment.value.ok, text: payment.value.text, at: '' }
  const stored = lastProbe.value
  if (!stored || stored.stamp !== credentialStamp()) return null
  return { ok: stored.ok, text: stored.msg, at: probeTime(stored.at) }
})

async function refreshReadiness() {
  try {
    const doc = await getRuntimeSettings()
    serverMissing.value = doc.payment_missing ?? []
    serverWarnings.value = doc.payment_warnings ?? []
    serverNewAPIMissing.value = doc.new_api_missing ?? []
    oauthMissing.value = doc.oauth_missing ?? []
    oauthWarnings.value = doc.oauth_warnings ?? []
  } catch {
    // A failed diagnostic read must not block the page that fixes the problem.
    serverMissing.value = []
    serverWarnings.value = []
    serverNewAPIMissing.value = []
    oauthMissing.value = []
    oauthWarnings.value = []
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

/** 掩码代表「保持原值」，显式清除才是真的删除。 */
function clearSecret(
  target: 'oauth-secret' | 'payment-token' | 'payment-secret' | 'smtp-pass' | 'new-api-token',
) {
  switch (target) {
    case 'oauth-secret':
      settings.oauth.client_secret = ''
      break
    case 'payment-token':
      settings.payment.token = ''
      break
    case 'payment-secret':
      settings.payment.secret_key = ''
      break
    case 'smtp-pass':
      settings.smtp.password = ''
      break
    case 'new-api-token':
      settings.new_api.admin_access_token = ''
      break
  }
  message.value = '已标记为清除，保存后旧密钥会从运行时配置中删除。'
  messageType.value = 'warn'
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
    Object.assign(settings.new_api, result.new_api)
    if (result.smtp) Object.assign(settings.smtp, result.smtp)
    Object.assign(settings.features, result.features)
    Object.assign(settings.theme, result.theme)
    stockThreshold.value = String(settings.features.stock_alert_threshold ?? 5)
    snapshot.value = JSON.stringify(settings)
    serverMissing.value = document.payment_missing ?? []
    serverWarnings.value = document.payment_warnings ?? []
    serverNewAPIMissing.value = document.new_api_missing ?? []
    oauthMissing.value = document.oauth_missing ?? []
    oauthWarnings.value = document.oauth_warnings ?? []
    lastProbe.value = readStoredProbe()
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
    // The server's sentence is the diagnosis (「Client ID 与 Client Secret 对不上」 and
    // not a bare 通过/未通过), so it shows on both outcomes; the authorize link is what
    // the owner can click next.
    const note = result.msg || (result.ok ? 'NodeLoc 认这组凭据' : '请检查 OAuth 参数')
    oauth.value = {
      ok: result.ok,
      text: result.authorize_url ? `${note} 授权链接：${result.authorize_url}` : note,
    }
  } catch (err) {
    oauth.value = { ok: false, text: `失败：${errorMessage(err, '无法读取 OAuth 配置')}` }
  } finally {
    testingOAuth.value = false
  }
}

// The shop's own record of every NodeLoc 登录 round trip. A container log is out
// of reach for someone running Docker from a panel, and 「登录不了」 without a step
// attached is not something they can act on.
async function loadOAuthAttempts() {
  if (!canViewAttempts.value) return
  loadingAttempts.value = true
  attemptsError.value = ''
  try {
    attempts.value = await getOAuthAttempts(20)
  } catch (err) {
    attemptsError.value = errorMessage(err, '无法读取登录记录')
  } finally {
    loadingAttempts.value = false
  }
}

async function runMailTest() {
  testingMail.value = true
  mailMessage.value = ''
  try {
    const result = await testMail(mailRecipient.value.trim())
    mailMessage.value = result.message || 'SMTP 测试邮件已发送'
  } catch (err) {
    mailMessage.value = `失败：${errorMessage(err, 'SMTP 测试发送失败')}`
  } finally {
    testingMail.value = false
  }
}

async function runPaymentTest() {
  testingPayment.value = true
  payment.value = null
  try {
    const result = await testPayment()
    payment.value = { ok: result.ok, text: result.msg || (result.ok ? '支付网关连通正常' : '支付网关不可用') }
    lastProbe.value = {
      at: new Date().toISOString(),
      ok: result.ok,
      msg: payment.value.text,
      stamp: credentialStamp(),
    }
    try {
      localStorage.setItem(probeKey, JSON.stringify(lastProbe.value))
    } catch {
      // A private-mode storage failure only loses the remembered note, not the
      // result on screen.
    }
  } catch (err) {
    payment.value = { ok: false, text: `失败：${errorMessage(err, '无法连接支付网关')}` }
  } finally {
    testingPayment.value = false
  }
}

// 有未保存改动时离开页面会先问一句，避免长表单白填。
function guardUnload(event: BeforeUnloadEvent) {
  if (!dirty.value || saving.value) return
  event.preventDefault()
  event.returnValue = ''
}

// beforeunload only covers a tab close or hard reload. The settings screen is a
// long form, so switching to another admin page must offer the same protection.
onBeforeRouteLeave(() => {
  if (!dirty.value || saving.value) return true
  return window.confirm('有未保存的系统设置，确定离开并放弃这些更改吗？')
})

onMounted(() => {
  window.addEventListener('beforeunload', guardUnload)
  void load()
  void loadOAuthAttempts()
})

onBeforeUnmount(() => {
  window.removeEventListener('beforeunload', guardUnload)
})
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
    <div class="settings-page-head">
      <div>
        <p class="eyebrow">运行时配置</p>
        <h2 class="mt-1 text-xl font-bold">系统设置</h2>
        <p class="mt-1 text-sm text-[var(--text-quiet)]">
          {{ activeTabHint }}
        </p>
      </div>
      <div class="settings-page-actions">
        <span v-if="!canManage" class="badge badge-neutral">只读</span>
        <span v-else-if="dirty" class="badge badge-warning">有未保存的更改</span>
        <button class="btn btn-primary" :disabled="saving || !dirty || !canManage" @click="save">
          <span v-if="saving" class="spinner spinner-light !size-4" />
          {{ saving ? '正在保存…' : '保存设置' }}
        </button>
      </div>
    </div>

    <!-- 保存栏固定在顶部：在长表单里滚到下半页也不必回到页首才能保存。 -->
    <div
      v-if="canManage && dirty"
      class="sticky top-[76px] z-20 flex flex-wrap items-center gap-3 rounded-[var(--radius-sm)] border border-[var(--accent-line)] bg-[var(--glass)] px-4 py-2.5 backdrop-blur"
      role="status"
    >
      <span class="text-sm">有你改过但还没保存的设置</span>
      <button class="btn btn-quiet btn-sm" :disabled="saving" @click="discard">放弃更改</button>
      <button class="btn btn-primary btn-sm ml-auto" :disabled="saving" @click="save">
        {{ saving ? '正在保存…' : '立即保存' }}
      </button>
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

    <!-- 设置分组：左侧是分区导航，右侧只显示当前分区。
         所有分区仍在同一份表单里，保存一次会整体生效。 -->
    <div class="settings-layout">
      <nav class="settings-nav-panel hidden lg:block" aria-label="设置分组">
        <ul class="space-y-0.5">
          <li v-for="item in SETTINGS_TABS" :key="item.key">
            <button
              class="settings-nav-item w-full text-left"
              :class="settingsTab === item.key ? 'settings-nav-active' : ''"
              :aria-current="settingsTab === item.key ? 'page' : undefined"
              @click="settingsTab = item.key"
            >
              <span class="block text-[13px]">{{ item.label }}</span>
              <span class="quiet block text-[11px]">{{ item.hint }}</span>
            </button>
          </li>
        </ul>
      </nav>

      <div class="settings-content min-w-0 space-y-5">
        <!-- 移动端：分区变成一排横向 chips，避免长页面来回滚动。 -->
        <div class="flex flex-wrap gap-1.5 lg:hidden">
          <button
            v-for="item in SETTINGS_TABS"
            :key="item.key"
            class="chip"
            :class="settingsTab === item.key ? 'chip-active' : ''"
            @click="settingsTab = item.key"
          >
            {{ item.label }}
          </button>
        </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <!-- A disabled fieldset turns off every control inside it at once, so the
           read-only view cannot be edited even where an input has no :disabled. -->
      <fieldset class="m-0 min-w-0 space-y-5 border-0 p-0 lg:col-span-2" :disabled="!canManage">
        <!-- 站点信息 -->
        <div v-show="settingsTab === 'site'" class="card">
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
        <div v-show="settingsTab === 'oauth'" class="card">
          <div class="mb-5 flex items-center justify-between gap-4">
            <div>
              <h3 class="font-semibold">NodeLoc OAuth 登录</h3>
              <p class="hint mt-0.5">买家与管理员均可使用 NodeLoc 账号登录</p>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="oauthBlocked" class="badge badge-danger" title="开关是开的，但登录参数不完整，买家点「用 NodeLoc 登录」仍会被拒">还登不了</span>
              <span v-else-if="oauthWarnings.length" class="badge badge-warning" title="参数都填了，但商店看出有一项不可能工作；看下面这几条">配置可疑</span>
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
                <div class="flex gap-2">
                  <input id="oauth-secret" v-model="settings.oauth.client_secret" type="password" class="input mono min-w-0 flex-1" placeholder="保持 ******** 则不修改" autocomplete="off" />
                  <button class="btn btn-quiet btn-sm shrink-0" type="button" @click="clearSecret('oauth-secret')">清除</button>
                </div>
                <p class="hint mt-1">已保存时显示 ********；不修改就保持原值，点「清除」并保存才会删除。</p>
              </div>
            </div>
            <div>
              <label class="label" for="oauth-redirect">重定向 URI</label>
              <input id="oauth-redirect" v-model="settings.oauth.redirect_uri" class="input mono" :placeholder="redirectPreview" />
              <p class="hint mt-1">
                留空则按站点域名自动生成：<span class="mono">{{ redirectPreview }}</span>。
                清空这一格并保存，就会丢掉之前手填的地址，回到上面这个。
              </p>
            </div>
            <div>
              <label class="label" for="oauth-scopes">授权范围 scope</label>
              <input id="oauth-scopes" v-model="settings.oauth.scopes" class="input mono" placeholder="openid profile" />
              <p class="hint mt-1">
                必须包含 <span class="mono">openid</span>（NodeLoc 强制，缺失会被自动补上）。
                <span class="mono">email</span> 需要 NodeLoc 管理员审批，应用没获批时填写会导致授权被拒。
              </p>
            </div>
            <div v-if="oauthBlocked && oauthMissingLabels.length" class="alert alert-danger" role="alert">
              NodeLoc 登录现在还不能用，商店发现这几项是空的：{{ oauthMissingLabels.join('、') }}。
              站点域名决定回调地址，缺了它商店无从生成，买家会在授权那一步被拒。
            </div>
            <div v-if="oauthWarnings.length" class="alert alert-warning" role="alert">
              <p class="font-medium">这几项填了，但商店认为它们跑不通：</p>
              <ul class="mt-1.5 list-disc space-y-1 pl-5">
                <li v-for="(warning, index) in oauthWarnings" :key="index">{{ warning }}</li>
              </ul>
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
        <div v-show="settingsTab === 'payment'" class="card">
          <div class="mb-5 flex items-center justify-between gap-4">
            <div>
              <h3 class="font-semibold">NodeLoc Payments 支付</h3>
              <p class="hint mt-0.5">下单从买家的 NodeLoc 账户扣 NL，回调地址由订单号自动拼接</p>
            </div>
            <div class="flex items-center gap-2">
              <span class="hint">{{ settings.payment.enabled ? '已启用' : '已禁用' }}</span>
              <span v-if="paymentBlocked" class="badge badge-danger" title="开关是开的，但凭据不完整，买家下单仍会失败">还收不了款</span>
              <span v-else-if="serverWarnings.length" class="badge badge-warning" title="三项都填了，但商店怀疑其中某两串拿错了；展开下面这张卡片看是哪一项">凭据可疑</span>
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
                <div class="flex gap-2">
                  <input id="payment-token" v-model="settings.payment.token" type="password" class="input mono min-w-0 flex-1" placeholder="tk_xxx；保持 ******** 则不修改" autocomplete="off" />
                  <button class="btn btn-quiet btn-sm shrink-0" type="button" @click="clearSecret('payment-token')">清除</button>
                </div>
                <p class="hint mt-1">文档用它签名下单与转账（SHA-256 后再作 HMAC 密钥）。与下面两项任选其一填写即可。</p>
              </div>
              <div class="sm:col-span-2">
                <label class="label" for="payment-secret">Secret Key（商户密钥）</label>
                <div class="flex gap-2">
                  <input id="payment-secret" v-model="settings.payment.secret_key" type="password" class="input mono min-w-0 flex-1" placeholder="保持 ******** 则不修改" autocomplete="off" />
                  <button class="btn btn-quiet btn-sm shrink-0" type="button" @click="clearSecret('payment-secret')">清除</button>
                </div>
                <p class="hint mt-1">原样用于查单签名与回调验签。NodeLoc 的支付应用只给你一串密钥时，把它填在这里或上面任意一格都可以。</p>
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
              Payment ID 必填，签名密钥填 Payment Token 或 Secret Key 其中一格即可（NodeLoc 给两串就都填）。缺任何一项，买家下单都会失败。
            </p>
            <p v-else-if="serverMissing.length" class="alert alert-warning" role="alert">
              开关是开着的，但商店还收不了钱：服务端认为缺少
              <strong>{{ missingLabels.join('、') }}</strong>。
              填好后点「保存设置」，再用下面的「测试支付网关」复核。
            </p>
            <div v-if="serverWarnings.length" class="alert alert-warning" role="alert">
              <p class="font-medium">这几项填了，但商店认为它们用不了：</p>
              <ul class="mt-1.5 list-disc space-y-1.5 pl-5 text-xs leading-relaxed">
                <li v-for="(warning, index) in serverWarnings" :key="index">{{ warning }}</li>
              </ul>
            </div>
            <div class="flex flex-wrap items-center gap-3">
              <button class="btn btn-secondary btn-sm" type="button" :disabled="testingPayment" @click="runPaymentTest">
                {{ testingPayment ? '测试中…' : '测试支付网关' }}
              </button>
              <span v-if="paymentProbeNote" :class="['badge', paymentProbeNote.ok ? 'badge-success' : 'badge-danger']">
                {{ paymentProbeNote.ok ? '通过' : '未通过' }}
              </span>
              <span v-if="paymentProbeNote?.at" class="hint">上次实测 {{ paymentProbeNote.at }}</span>
            </div>
            <p v-if="paymentProbeNote" class="codebox text-xs">{{ paymentProbeNote.text }}</p>
          </div>
        </div>

        <!-- New-API redemption delivery -->
        <div v-show="settingsTab === 'new_api'" class="card">
          <div class="mb-5 flex flex-wrap items-center justify-between gap-4">
            <div>
              <h3 class="font-semibold">New-API 兑换码发货</h3>
              <p class="hint mt-0.5">
                付款成功后创建兑换码并交付给买家。这是兑换码交付，不是自动充值到账。
              </p>
            </div>
            <div class="flex items-center gap-2">
              <span v-if="settings.new_api.enabled && serverNewAPIMissing.length" class="badge badge-danger">还发不了码</span>
              <span class="hint">{{ settings.new_api.enabled ? '已启用' : '已禁用' }}</span>
              <button
                class="switch"
                :class="{ 'switch-on': settings.new_api.enabled }"
                type="button"
                role="switch"
                :aria-checked="settings.new_api.enabled"
                aria-label="启用 New-API 兑换码发货"
                @click="settings.new_api.enabled = !settings.new_api.enabled"
              />
            </div>
          </div>

          <div class="grid gap-4 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <label class="label" for="new-api-base">API 基础地址</label>
              <input
                id="new-api-base"
                v-model="settings.new_api.base_url"
                class="input mono"
                placeholder="https://new-api.example.com"
              />
              <p class="hint mt-1">
                系统实际请求 <span class="mono">{{ settings.new_api.base_url || 'https://new-api.example.com' }}/api/redemption/</span>。
                仅允许 HTTP/HTTPS 公网地址，内网和云元数据地址会被拒绝。
              </p>
            </div>
            <div>
              <label class="label" for="new-api-token">管理员 AccessToken</label>
              <div class="flex gap-2">
                <input
                  id="new-api-token"
                  v-model="settings.new_api.admin_access_token"
                  type="password"
                  class="input mono min-w-0 flex-1"
                  placeholder="保持 ******** 则不修改"
                  autocomplete="off"
                />
                <button class="btn btn-quiet btn-sm shrink-0" type="button" @click="clearSecret('new-api-token')">清除</button>
              </div>
              <p class="hint mt-1">对应 Authorization Bearer；只在服务端保存，不会回显明文。</p>
            </div>
            <div>
              <label class="label" for="new-api-user">管理员用户 ID</label>
              <input id="new-api-user" v-model="settings.new_api.admin_user_id" class="input mono" inputmode="numeric" placeholder="例如 1" />
              <p class="hint mt-1">对应 New-Api-User 请求头，必须是正整数。</p>
            </div>
            <div class="sm:col-span-2">
              <label class="label" for="new-api-rate">NL 与美元兑换比例</label>
              <input
                id="new-api-rate"
                v-model="settings.new_api.nl_usd_rate"
                class="input nums"
                inputmode="decimal"
                placeholder="例如 1（表示 1 NL = 1 美元）"
              />
              <p class="hint mt-1">
                填写“1 NL 等于多少美元”，必须为正数。买家下的是 NL 额度，系统按
                <span class="mono">订单 NL × 该比例 × 500000</span>
                计算上游 quota（上游以 500000 quota = 1 美元），全程由服务端计算。
              </p>
            </div>
          </div>

          <p v-if="settings.new_api.enabled && serverNewAPIMissing.length" class="alert alert-warning mt-4" role="alert">
            渠道已启用但缺少：{{ serverNewAPIMissing.join('、') }}。
          </p>
          <p class="alert alert-warning mt-4" role="status">
            上游响应按 success + data 数组解析：success=true 且 data 内是合法兑换码才会记为已发码；
            success=false、缺少 data 或码格式不合法都会作为失败处理，不会伪造空兑换码。
          </p>
        </div>

        <div v-show="settingsTab === 'smtp'" class="card">
          <div class="mb-5">
            <h3 class="font-semibold">SMTP 邮件设置</h3>
            <p class="hint mt-0.5">配置保存到应用运行时设置，仅管理员可测试；密码只显示掩码。</p>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <label class="flex items-center gap-3 text-sm font-semibold"><input v-model="settings.smtp.enabled" type="checkbox" class="size-4 accent-[var(--accent)]" />启用 SMTP 发信</label>
            <div>
              <label class="label" for="smtp-port">端口</label>
              <input id="smtp-port" v-model.number="settings.smtp.port" class="input mono" type="number" min="1" max="65535" />
            </div>
            <div>
              <label class="label" for="smtp-host">SMTP Host</label>
              <input id="smtp-host" v-model="settings.smtp.host" class="input mono" placeholder="smtp.example.com" />
            </div>
            <div>
              <label class="label" for="smtp-secure">加密</label>
              <select id="smtp-secure" v-model="settings.smtp.secure" class="input"><option value="ssl">SSL（465）</option><option value="starttls">STARTTLS（587）</option></select>
            </div>
            <div>
              <label class="label" for="smtp-user">用户名</label>
              <input id="smtp-user" v-model="settings.smtp.username" class="input" autocomplete="username" />
            </div>
            <div>
              <label class="label" for="smtp-pass">密码</label>
              <div class="flex gap-2">
                <input id="smtp-pass" v-model="settings.smtp.password" class="input min-w-0 flex-1" type="password" placeholder="保持 ******** 则不修改" autocomplete="new-password" />
                <button class="btn btn-quiet btn-sm shrink-0" type="button" @click="clearSecret('smtp-pass')">清除</button>
              </div>
            </div>
            <div class="sm:col-span-2">
              <label class="label" for="smtp-from">发件人（名称 + 地址）</label>
              <input id="smtp-from" v-model="settings.smtp.from" class="input" placeholder="Kaoqy Shop <mailer@example.com>" />
            </div>
          </div>
          <div class="mt-4 flex flex-col gap-2 sm:flex-row">
            <input v-model="mailRecipient" class="input min-w-0 flex-1" type="email" placeholder="测试收件人邮箱" autocomplete="email" />
            <button class="btn btn-secondary shrink-0" type="button" :disabled="testingMail || !mailRecipient.trim()" @click="runMailTest">
              {{ testingMail ? '发送中…' : '发送测试邮件' }}
            </button>
          </div>
          <p v-if="mailMessage" class="alert mt-3" :class="mailMessage.startsWith('失败') ? 'alert-danger' : 'alert-success'" role="status">{{ mailMessage }}</p>
          <p class="hint mt-3">支持 SMTP 465 SSL 或 SMTP 587 STARTTLS。测试邮件只验证连接、TLS、认证和发信。</p>
          <div class="smtp-notes">
            <p class="smtp-note-title">填之前先确认三件事</p>
            <ul class="smtp-note-list">
              <li>
                <strong>发件地址要和 SMTP 账号同域</strong>，否则多数邮箱服务商会拒收或被判为垃圾邮件。
                例如用 QQ 邮箱发信，发件人就写你自己的 QQ 邮箱地址。
              </li>
              <li>
                <strong>密码填授权码，不是网页登录密码。</strong>
                QQ 邮箱在「设置 → 账户 → POP3/IMAP/SMTP 服务」里开启后生成授权码；
                腾讯企业邮、网易、Gmail 同理，都要在邮箱后台单独开 SMTP 并复制授权码。
              </li>
              <li>
                <strong>测试收件人先填你自己的邮箱</strong>，能收到再换成老板或客服组邮箱，
                免得配错了还发不到人。
              </li>
            </ul>
            <p class="hint mt-2">
              提醒事件默认发到客服团队（员工站内信 + 员工邮箱）。若要把「订单异常」单独发到某个邮箱，
              需要在提醒事件里把收件人写成该邮箱地址，并勾选「邮件」渠道；
              目前配置中心还不支持在事件开关旁边直接指定邮箱地址。
            </p>
          </div>
        </div>

        <!-- 公告与页脚 -->
        <div v-show="settingsTab === 'content'" class="card">
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
        <div v-show="settingsTab === 'content'" class="card">
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

        <div v-show="settingsTab === 'site'" class="card">
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

      </fieldset>
    </div>

    <!-- 登录记录 -->
    <div v-show="canViewAttempts && settingsTab === 'oauth'" class="card">
      <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 class="font-semibold">最近 NodeLoc 登录记录</h3>
          <p class="hint mt-0.5">买家说「登录不了」时先看这里：卡在哪一步、商店记下的原因，以及它当时发出的回调地址</p>
        </div>
        <div class="flex items-center gap-2">
          <span v-if="attempts.length" class="hint mono">最近 {{ attempts.length }} 条</span>
          <button class="btn btn-secondary btn-sm" type="button" :disabled="loadingAttempts" @click="loadOAuthAttempts">
            {{ loadingAttempts ? '读取中…' : '刷新' }}
          </button>
        </div>
      </div>

      <p v-if="attemptsError" class="alert alert-danger">{{ attemptsError }}</p>
      <div v-else-if="loadingAttempts && !attempts.length" class="space-y-2">
        <div v-for="i in 3" :key="i" class="skeleton h-12" />
      </div>
      <p v-else-if="!attempts.length" class="hint leading-relaxed">
        还没有登录记录。让人从登录页走一次「用 NodeLoc 登录」（成功或失败都会记下来），这里就会写下它停在哪一步。
        上面的「测试 OAuth 配置」只核对 Client ID 与 Secret，不会留下记录。
      </p>
      <ul v-else class="text-sm">
        <li v-for="(row, index) in attempts" :key="row.id" :class="index ? 'mt-3 border-t border-[var(--stroke-quiet)] pt-3' : ''">
          <div class="flex flex-wrap items-center gap-2">
            <span class="mono text-xs text-[var(--text-quiet)]">{{ when(row.created_at) }}</span>
            <span class="font-semibold">{{ oauthStep(row.step) }}</span>
            <span :class="['badge', oauthOutcome(row.outcome).badge]">{{ oauthOutcome(row.outcome).label }}</span>
            <span v-if="row.binding" class="badge badge-neutral">绑定已有账号</span>
            <span v-if="row.username" class="badge badge-info">{{ row.username }}</span>
          </div>
          <p v-if="oauthReason(row.reason)" class="mt-1.5 text-[var(--text)]">{{ oauthReason(row.reason) }}</p>
          <p v-if="row.detail" class="codebox mt-2 text-xs">{{ row.detail }}</p>
          <p v-if="row.redirect_uri" class="hint mt-1.5 break-all">
            回调地址：<span class="mono">{{ row.redirect_uri }}</span>
          </p>
        </li>
      </ul>
    </div>
      </div>
    </div>
  </section>
</template>

<style scoped>
.settings-page-head {
  display: flex;
  flex-wrap: wrap;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4);
  border-bottom: 1px solid var(--stroke-quiet);
  padding-bottom: var(--space-5);
}

.settings-page-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
}

.settings-layout {
  display: grid;
  gap: var(--space-5);
}

.settings-nav-panel {
  border: 1px solid var(--stroke);
  border-radius: var(--radius-xl);
  background: color-mix(in srgb, var(--surface) 95%, transparent);
  box-shadow: var(--shadow-xs);
  padding: var(--space-2);
}

/* 设置分组导航：和配置中心使用同一套视觉语言，后台看起来是一个系统。 */
.settings-nav-item {
  position: relative;
  display: block;
  padding: 8px 10px 8px 12px;
  border-radius: var(--radius-sm);
  color: var(--text-dim);
  transition: background var(--fast), color var(--fast);
}
.settings-nav-item::before {
  content: '';
  position: absolute;
  left: 0;
  top: 50%;
  width: 3px;
  height: 0;
  border-radius: var(--radius-pill);
  background: var(--accent);
  transform: translateY(-50%);
  transition: height var(--normal) var(--spring);
}
.settings-nav-item:hover { background: var(--surface-hi); color: var(--text); }
.settings-nav-active { background: var(--accent-soft); color: var(--accent); font-weight: 650; }
.settings-nav-active::before { height: 20px; }
.smtp-notes {
  margin-top: 14px;
  padding: 14px 16px;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-lg, 12px);
  background: var(--surface);
}
.smtp-note-title { font-size: 12.5px; font-weight: 650; color: var(--text-dim); }
.smtp-note-list { margin-top: 8px; display: flex; flex-direction: column; gap: 7px; }

@media (min-width: 1024px) {
  .settings-layout {
    grid-template-columns: 232px minmax(0, 1fr);
    align-items: start;
  }
}

@media (max-width: 640px) {
  .settings-page-actions {
    width: 100%;
  }

  .settings-page-actions > * {
    flex: 1;
  }
}
.smtp-note-list li {
  position: relative;
  padding-left: 15px;
  font-size: 12px;
  line-height: 1.65;
  color: var(--text-quiet);
}
.smtp-note-list li::before {
  content: "";
  position: absolute;
  left: 2px;
  top: 7px;
  width: 4px;
  height: 4px;
  border-radius: 50%;
  background: var(--stroke);
}
.smtp-note-list strong { color: var(--text-dim); font-weight: 650; }
</style>
