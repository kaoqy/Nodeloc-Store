<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { getSettings, saveSettings } from '../api/settings'

const loading = ref(true)
const saving = ref(false)
const settings = reactive({
  site_name: '',
  site_logo: '',
  site_description: '',
  theme_primary: '#6366f1',
  default_locale: 'zh-CN',
  enabled_oauth: true,
  enabled_registration: true,
})

async function load() {
  loading.value = true
  try {
    const result = await getSettings()
    Object.assign(settings, result)
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    await saveSettings(settings)
  } finally {
    saving.value = false
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
        <p class="text-sm text-[#6b6b80]">配置平台的基本参数</p>
      </div>
      <button class="btn-primary" :disabled="saving" @click="save">
        {{ saving ? '保存中...' : '保存设置' }}
      </button>
    </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="space-y-4 lg:col-span-2">
        <div class="card">
          <h3 class="mb-4 font-semibold">基本信息</h3>
          <div class="space-y-3">
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">网站名称</label>
              <input v-model="settings.site_name" class="input" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">Logo URL</label>
              <input v-model="settings.site_logo" class="input" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">网站描述</label>
              <textarea v-model="settings.site_description" class="input min-h-20 resize-none" />
            </div>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">外观</h3>
          <div class="space-y-3">
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">主题色</label>
              <input v-model="settings.theme_primary" type="color" class="h-10 w-20 cursor-pointer rounded-lg border-0 bg-transparent" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">默认语言</label>
              <select v-model="settings.default_locale" class="input">
                <option value="zh-CN">简体中文</option>
                <option value="zh-TW">繁體中文</option>
                <option value="en">English</option>
              </select>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card">
          <h3 class="mb-4 font-semibold">功能开关</h3>
          <div class="space-y-3">
            <label class="flex items-center justify-between">
              <span class="text-sm">OAuth 登录</span>
              <button
                :class="['btn', settings.enabled_oauth ? 'btn-primary' : 'btn-secondary']"
                @click="settings.enabled_oauth = !settings.enabled_oauth"
              >
                {{ settings.enabled_oauth ? '已启用' : '已禁用' }}
              </button>
            </label>
            <label class="flex items-center justify-between">
              <span class="text-sm">开放注册</span>
              <button
                :class="['btn', settings.enabled_registration ? 'btn-primary' : 'btn-secondary']"
                @click="settings.enabled_registration = !settings.enabled_registration"
              >
                {{ settings.enabled_registration ? '已启用' : '已禁用' }}
              </button>
            </label>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">系统信息</h3>
          <div class="space-y-2 text-sm">
            <div class="flex justify-between">
              <span class="text-[#6b6b80]">版本</span>
              <span>v1.0.0</span>
            </div>
            <div class="flex justify-between">
              <span class="text-[#6b6b80]">运行环境</span>
              <span>Docker</span>
            </div>
            <div class="flex justify-between">
              <span class="text-[#6b6b80]">数据库</span>
              <span>SQLite</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
