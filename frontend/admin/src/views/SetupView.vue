<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { install } from '../api/system'

const step = ref(0)
const submitting = ref(false)
const error = ref('')
const done = ref(false)

const form = reactive({
  site_name: 'Nodeloc Store',
  site_slogan: '数字商品交易平台',
  scheme: 'https' as 'https' | 'http',
  domain: '',
  db_driver: 'sqlite',
  db_dsn: '',
  oauth_base_url: 'https://www.nodeloc.com',
  oauth_client_id: '',
  oauth_client_secret: '',
  oauth_redirect_uri: '',
  payment_id: '',
  payment_secret: '',
  admin_username: '',
  admin_email: '',
  admin_password: '',
})

const steps = ['站点与数据库', 'NodeLoc 集成', '管理员账号']

const siteOrigin = computed(() => (form.domain.trim() ? `${form.scheme}://${form.domain.trim()}` : '（待填写）'))

const previewRedirect = computed(() => {
  if (form.oauth_redirect_uri.trim()) return form.oauth_redirect_uri.trim()
  if (!form.domain.trim()) return ''
  return `${form.scheme}://${form.domain.trim()}/api/v1/auth/oauth/callback`
})

const paymentReady = computed(() => form.payment_id.trim() !== '' && form.payment_secret.trim() !== '')

function next() {
  error.value = ''
  if (step.value === 0) {
    if (!form.domain.trim()) {
      error.value = '请填写站点域名（例如 store.example.com）'
      return
    }
    if (form.db_driver === 'mysql' && !form.db_dsn.trim()) {
      error.value = 'MySQL 需要填写 DSN'
      return
    }
  }
  if (step.value === 1) {
    if (!form.oauth_client_id.trim() || !form.oauth_client_secret.trim()) {
      error.value = 'NodeLoc OAuth 的 Client ID 与 Client Secret 为必填项'
      return
    }
  }
  step.value += 1
}

function back() {
  error.value = ''
  step.value -= 1
}

