<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  deleteTemplate,
  listNotificationLogs,
  listSystemConfigs,
  listTemplates,
  saveSystemConfig,
  saveTemplate,
  type NotificationLogRow,
  type NotificationTemplate,
  type SystemConfig,
} from '../api/configCenter'
import { errorMessage, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import SettingsSection from '../components/SettingsSection.vue'
import AgentPanel from './settings/AgentPanel.vue'

/**
 * 业务配置。
 *
 * 站点、登录、支付、SMTP 属于「系统配置」（/settings），因为那几项
 * 决定店铺能不能开门；这里只放日常运营要调的规则：
 *   - 工单规则与客服坐席
 *   - 提醒事件：哪些业务事件要提醒、走哪个渠道、发给谁
 *   - 风控与数据保留
 *
 * 分组按「店家要做的决定」划分，不是按数据表划分。
 */
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

type TabKey = 'support' | 'remind' | 'safety'

interface TabDef {
  key: TabKey
  label: string
  hint: string
  resources: string[]
}

const TABS: TabDef[] = [
  { key: 'support', label: '工单与客服', hint: '工单规则、坐席与快捷回复', resources: ['config_center', 'agents'] },
  { key: 'remind', label: '提醒事件', hint: '哪些事件要提醒、发给谁', resources: ['notification_templates', 'config_center'] },
  { key: 'safety', label: '风控与保留', hint: '限频、二次确认与数据保留', resources: ['config_center'] },
]

const visibleTabs = computed(() =>
  TABS.filter((item) => item.resources.some((resource) => auth.allows(resource, 'view'))),
)

const tab = ref<TabKey>('support')
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const templates = ref<NotificationTemplate[]>([])
const logs = ref<NotificationLogRow[]>([])
const logTotal = ref(0)
const logStatus = ref('')
const configs = ref<SystemConfig[]>([])

const templateDraft = ref<Partial<NotificationTemplate>>({
  key: '', name: '', event: '', category: 'ticket', is_enabled: true, in_app: true,
  mail: false, recipients: 'staff', title_template: '', content_template: '', retry_limit: 3, sort_order: 0,
})

const groupLabels: Record<string, string> = {
  ticket: '工单规则',
  risk: '风控与限频',
  retention: '数据保留',
  upload: '文件上传',
}

/** 一个标签页包含哪些配置分组。 */
const TAB_GROUPS: Record<TabKey, string[]> = {
  support: ['ticket'],
  remind: [],
  safety: ['risk', 'retention', 'upload'],
}

/** 提醒事件的分类，用于把模板按业务场景分组展示。 */
const categoryLabels: Record<string, string> = {
  ticket: '工单',
  order: '订单',
  activity: '活动',
  coupon: '优惠券',
  system: '系统',
}

const currentTab = computed(() => TABS.find((item) => item.key === tab.value))
const tabTitle = computed(() => currentTab.value?.label ?? '业务配置')

const canManageTemplates = computed(() => auth.allows('notification_templates', 'manage'))
const canManageSystem = computed(() => auth.allows('config_center', 'manage'))

/** 提醒事件按分类分组，便于一眼看清「订单类事哪些会提醒」。 */
const templateGroups = computed(() => {
  const map = new Map<string, NotificationTemplate[]>()
  for (const item of templates.value) {
    const key = item.category || 'system'
    const list = map.get(key) ?? []
    list.push(item)
    map.set(key, list)
  }
  return Array.from(map.entries())
})

const grouped = computed(() => {
  const map = new Map<string, SystemConfig[]>()
  for (const config of configs.value) {
    const list = map.get(config.group) ?? []
    list.push(config)
    map.set(config.group, list)
  }
  const wanted = TAB_GROUPS[tab.value] ?? []
  return wanted
    .filter((group) => map.has(group))
    .map((group) => [group, map.get(group) as SystemConfig[]] as [string, SystemConfig[]])
})

// 旧地址继续可用，落到合并后的新分组。
const LEGACY_TABS: Record<string, TabKey> = {
  ai: 'support',
  knowledge: 'support',
  service: 'support',
  ticket: 'support',
  notify: 'remind',
  risk: 'safety',
  retention: 'safety',
  upload: 'safety',
  order: 'safety',
  product: 'safety',
  activity: 'safety',
  site: 'safety',
}

function normaliseTab(value: unknown): TabKey {
  const key = typeof value === 'string' ? value : ''
  const found = TABS.find((item) => item.key === key)
  if (found) return found.key
  return LEGACY_TABS[key] ?? 'support'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [templateList, configList] = await Promise.all([
      listTemplates().catch(() => []),
      listSystemConfigs().catch(() => []),
    ])
    templates.value = templateList
    configs.value = configList
    if (tab.value === 'remind') {
      const page = await listNotificationLogs({ status: logStatus.value || undefined, limit: 50 })
        .catch(() => ({ data: [], total: 0 }))
      logs.value = page.data
      logTotal.value = page.total
    }
  } catch (err) {
    error.value = errorMessage(err, '加载配置失败')
  } finally {
    loading.value = false
  }
}

