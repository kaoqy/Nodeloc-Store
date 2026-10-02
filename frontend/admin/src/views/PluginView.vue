<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  deleteBinding,
  disablePlugin,
  enablePlugin,
  enrollPlugin,
  listBindings,
  listPluginCatalog,
  saveBinding,
  uninstallPlugin,
  updatePluginConfig,
} from '../api/plugins'
import { listProducts } from '../api/products'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { PluginBinding, PluginCatalogEntry, PluginConfigField, Product, ProductFormField } from '../types'

const auth = useAuthStore()
const canManage = computed(() => auth.allows('plugins', 'manage'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const message = ref('')
const entries = ref<PluginCatalogEntry[]>([])
const products = ref<Product[]>([])
const bindings = ref<PluginBinding[]>([])

const activating = ref<PluginCatalogEntry | null>(null)
const configDraft = ref<{ settings: Record<string, string>; secrets: Record<string, string> }>({
  settings: {},
  secrets: {},
})
const bindingDraft = ref<{
  id?: number
  plugin_id: number
  product_id: number
  match_field: string
  value: string
  remote_name: string
  remote_ref: string
  sort_order: number
  is_enabled: boolean
} | null>(null)

const installed = computed(() => entries.value.filter((entry) => entry.installed))

function capabilityLabel(value: string): string {
  if (value === 'form') return '购买表单'
  if (value === 'fulfill') return '订单交付'
  if (value === 'notify') return '买家通知'
  return value
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [catalog, productList, rules] = await Promise.all([
      listPluginCatalog(),
      listProducts(),
      listBindings(),
    ])
    entries.value = catalog
    products.value = productList
    bindings.value = rules
  } catch (err) {
    error.value = errorMessage(err, '插件信息加载失败')
  } finally {
    loading.value = false
  }
}

async function install(entry: PluginCatalogEntry) {
  if (busy.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    const plugin = await enrollPlugin(entry.manifest.key)
    message.value = `已安装「${plugin.name}」，接下来填写配置并启用。`
    await load()
    const fresh = entries.value.find((item) => item.manifest.key === entry.manifest.key)
    if (fresh) openConfig(fresh)
  } catch (err) {
    error.value = errorMessage(err, '安装插件失败')
  } finally {
    busy.value = false
  }
}

function openConfig(entry: PluginCatalogEntry) {
  const settings: Record<string, string> = {}
  for (const field of entry.manifest.config_schema) {
    if (field.type === 'password') continue
    settings[field.key] = entry.settings?.[field.key] ?? ''
  }
  configDraft.value = { settings, secrets: {} }
  activating.value = entry
}

async function saveConfig() {
  const entry = activating.value
  if (!entry?.plugin_id || busy.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    await updatePluginConfig(entry.plugin_id, configDraft.value.settings, configDraft.value.secrets)
    message.value = '配置已保存。'
    await load()
    const fresh = entries.value.find((item) => item.plugin_id === entry.plugin_id)
    activating.value = fresh ?? null
  } catch (err) {
    error.value = errorMessage(err, '保存插件配置失败')
  } finally {
    busy.value = false
  }
}

async function toggle(entry: PluginCatalogEntry) {
  if (!entry.plugin_id || busy.value) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    if (entry.enabled) {
      await disablePlugin(entry.plugin_id)
      message.value = `已停用「${entry.manifest.name}」，新订单不再走这个插件。`
    } else {
      await enablePlugin(entry.plugin_id)
      message.value = `已启用「${entry.manifest.name}」。`
    }
    await load()
  } catch (err) {
    // The server refuses to enable a plugin whose required configuration is
    // still missing, and says which field to fill in.
    error.value = errorMessage(err, '切换插件状态失败')
    if (!entry.enabled) openConfig(entry)
  } finally {
    busy.value = false
  }
}

async function remove(entry: PluginCatalogEntry) {
  if (!entry.plugin_id || busy.value) return
  const count = entry.binding_count
  const warning = count
    ? `删除插件「${entry.manifest.name}」会一并删除 ${count} 条商品匹配规则，已交付的订单不受影响。确认继续？`
    : `删除插件「${entry.manifest.name}」？已交付的订单不受影响。`
  if (!confirm(warning)) return
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    await uninstallPlugin(entry.plugin_id)
    message.value = '插件已移除。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '移除插件失败')
  } finally {
    busy.value = false
  }
}

