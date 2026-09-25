<script setup lang="ts">
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { install } from '../api/system'

const router = useRouter()
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

const previewRedirect = computed(() => {
  if (form.oauth_redirect_uri.trim()) return form.oauth_redirect_uri.trim()
  if (!form.domain.trim()) return ''
  return `${form.scheme}://${form.domain.trim()}/api/v1/auth/oauth/callback`
})

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
        enabled: true,
        payment_id: form.payment_id.trim(),
        secret_key: form.payment_secret.trim(),
      },
      admin: {
        username: form.admin_username.trim(),
        email: form.admin_email.trim(),
        password: form.admin_password,
      },
      features: { enabled_registration: true },
      theme: { theme_primary: '#6366f1', default_locale: 'zh-CN' },
    })
    done.value = true
  } catch (err: any) {
    error.value = err.message || '初始化失败'
  } finally {
    submitting.value = false
  }
}

function goLogin() {
  router.push('/login')
}
</script>

<template>
  <div class="flex min-h-screen items-center justify-center px-4 py-10">
    <div class="w-full max-w-xl">
      <!-- 完成页 -->
      <div v-if="done" class="card rise-in p-8 text-center">
        <div class="mx-auto mb-6 grid size-16 place-items-center rounded-[20px] bg-gradient-to-br from-emerald-400 to-teal-600 text-3xl shadow-[0_18px_44px_-10px_rgba(16,185,129,0.55)]">✓</div>
        <h1 class="text-2xl font-black">初始化完成</h1>
        <p class="mt-3 text-sm text-[#b3b1c4]">
          数据库已迁移，运行时配置已写入系统。现在可以使用刚创建的管理员账号登录后台，之后所有配置都可以在「系统设置」中修改。
        </p>
        <button class="btn-primary mt-8 w-full" @click="goLogin">前往管理登录</button>
      </div>

      <!-- 向导 -->
      <div v-else class="card p-6 sm:p-8">
        <div class="mb-6 text-center">
          <div class="mx-auto mb-4 grid size-14 place-items-center rounded-2xl bg-gradient-to-br from-indigo-400 via-indigo-600 to-purple-600 text-2xl font-black text-white shadow-[inset_0_2px_0_rgba(255,255,255,0.5),0_18px_44px_-10px_rgba(99,102,241,0.6)]">NL</div>
          <h1 class="text-2xl font-black tracking-tight">初始化 <span class="gradient-text">Nodeloc Store</span></h1>
          <p class="mt-2 text-sm text-[#7a7890]">无需编辑任何配置文件，按步骤完成首次配置即可</p>
        </div>

        <!-- 步骤条 -->
        <div class="mb-7 flex items-center gap-2">
          <template v-for="(label, i) in steps" :key="label">
            <div class="flex flex-1 flex-col items-center gap-1.5">
              <div
                :class="[
                  'grid size-7 place-items-center rounded-full text-xs font-bold transition',
                  i < step ? 'bg-emerald-500/90 text-white' : i === step ? 'btn-primary !size-7 !p-0' : 'border border-white/15 bg-white/[0.06] text-[#7a7890]',
                ]"
              >
                {{ i < step ? '✓' : i + 1 }}
              </div>
              <span :class="['text-[11px]', i === step ? 'text-white' : 'text-[#7a7890]']">{{ label }}</span>
            </div>
            <div v-if="i < steps.length - 1" class="mb-5 h-px flex-1 bg-white/15" />
          </template>
        </div>

        <p v-if="error" class="mb-4 rounded-xl border border-rose-400/30 bg-rose-500/10 px-4 py-2.5 text-sm text-rose-200">{{ error }}</p>

        <!-- 第一步：站点 + 数据库 -->
        <div v-if="step === 0" class="space-y-4">
          <div>
            <label class="mb-1 block text-sm text-[#b3b1c4]">网站名称</label>
            <input v-model="form.site_name" class="input" placeholder="Nodeloc Store" />
          </div>
          <div>
            <label class="mb-1 block text-sm text-[#b3b1c4]">站点域名 <span class="text-rose-400">*</span></label>
            <input v-model="form.domain" class="input" placeholder="store.example.com" />
            <p class="mt-1 text-xs text-[#7a7890]">用于生成 OAuth 回调地址，不含协议与斜杠</p>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">协议</label>
              <select v-model="form.scheme" class="input">
                <option value="https">https</option>
                <option value="http">http</option>
              </select>
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">数据库</label>
              <select v-model="form.db_driver" class="input">
                <option value="sqlite">SQLite（默认，零配置）</option>
                <option value="mysql">MySQL</option>
              </select>
            </div>
          </div>
          <div v-if="form.db_driver === 'mysql'">
            <label class="mb-1 block text-sm text-[#b3b1c4]">MySQL DSN <span class="text-rose-400">*</span></label>
            <input v-model="form.db_dsn" class="input" placeholder="user:pass@tcp(host:3306)/nodeloc_store?charset=utf8mb4&parseTime=True&loc=Local" />
          </div>
          <p v-else class="rounded-xl border border-white/10 bg-white/[0.04] px-4 py-2.5 text-xs text-[#7a7890]">
            SQLite 将自动创建数据文件（data/store.db），无需额外配置。
          </p>
        </div>

        <!-- 第二步：NodeLoc OAuth + Payment -->
        <div v-else-if="step === 1" class="space-y-4">
          <div>
            <label class="mb-1 block text-sm text-[#b3b1c4]">NodeLoc 站点地址</label>
            <input v-model="form.oauth_base_url" class="input" placeholder="https://www.nodeloc.com" />
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">OAuth Client ID <span class="text-rose-400">*</span></label>
              <input v-model="form.oauth_client_id" class="input" placeholder="在 NodeLoc OAuth 应用详情中获取" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">OAuth Client Secret <span class="text-rose-400">*</span></label>
              <input v-model="form.oauth_client_secret" type="password" class="input" placeholder="••••••••" autocomplete="off" />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm text-[#b3b1c4]">OAuth 回调地址（可留空自动生成）</label>
            <input v-model="form.oauth_redirect_uri" class="input" :placeholder="previewRedirect || 'https://你的域名/api/v1/auth/oauth/callback'" />
            <p class="mt-1 text-xs text-[#7a7890]">请将该地址添加到 NodeLoc OAuth 应用的重定向 URI 白名单中</p>
          </div>
          <div class="h-px bg-white/10" />
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">Payments 支付 ID <span class="text-[#7a7890]">(可稍后在设置中填写)</span></label>
              <input v-model="form.payment_id" class="input" placeholder="例如 12" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">支付密钥 Secret Key</label>
              <input v-model="form.payment_secret" type="password" class="input" placeholder="••••••••" autocomplete="off" />
            </div>
          </div>
        </div>

        <!-- 第三步：管理员 -->
        <div v-else class="space-y-4">
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">管理员用户名 <span class="text-rose-400">*</span></label>
              <input v-model="form.admin_username" class="input" placeholder="admin" autocomplete="username" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#b3b1c4]">邮箱（可选）</label>
              <input v-model="form.admin_email" class="input" placeholder="you@example.com" autocomplete="email" />
            </div>
          </div>
          <div>
            <label class="mb-1 block text-sm text-[#b3b1c4]">管理员密码 <span class="text-rose-400">*</span></label>
            <input v-model="form.admin_password" type="password" class="input" placeholder="至少 8 位" autocomplete="new-password" />
          </div>
          <div class="rounded-xl border border-white/10 bg-white/[0.04] p-4 text-xs leading-relaxed text-[#7a7890]">
            <p class="mb-1 font-semibold text-[#b3b1c4]">确认摘要</p>
            <p>站点：{{ form.scheme }}://{{ form.domain || '(未填写)' }} · 数据库：{{ form.db_driver }}</p>
            <p>OAuth：{{ form.oauth_client_id ? `Client ${form.oauth_client_id}` : '(未填写)' }} · 回调：{{ previewRedirect || '(自动生成)' }}</p>
            <p>支付：{{ form.payment_id ? `Payment ID ${form.payment_id}` : '未配置（可稍后设置）' }} · 管理员：{{ form.admin_username || '(未填写)' }}</p>
          </div>
        </div>

        <!-- 操作按钮 -->
        <div class="mt-7 flex items-center justify-between gap-3">
          <button v-if="step > 0" class="btn btn-secondary" :disabled="submitting" @click="back">上一步</button>
          <span v-else />
          <button v-if="step < 2" class="btn-primary" @click="next">下一步</button>
          <button v-else class="btn-primary min-w-32" :disabled="submitting" @click="submit">
            {{ submitting ? '正在初始化…' : '完成初始化' }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