async function switchTab(next: TabKey) {
  tab.value = next
  await router.replace({ path: '/config', query: { tab: next } })
  if (next === 'remind') await load()
}

/** 直接切换一个提醒事件的启用状态，不必进编辑表单。 */
async function toggleTemplate(item: NotificationTemplate) {
  if (!canManageTemplates.value || !item.id) return
  busy.value = true
  error.value = ''
  try {
    await saveTemplate({ ...item, is_enabled: !item.is_enabled })
    item.is_enabled = !item.is_enabled
    notice.value = '提醒事件「' + item.name + '」已' + (item.is_enabled ? '启用' : '停用') + '。'
  } catch (err) {
    error.value = errorMessage(err, '更新失败')
  } finally {
    busy.value = false
  }
}

/** 切换某一个发送渠道。 */
async function toggleChannel(item: NotificationTemplate, channel: 'in_app' | 'mail') {
  if (!canManageTemplates.value || !item.id) return
  busy.value = true
  error.value = ''
  try {
    const next = { ...item, [channel]: !item[channel] }
    await saveTemplate(next)
    item[channel] = next[channel]
    notice.value = '「' + item.name + '」的' + (channel === 'mail' ? '邮件' : '站内') + '渠道已更新。'
  } catch (err) {
    error.value = errorMessage(err, '更新失败')
  } finally {
    busy.value = false
  }
}

async function saveTemplateDraft() {
  if (!templateDraft.value.key?.trim() || !templateDraft.value.name?.trim()) {
    error.value = '模板标识和名称都要填。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await saveTemplate(templateDraft.value)
    notice.value = '提醒事件已保存。'
    templateDraft.value = {
      key: '', name: '', event: '', category: 'ticket', is_enabled: true, in_app: true,
      mail: false, recipients: 'staff', title_template: '', content_template: '', retry_limit: 3, sort_order: 0,
    }
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存失败')
  } finally {
    busy.value = false
  }
}

function recipientsLabel(value?: string): string {
  const raw = (value || '').trim()
  if (!raw) return '默认（买家）'
  if (raw === 'staff') return '客服团队'
  if (raw === 'user') return '站内用户'
  if (raw === 'custom') return '自定义邮箱'
  return raw
}

function editTemplate(template: NotificationTemplate) {
  templateDraft.value = { ...template }
}

async function removeTemplate(template: NotificationTemplate) {
  if (!template.id || !window.confirm('删除提醒事件「' + template.name + '」？')) return
  try {
    await deleteTemplate(template.id)
    await load()
    notice.value = '提醒事件已删除。'
  } catch (err) {
    error.value = errorMessage(err, '删除失败')
  }
}

async function saveConfig(config: SystemConfig) {
  busy.value = true
  error.value = ''
  try {
    await saveSystemConfig(config)
    notice.value = '配置「' + (config.label || config.key) + '」已保存。'
  } catch (err) {
    error.value = errorMessage(err, '保存配置失败')
  } finally {
    busy.value = false
  }
}

function toggleBool(config: SystemConfig) {
  config.value = config.value === 'true' || config.value === '1' ? 'false' : 'true'
  void saveConfig(config)
}