async function submit() {
  error.value = ''
  if (!form.admin_username.trim()) {
    error.value = '请填写管理员用户名'
    return
  }
  if (form.admin_password.length < 8) {
    error.value = '管理员密码至少需要 8 位'
    return
  }
  submitting.value = true
  try {
    await install({
      app: {
        site_name: form.site_name.trim(),
        site_slogan: form.site_slogan,
        site_description: '',
        site_logo: '',
        scheme: form.scheme,
        domain: form.domain.trim(),
      },
      database: { driver: form.db_driver, dsn: form.db_dsn.trim() },
      oauth: {
        enabled: true,
        base_url: form.oauth_base_url.trim(),
        client_id: form.oauth_client_id.trim(),
        client_secret: form.oauth_client_secret.trim(),
        redirect_uri: form.oauth_redirect_uri.trim(),
        scopes: 'openid profile email',
      },
      payment: {
        enabled: paymentReady.value,
        payment_id: form.payment_id.trim(),
        secret_key: form.payment_secret.trim(),
      },
      admin: {
        username: form.admin_username.trim(),
        email: form.admin_email.trim(),
        password: form.admin_password,
      },
      features: { enabled_registration: true },
      theme: { theme_primary: '#f2704a', default_locale: 'zh-CN' },
    })
    done.value = true
  } catch (err: any) {
    error.value = err.message || '初始化失败'
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <div class="mx-auto flex min-h-screen w-full max-w-2xl flex-col justify-center px-4 py-10 sm:py-14">
    <!-- 完成 -->
    <div v-if="done" class="card rise-in p-6 sm:p-8">
      <span class="badge badge-success">初始化完成</span>
      <h1 class="mt-4 text-2xl font-bold">商店已就绪</h1>
      <p class="mt-2 text-sm leading-relaxed text-[var(--text-dim)]">
        数据库已迁移，运行时配置已写入系统。使用刚创建的管理员账号即可登录后台，之后所有参数都能在「系统设置」中修改。
      </p>
      <dl class="mt-6 space-y-2.5 border-t border-[var(--stroke-quiet)] pt-6 text-sm">
        <div class="flex justify-between gap-4">
          <dt class="quiet">站点</dt>
          <dd class="mono">{{ siteOrigin }}</dd>
        </div>
        <div class="flex justify-between gap-4">
          <dt class="quiet">数据库</dt>
          <dd class="mono">{{ form.db_driver }}</dd>
        </div>
        <div class="flex justify-between gap-4">
          <dt class="quiet">OAuth 回调</dt>
          <dd class="mono truncate">{{ previewRedirect || '自动生成' }}</dd>
        </div>
        <div class="flex justify-between gap-4">
          <dt class="quiet">支付</dt>
          <dd class="mono">{{ paymentReady ? `Payment ID ${form.payment_id.trim()}` : '未配置' }}</dd>
        </div>
      </dl>
      <div class="mt-7 flex flex-wrap gap-3">
        <RouterLink to="/login" class="btn btn-primary">前往管理登录</RouterLink>
      </div>
    </div>

    <!-- 向导 -->
    <div v-else>
      <div class="mb-5 flex items-center gap-3">
        <span class="brand-mark">N</span>
        <div>
          <p class="eyebrow">Nodeloc Store · 首次初始化</p>
          <h1 class="mt-0.5 text-xl font-bold">三步完成配置</h1>
        </div>
      </div>

      <div class="card p-5 sm:p-7">
        <ol class="mb-6 flex items-center gap-2">
          <template v-for="(label, i) in steps" :key="label">
            <li class="flex min-w-0 flex-1 items-center gap-2">
              <span
                :class="[
                  'nums grid size-7 shrink-0 place-items-center rounded-full border text-xs font-bold transition-colors',
                  i < step
                    ? 'border-transparent bg-[var(--success-soft)] text-[var(--success)]'
                    : i === step
                      ? 'border-[var(--accent-line)] bg-[var(--accent-soft)] text-[var(--accent)]'
                      : 'border-[var(--stroke)] text-[var(--text-quiet)]',
                ]"
              >{{ i < step ? '✓' : i + 1 }}</span>
              <span :class="['truncate text-xs', i === step ? '' : 'quiet']">{{ label }}</span>
            </li>
            <li v-if="i < steps.length - 1" class="h-px flex-1 bg-[var(--stroke-quiet)]" aria-hidden="true" />
          </template>
        </ol>

        <p v-if="error" class="alert alert-danger mb-5">{{ error }}</p>

        <!-- 1. 站点与数据库 -->
        <div v-if="step === 0" class="space-y-4">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="site-name">网站名称</label>
              <input id="site-name" v-model="form.site_name" class="input" placeholder="Nodeloc Store" />
            </div>
            <div>
              <label class="label" for="site-slogan">标语</label>
              <input id="site-slogan" v-model="form.site_slogan" class="input" placeholder="数字商品交易平台" />
            </div>
          </div>
          <div>
            <label class="label" for="site-domain">站点域名 <span class="accent-text">*</span></label>
            <input id="site-domain" v-model="form.domain" class="input mono" placeholder="store.example.com" autocomplete="off" />
            <p class="hint mt-1">不含协议与结尾斜杠，用于拼接 OAuth 回调地址</p>
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="site-scheme">协议</label>
              <select id="site-scheme" v-model="form.scheme" class="input">
                <option value="https">https</option>
                <option value="http">http</option>
              </select>
            </div>
            <div>
              <label class="label" for="db-driver">数据库</label>
              <select id="db-driver" v-model="form.db_driver" class="input">
                <option value="sqlite">SQLite（默认，零配置）</option>
                <option value="mysql">MySQL</option>
              </select>
            </div>
          </div>
          <div v-if="form.db_driver === 'mysql'">
            <label class="label" for="db-dsn">MySQL DSN <span class="accent-text">*</span></label>
            <input id="db-dsn" v-model="form.db_dsn" class="input mono" placeholder="user:pass@tcp(host:3306)/nodeloc_store?charset=utf8mb4&amp;parseTime=True&amp;loc=Local" />
          </div>
          <p v-else class="hint rounded-xl border border-[var(--stroke)] bg-[var(--surface-hi)] px-4 py-3 leading-relaxed">
            SQLite 会在数据目录自动创建 store.db，无需额外配置；之后仍可在「系统设置」中切换到 MySQL。
          </p>
        </div>

        <!-- 2. NodeLoc 集成 -->
        <div v-else-if="step === 1" class="space-y-4">
          <div>
            <label class="label" for="oauth-base">NodeLoc 站点地址</label>
            <input id="oauth-base" v-model="form.oauth_base_url" class="input mono" placeholder="https://www.nodeloc.com" />
          </div>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="oauth-id">OAuth Client ID <span class="accent-text">*</span></label>
              <input id="oauth-id" v-model="form.oauth_client_id" class="input mono" placeholder="应用详情页获取" autocomplete="off" />
            </div>
            <div>
              <label class="label" for="oauth-secret">OAuth Client Secret <span class="accent-text">*</span></label>
              <input id="oauth-secret" v-model="form.oauth_client_secret" type="password" class="input mono" placeholder="••••••••" autocomplete="off" />
            </div>
          </div>
          <div>
            <label class="label" for="oauth-redirect">回调地址（可留空自动生成）</label>
            <input
              id="oauth-redirect"
              v-model="form.oauth_redirect_uri"
              class="input mono"
              :placeholder="previewRedirect || 'https://你的域名/api/v1/auth/oauth/callback'"
            />
            <p class="hint mt-1">请把该地址加入 NodeLoc OAuth 应用的重定向 URI 白名单</p>
          </div>

          <div class="divider" />

          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="payment-id">Payments 支付 ID</label>
              <input id="payment-id" v-model="form.payment_id" class="input mono" placeholder="例如 12" autocomplete="off" />
            </div>
            <div>
              <label class="label" for="payment-secret">支付 Secret Key</label>
              <input id="payment-secret" v-model="form.payment_secret" type="password" class="input mono" placeholder="••••••••" autocomplete="off" />
            </div>
          </div>
          <p class="hint">支付参数可以留空、稍后再填；未填写时买家仍能下单，但无法完成付款。</p>
        </div>

        <!-- 3. 管理员 -->
        <div v-else class="space-y-4">
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="admin-user">管理员用户名 <span class="accent-text">*</span></label>
              <input id="admin-user" v-model="form.admin_username" class="input" placeholder="admin" autocomplete="username" />
            </div>
            <div>
              <label class="label" for="admin-email">邮箱（可选）</label>
              <input id="admin-email" v-model="form.admin_email" class="input" placeholder="you@example.com" autocomplete="email" />
            </div>
          </div>
          <div>
            <label class="label" for="admin-password">管理员密码 <span class="accent-text">*</span></label>
            <input id="admin-password" v-model="form.admin_password" type="password" class="input" placeholder="至少 8 位" autocomplete="new-password" />
          </div>

          <div class="rounded-xl border border-[var(--stroke)] bg-[var(--surface-hi)] p-4">
            <p class="eyebrow mb-3">确认摘要</p>
            <dl class="space-y-2 text-sm">
              <div class="flex justify-between gap-4">
                <dt class="quiet">站点</dt>
                <dd class="mono">{{ siteOrigin }}</dd>
              </div>
              <div class="flex justify-between gap-4">
                <dt class="quiet">数据库</dt>
                <dd class="mono">{{ form.db_driver }}</dd>
              </div>
              <div class="flex justify-between gap-4">
                <dt class="quiet">OAuth</dt>
                <dd class="mono truncate">Client {{ form.oauth_client_id || '（未填写）' }}</dd>
              </div>
              <div class="flex justify-between gap-4">
                <dt class="quiet">回调</dt>
                <dd class="mono truncate">{{ previewRedirect || '自动生成' }}</dd>
              </div>
              <div class="flex justify-between gap-4">
                <dt class="quiet">支付</dt>
                <dd class="mono">{{ paymentReady ? `Payment ID ${form.payment_id.trim()}` : '稍后配置' }}</dd>
              </div>
              <div class="flex justify-between gap-4">
                <dt class="quiet">管理员</dt>
                <dd class="mono">{{ form.admin_username || '（未填写）' }}</dd>
              </div>
            </dl>
          </div>
        </div>

        <div class="mt-7 flex items-center justify-between gap-3">
          <button v-if="step > 0" class="btn btn-quiet" :disabled="submitting" @click="back">上一步</button>
          <span v-else class="hint">第 {{ step + 1 }} / {{ steps.length }} 步</span>
          <button v-if="step < steps.length - 1" class="btn btn-primary" @click="next">下一步</button>
          <button v-else class="btn btn-primary min-w-32" :disabled="submitting" @click="submit">
            <span v-if="submitting" class="spinner !size-4 !border-t-white/80" />
            {{ submitting ? '正在初始化…' : '完成初始化' }}
          </button>
        </div>
      </div>

      <p class="hint mt-4 text-center">全部参数之后都能在「系统设置」中修改，无需编辑任何配置文件。</p>
    </div>
  </div>
</template>