function startBinding(entry: PluginCatalogEntry, existing?: PluginBinding) {
  if (!entry.plugin_id) return
  bindingDraft.value = existing
    ? {
        id: existing.id,
        plugin_id: existing.plugin_id,
        product_id: existing.product_id,
        match_field: existing.match_field ?? '',
        value: existing.value,
        remote_name: existing.remote_name ?? '',
        remote_ref: existing.remote_ref,
        sort_order: existing.sort_order ?? 0,
        is_enabled: existing.is_enabled,
      }
    : {
        plugin_id: entry.plugin_id,
        product_id: products.value[0]?.id ?? 0,
        match_field: '',
        value: '',
        remote_name: '',
        remote_ref: '',
        sort_order: 0,
        is_enabled: true,
      }
}

/** The form fields a chosen product offers, so the match field is a picker. */
const bindingFields = computed<ProductFormField[]>(() => {
  const productID = bindingDraft.value?.product_id
  const product = products.value.find((item) => item.id === productID)
  if (!product?.form_schema) return []
  try {
    const fields = JSON.parse(product.form_schema) as ProductFormField[]
    return Array.isArray(fields) ? fields : []
  } catch {
    return []
  }
})

async function submitBinding() {
  const draft = bindingDraft.value
  if (!draft || busy.value) return
  if (!draft.product_id) {
    error.value = '请选择要绑定到哪个商品。'
    return
  }
  busy.value = true
  error.value = ''
  message.value = ''
  try {
    await saveBinding(draft)
    message.value = '匹配规则已保存。'
    bindingDraft.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存匹配规则失败')
  } finally {
    busy.value = false
  }
}

async function removeBinding(binding: PluginBinding) {
  if (busy.value) return
  if (!confirm('删除这条匹配规则？买家再选这个选项时会提示没有对应交付项目。')) return
  busy.value = true
  error.value = ''
  try {
    await deleteBinding(binding.id)
    message.value = '匹配规则已删除。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除匹配规则失败')
  } finally {
    busy.value = false
  }
}

function productName(id: number): string {
  return products.value.find((item) => item.id === id)?.name ?? `#${id}`
}

onMounted(load)
</script>