watch(() => route.query.tab, (value) => { tab.value = normaliseTab(value) })

onMounted(() => {
  tab.value = normaliseTab(route.query.tab)
  void load()
})
</script>

<template>
  <section class="space-y-4">
    <div>
      <h1 class="text-lg font-bold">业务配置</h1>
      <p class="quiet mt-1 text-xs">
        日常运营要调的规则都在这里。站点、登录、支付与邮件在「系统配置」。
      </p>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div class="grid gap-4 lg:grid-cols-[240px_minmax(0,1fr)]">
      <nav class="card !p-2.5" aria-label="配置分组">
        <ul class="space-y-0.5">
          <li v-for="item in visibleTabs" :key="item.key">
            <button
              class="config-nav-item w-full text-left"
              :class="tab === item.key ? 'config-nav-active' : ''"
              :aria-current="tab === item.key ? 'page' : undefined"
              @click="switchTab(item.key)"
            >
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13px]">{{ item.label }}</span>
                <span class="quiet block truncate text-[11px]">{{ item.hint }}</span>
              </span>
            </button>
          </li>
        </ul>
      </nav>

      <div class="min-w-0 space-y-4">
        <div v-if="loading" class="card space-y-3">
          <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
        </div>

        <!-- 工单与客服 -->
        <template v-else-if="tab === 'support'">
          <SettingsSection
            title="工单规则"
            description="工单怎么编号、多久算超时、买家能不能撤销或重开。"
            resource="config_center"
          >
            <p v-if="!grouped.length" class="card py-12 text-center text-sm text-[var(--text-quiet)]">
              这个分组暂无配置项。
            </p>
            <section v-for="[group, items] in grouped" :key="group" class="config-group">
              <div class="config-group-head">
                <h3>{{ groupLabels[group] || group }}</h3>
                <span class="quiet text-[11.5px]">{{ items.length }} 项</span>
              </div>
              <div class="config-rows">
                <div v-for="config in items" :key="config.key" class="config-row">
                  <div class="config-row-label">
                    <p class="text-sm font-semibold">{{ config.label || config.key }}</p>
                    <p v-if="config.description" class="quiet mt-0.5 text-[12px]">{{ config.description }}</p>
                  </div>
                  <div class="config-row-control">
                    <label v-if="config.value_type === 'bool'" class="config-switch">
                      <input
                        type="checkbox"
                        :checked="config.value === 'true' || config.value === '1'"
                        :disabled="!canManageSystem"
                        @change="toggleBool(config)"
                      />
                      <span>{{ config.value === 'true' || config.value === '1' ? '已启用' : '已停用' }}</span>
                    </label>
                    <template v-else>
                      <input
                        v-model="config.value"
                        class="input"
                        :type="config.value_type === 'int' ? 'number' : 'text'"
                        :disabled="!canManageSystem"
                        @keyup.enter="saveConfig(config)"
                      />
                      <button v-if="canManageSystem" class="btn btn-secondary btn-sm" :disabled="busy" @click="saveConfig(config)">
                        保存
                      </button>
                    </template>
                  </div>
                </div>
              </div>
            </section>
          </SettingsSection>
          <AgentPanel />
        </template>

        <!-- 提醒事件 -->
        <template v-else-if="tab === 'remind'">
          <SettingsSection
            title="提醒事件"
            description="选择哪些业务事件要提醒，以及用哪个渠道发给谁。停用的事件不会产生任何通知。"
            resource="notification_templates"
          >
            <div v-if="!templates.length" class="card py-12 text-center text-sm text-[var(--text-quiet)]">
              还没有提醒事件。
            </div>

            <section v-for="[category, items] in templateGroups" :key="category" class="config-group">
              <div class="config-group-head">
                <h3>{{ categoryLabels[category] || category }}</h3>
                <span class="quiet text-[11.5px]">{{ items.length }} 个事件</span>
              </div>
              <div class="config-rows">
                <div v-for="item in items" :key="item.id" class="config-row">
                  <div class="config-row-label">
                    <p class="text-sm font-semibold">{{ item.name }}</p>
                    <p class="quiet mt-0.5 text-[12px]">
                      {{ item.title_template || item.content_template || '未设置文案' }}
                    </p>
                    <p v-if="item.variables" class="mono quiet mt-0.5 text-[11px]">变量：{{ item.variables }}</p>
                    <p class="quiet mt-0.5 text-[11px]">提醒收件人：{{ recipientsLabel(item.recipients) }}</p>
                  </div>
                  <div class="config-row-control">
                    <label class="config-switch">
                      <input type="checkbox" :checked="item.in_app" :disabled="!canManageTemplates || busy" @change="toggleChannel(item, 'in_app')" />
                      <span>站内</span>
                    </label>
                    <label class="config-switch">
                      <input type="checkbox" :checked="item.mail" :disabled="!canManageTemplates || busy" @change="toggleChannel(item, 'mail')" />
                      <span>邮件</span>
                    </label>
                    <label class="config-switch">
                      <input type="checkbox" :checked="item.is_enabled" :disabled="!canManageTemplates || busy" @change="toggleTemplate(item)" />
                      <span>{{ item.is_enabled ? '已启用' : '已停用' }}</span>
                    </label>
                    <button v-if="canManageTemplates" class="btn btn-quiet btn-sm" @click="editTemplate(item)">编辑</button>
                  </div>
                </div>
              </div>
            </section>

            <div v-if="canManageTemplates" class="card space-y-3">
              <p class="eyebrow">{{ templateDraft.id ? '编辑提醒事件' : '新增提醒事件' }}</p>
              <div class="grid gap-3 md:grid-cols-3">
                <input v-model="templateDraft.key" class="input mono text-xs" placeholder="标识，如 order.paid" />
                <input v-model="templateDraft.name" class="input" placeholder="名称" />
                <input v-model="templateDraft.event" class="input mono text-xs" placeholder="事件，如 order_paid" />
                <select v-model="templateDraft.category" class="input">
                  <option value="order">订单</option>
                  <option value="ticket">工单</option>
                  <option value="activity">活动</option>
                  <option value="coupon">优惠券</option>
                  <option value="system">系统</option>
                </select>
                <input v-model="templateDraft.variables" class="input mono text-xs" placeholder="可用变量，逗号分隔" />
                <input v-model="templateDraft.recipients" class="input mono text-xs" placeholder="收件人：staff 或邮箱，逗号分隔" />
                <input v-model.number="templateDraft.sort_order" class="input nums" type="number" placeholder="排序" />
              </div>
              <input v-model="templateDraft.title_template" class="input" placeholder="标题模板" />
              <textarea v-model="templateDraft.content_template" class="input min-h-[90px]" placeholder="内容模板" />
              <div class="flex flex-wrap items-center gap-4">
                <label class="config-switch"><input v-model="templateDraft.is_enabled" type="checkbox" /><span>启用</span></label>
                <label class="config-switch"><input v-model="templateDraft.in_app" type="checkbox" /><span>站内</span></label>
                <label class="config-switch"><input v-model="templateDraft.mail" type="checkbox" /><span>邮件</span></label>
                <input v-model.number="templateDraft.retry_limit" class="input nums !w-28" type="number" placeholder="重试" />
                <button class="btn btn-primary btn-sm ml-auto" :disabled="busy" @click="saveTemplateDraft">保存</button>
              </div>
            </div>
          </SettingsSection>

          <SettingsSection
            title="发送记录"
            description="每一次提醒的投递结果，排查「没收到通知」时看这里。"
            resource="notification_templates"
          >
            <div class="flex flex-wrap items-center gap-2">
              <select v-model="logStatus" class="input !w-auto ml-auto" @change="load">
                <option value="">全部结果</option>
                <option value="sent">已发送</option>
                <option value="failed">发送失败</option>
              </select>
              <span class="quiet text-xs">共 {{ logTotal }} 条</span>
            </div>
            <div class="table-container">
              <table class="table">
                <thead>
                  <tr><th>事件</th><th>渠道</th><th>收件人</th><th>标题</th><th>状态</th><th>时间</th></tr>
                </thead>
                <tbody>
                  <tr v-if="!logs.length">
                    <td colspan="6" class="py-10 text-center text-[var(--text-quiet)]">还没有发送记录</td>
                  </tr>
                  <tr v-for="row in logs" :key="row.id">
                    <td class="mono text-xs">{{ row.template_key }}</td>
                    <td class="text-xs">{{ row.channel === 'mail' ? '邮件' : '站内' }}</td>
                    <td class="mono text-xs">{{ row.user_id ? '#' + row.user_id : '—' }}</td>
                    <td class="quiet max-w-[220px] truncate text-xs">{{ row.title }}</td>
                    <td>
                      <span :class="row.status === 'sent' ? 'badge-success' : 'badge-danger'">{{ row.status === 'sent' ? '已发送' : '失败' }}</span>
                      <p v-if="row.error" class="quiet mt-0.5 max-w-[200px] truncate text-[11px]">{{ row.error }}</p>
                    </td>
                    <td class="quiet text-xs">{{ when(row.created_at) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </SettingsSection>
        </template>

        <!-- 风控与保留 -->
        <SettingsSection
          v-else
          :title="tabTitle"
          description="限频、二次确认与各类记录保留多久。改动会写入操作审计。"
          resource="config_center"
        >
          <p v-if="!grouped.length" class="card py-12 text-center text-sm text-[var(--text-quiet)]">
            这个分组暂无配置项。
          </p>
          <section v-for="[group, items] in grouped" :key="group" class="config-group">
            <div class="config-group-head">
              <h3>{{ groupLabels[group] || group }}</h3>
              <span class="quiet text-[11.5px]">{{ items.length }} 项</span>
            </div>
            <div class="config-rows">
              <div v-for="config in items" :key="config.key" class="config-row">
                <div class="config-row-label">
                  <p class="text-sm font-semibold">{{ config.label || config.key }}</p>
                  <p v-if="config.description" class="quiet mt-0.5 text-[12px]">{{ config.description }}</p>
                </div>
                <div class="config-row-control">
                  <label v-if="config.value_type === 'bool'" class="config-switch">
                    <input type="checkbox" :checked="config.value === 'true' || config.value === '1'" :disabled="!canManageSystem" @change="toggleBool(config)" />
                    <span>{{ config.value === 'true' || config.value === '1' ? '已启用' : '已停用' }}</span>
                  </label>
                  <template v-else>
                    <input v-model="config.value" class="input" :type="config.value_type === 'int' ? 'number' : 'text'" :disabled="!canManageSystem" @keyup.enter="saveConfig(config)" />
                    <button v-if="canManageSystem" class="btn btn-secondary btn-sm" :disabled="busy" @click="saveConfig(config)">保存</button>
                  </template>
                </div>
              </div>
            </div>
          </section>
        </SettingsSection>
      </div>
    </div>
  </section>
</template>

<style scoped>
.config-nav-item {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px 8px 12px;
  border-radius: var(--radius-sm);
  color: var(--text-dim);
  transition: background var(--fast), color var(--fast);
}
.config-nav-item::before {
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
.config-nav-item:hover { background: var(--surface-hi); color: var(--text); }
.config-nav-active { background: var(--accent-soft); color: var(--accent); font-weight: 650; }
.config-nav-active::before { height: 20px; }

.config-group { display: flex; flex-direction: column; gap: 10px; }
.config-group + .config-group { margin-top: 6px; }
.config-group-head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 10px;
  border-bottom: 1px solid var(--stroke-quiet);
  padding-bottom: 7px;
}
.config-group-head h3 { font-size: 13.5px; font-weight: 650; }
.config-rows { display: flex; flex-direction: column; gap: 8px; }
.config-row {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px 16px;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-sm);
  background: var(--surface-sunken);
  padding: 10px 12px;
  transition: border-color var(--fast);
}
.config-row:hover { border-color: var(--stroke); }
.config-row-label { min-width: 200px; flex: 1; }
.config-row-control { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-left: auto; }
.config-row-control .input { max-width: 220px; }
.config-switch { display: flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; }
.config-switch span { color: var(--text-dim); }
@media (max-width: 640px) {
  .config-row-control { margin-left: 0; width: 100%; }
  .config-row-control .input { max-width: none; flex: 1; }
}
</style>