<template>
  <section class="space-y-5">
    <header class="flex flex-wrap items-end justify-between gap-3">
      <div>
        <p class="eyebrow">扩展</p>
        <h1 class="mt-1 text-xl font-bold">插件管理</h1>
        <p class="hint mt-1 max-w-2xl">
          插件在这台服务器上运行，不需要下载或执行第三方代码。安装一个插件、填好配置并启用后，
          就能把商品的购买选项匹配到具体的交付项目。
        </p>
      </div>
    </header>

    <p v-if="message" class="alert alert-success" role="status">{{ message }}</p>
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div v-if="loading" class="space-y-3">
      <div v-for="i in 2" :key="i" class="skeleton h-24 rounded-[var(--radius-md)]" />
    </div>

    <template v-else>
      <article v-for="entry in entries" :key="entry.manifest.key" class="card">
        <div class="flex flex-wrap items-start justify-between gap-4">
          <div class="min-w-0 flex-1">
            <div class="flex flex-wrap items-center gap-2">
              <h2 class="text-[15px] font-bold">{{ entry.manifest.name }}</h2>
              <span class="badge mono">{{ entry.manifest.key }}</span>
              <span v-if="entry.enabled" class="badge badge-success">已启用</span>
              <span v-else-if="entry.installed" class="badge badge-neutral">已安装 · 未启用</span>
              <span v-else class="badge badge-neutral">未安装</span>
            </div>
            <p class="mt-2 text-[13px] leading-relaxed text-[var(--text-dim)]">
              {{ entry.manifest.description }}
            </p>
            <div class="mt-2.5 flex flex-wrap items-center gap-2">
              <span v-for="cap in entry.manifest.capabilities" :key="cap" class="badge badge-info">
                {{ capabilityLabel(cap) }}
              </span>
              <span class="hint">v{{ entry.manifest.version }} · {{ entry.manifest.author }}</span>
              <span v-if="entry.installed" class="hint">· {{ entry.binding_count }} 条匹配规则</span>
            </div>
          </div>

          <div v-if="canManage" class="flex shrink-0 flex-wrap gap-2">
            <template v-if="!entry.installed">
              <button class="btn btn-primary btn-sm" :disabled="busy" @click="install(entry)">安装</button>
            </template>
            <template v-else>
              <button class="btn btn-secondary btn-sm" :disabled="busy" @click="openConfig(entry)">配置</button>
              <button
                class="btn btn-sm"
                :class="entry.enabled ? 'btn-quiet' : 'btn-primary'"
                :disabled="busy"
                @click="toggle(entry)"
              >
                {{ entry.enabled ? '停用' : '启用' }}
              </button>
              <button class="btn btn-danger btn-sm" :disabled="busy" @click="remove(entry)">移除</button>
            </template>
          </div>
        </div>

        <!-- 匹配规则: one row per purchase-form answer that selects a delivery item. -->
        <template v-if="entry.installed && entry.plugin_id">
          <div class="my-4 divider" />
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <p class="text-[13px] font-semibold">商品匹配规则</p>
              <p class="hint">
                买家在购买表单里选的选项，按下面这张表映射到具体的交付项目。
              </p>
            </div>
            <button v-if="canManage" class="btn btn-secondary btn-sm" @click="startBinding(entry)">+ 新增规则</button>
          </div>

          <div class="table-container mt-3">
            <table>
              <thead>
                <tr>
                  <th>商品</th>
                  <th>匹配字段</th>
                  <th>选项值</th>
                  <th>交付项目</th>
                  <th>编号</th>
                  <th>状态</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="binding in bindings.filter((item) => item.plugin_id === entry.plugin_id)"
                  :key="binding.id"
                >
                  <td class="text-sm">{{ productName(binding.product_id) }}</td>
                  <td class="mono text-sm quiet">{{ binding.match_field || '—' }}</td>
                  <td class="mono text-sm">{{ binding.value || '（无需选择）' }}</td>
                  <td class="text-sm">{{ binding.remote_name || '—' }}</td>
                  <td class="mono text-sm quiet">{{ binding.remote_ref }}</td>
                  <td>
                    <span class="badge" :class="binding.is_enabled ? 'badge-success' : 'badge-neutral'">
                      {{ binding.is_enabled ? '生效中' : '已停用' }}
                    </span>
                  </td>
                  <td>
                    <div v-if="canManage" class="flex justify-end gap-2">
                      <button class="btn btn-quiet btn-sm" @click="startBinding(entry, binding)">编辑</button>
                      <button class="btn btn-danger btn-sm" @click="removeBinding(binding)">删除</button>
                    </div>
                  </td>
                </tr>
                <tr v-if="!bindings.some((item) => item.plugin_id === entry.plugin_id)">
                  <td colspan="7">
                    <div class="empty-state">
                      <p class="empty-title">还没有匹配规则</p>
                      <p class="empty-hint">绑定一个商品后，买家选中的选项才会对应到具体交付项目。</p>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </template>
      </article>
    </template>

    <!-- 配置弹窗: the provider's own schema, so this screen stays generic. -->
    <div v-if="activating" class="overlay" role="dialog" aria-modal="true" @click.self="activating = null">
      <div class="card w-full max-w-lg">
        <h2 class="text-lg font-bold">{{ activating.manifest.name }} · 配置</h2>
        <p class="hint mt-1">{{ activating.manifest.description }}</p>

        <form class="mt-5 space-y-4" @submit.prevent="saveConfig">
          <div v-for="field in activating.manifest.config_schema" :key="field.key">
            <label class="label" :for="`cfg-${field.key}`">
              {{ field.label }}
              <span v-if="field.required" class="text-[var(--danger)]">*</span>
            </label>

            <select
              v-if="field.type === 'select'"
              :id="`cfg-${field.key}`"
              v-model="configDraft.settings[field.key]"
              class="input"
            >
              <option value="">请选择…</option>
              <option v-for="option in field.options" :key="option" :value="option">{{ option }}</option>
            </select>
            <label v-else-if="field.type === 'bool'" class="flex items-center gap-2 text-sm">
              <input v-model="configDraft.settings[field.key]" type="checkbox" />
              开启
            </label>
            <input
              v-else
              :id="`cfg-${field.key}`"
              v-model="configDraft.secrets[field.key]"
              class="input"
              :type="field.type === 'password' ? 'password' : 'text'"
              :placeholder="field.placeholder"
              autocomplete="off"
            />
            <p v-if="field.help" class="hint mt-1">{{ field.help }}</p>
            <p
              v-if="field.type === 'password' && activating.secret_fields?.includes(field.key)"
              class="hint mt-1"
            >
              已保存一个值；留空保持原样，输入新值则覆盖。
            </p>
          </div>

          <div class="flex gap-2 pt-2">
            <button class="btn btn-primary btn-sm" type="submit" :disabled="busy">
              <span v-if="busy" class="spinner spinner-light" />
              {{ busy ? '保存中…' : '保存配置' }}
            </button>
            <button class="btn btn-quiet btn-sm" type="button" @click="activating = null">取消</button>
          </div>
        </form>
      </div>
    </div>

    <!-- 匹配规则弹窗 -->
    <div v-if="bindingDraft" class="overlay" role="dialog" aria-modal="true" @click.self="bindingDraft = null">
      <div class="card w-full max-w-lg">
        <h2 class="text-lg font-bold">{{ bindingDraft.id ? '编辑匹配规则' : '新增匹配规则' }}</h2>
        <p class="hint mt-1">买家选择的选项，将决定这一单具体交付哪个项目。</p>

        <form class="mt-5 space-y-4" @submit.prevent="submitBinding">
          <div>
            <label class="label" for="bind-product">商品</label>
            <select id="bind-product" v-model.number="bindingDraft.product_id" class="input">
              <option :value="0" disabled>请选择商品…</option>
              <option v-for="product in products" :key="product.id" :value="product.id">
                {{ product.name }}
              </option>
            </select>
          </div>

          <div>
            <label class="label" for="bind-field">匹配字段（购买表单项）</label>
            <select v-if="bindingFields.length" id="bind-field" v-model="bindingDraft.match_field" class="input">
              <option value="">（不需要选择）</option>
              <option v-for="field in bindingFields" :key="field.key" :value="field.key">
                {{ field.label }}（{{ field.key }}）
              </option>
            </select>
            <input
              v-else
              id="bind-field"
              v-model="bindingDraft.match_field"
              class="input"
              placeholder="例如 region；留空表示这个商品不需要选择"
            />
            <p class="hint mt-1">
              用哪个购买表单项来匹配。留空时，这个商品会固定交付下面这一项。
            </p>
          </div>

          <div>
            <label class="label" for="bind-value">选项值</label>
            <input
              id="bind-value"
              v-model="bindingDraft.value"
              class="input"
              placeholder="例如 beijing（大小写与首尾空格会自动忽略）"
            />
          </div>

          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="bind-name">交付项目名称</label>
              <input id="bind-name" v-model="bindingDraft.remote_name" class="input" placeholder="例如 北京节点" />
            </div>
            <div>
              <label class="label" for="bind-ref">交付项目编号 <span class="text-[var(--danger)]">*</span></label>
              <input id="bind-ref" v-model="bindingDraft.remote_ref" class="input" placeholder="SKU / 套餐 ID / 模板 ID" />
            </div>
          </div>

          <label class="flex items-center gap-2 text-sm">
            <input v-model="bindingDraft.is_enabled" type="checkbox" />
            生效
          </label>

          <div class="flex gap-2 pt-2">
            <button class="btn btn-primary btn-sm" type="submit" :disabled="busy">
              <span v-if="busy" class="spinner spinner-light" />
              {{ busy ? '保存中…' : '保存规则' }}
            </button>
            <button class="btn btn-quiet btn-sm" type="button" @click="bindingDraft = null">取消</button>
          </div>
        </form>
      </div>
    </div>
  </section>
</template>
