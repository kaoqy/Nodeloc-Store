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
import AIConfigPanel from './settings/AIConfigPanel.vue'
import KnowledgePanel from './settings/KnowledgePanel.vue'
import AgentPanel from './settings/AgentPanel.vue'

// 配置中心是后台所有「怎么运作」设置的家：AI、工具、知识库、客服人员、
// 工单、通知模板、风控与数据保留都从这里进出。左侧是分组，右侧是当前分组，
// 地址栏带 ?tab= 所以每个分组都可以直接分享或收藏。
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

// 五个分组按「店家要做的决定」划分，而不是按数据表划分。
// 之前 12 个标签页里有 8 个各自只有 1–3 行配置，店家为了改一个数字要在
// 侧栏里翻半天；现在同类决策放在一起。
type TabKey = 'ai' | 'knowledge' | 'support' | 'shop' | 'notify'

interface TabDef {
  key: TabKey
  label: string
  hint: string
  resources: string[]
}

const TABS: TabDef[] = [
  { key: 'ai', label: 'AI 客服', hint: '模型、提示词与转人工策略', resources: ['ai'] },
  { key: 'knowledge', label: '知识库', hint: 'AI 回答买家问题的依据', resources: ['knowledge'] },
  { key: 'support', label: '工单与客服', hint: '工单规则、坐席与快捷回复', resources: ['config_center', 'agents'] },
  { key: 'shop', label: '经营规则', hint: '订单、商品、活动与站点', resources: ['config_center'] },
  { key: 'notify', label: '通知与风控', hint: '通知模板、限频与数据保留', resources: ['notification_templates', 'config_center'] },
]

const visibleTabs = computed(() =>
  TABS.filter((item) => item.resources.some((resource) => auth.allows(resource, 'view'))),
)

const tab = ref<TabKey>('ai')
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
  mail: false, title_template: '', content_template: '', retry_limit: 3, sort_order: 0,
})

const groupLabels: Record<string, string> = {
  ticket: '工单规则',
  order: '订单规则',
  product: '商品规则',
  activity: '营销规则',
  site: '站点展示',
  risk: '风控与限频',
  retention: '数据保留',
  upload: '文件上传',
}

// 一个标签页可以包含多个配置分组：同类决策放在同一屏，店家改完一处
// 不必再去别处找第二处。
const TAB_GROUPS: Record<TabKey, string[]> = {
  ai: [],
  knowledge: [],
  // 工单规则和客服坐席是同一件事的两面：先定规则，再定谁来处理。
  support: ['ticket'],
  shop: ['order', 'product', 'activity', 'site'],
  notify: ['risk', 'retention', 'upload'],
}

const currentTab = computed(() => TABS.find((item) => item.key === tab.value))
const tabTitle = computed(() => currentTab.value?.label ?? '系统配置')

const canManageTemplates = computed(() => auth.allows('notification_templates', 'manage'))
const canManageSystem = computed(() => auth.allows('config_center', 'manage'))

/** 当前标签页要显示的配置分组，按 TAB_GROUPS 里声明的顺序排列。 */
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

// 旧地址（/knowledge、/service/agents）仍然可用：路径本身就能决定落在哪个分组，
// 已发出链接的管理员不会点进一个空页面。
// 旧地址继续可用：路径本身决定落在哪个分组。
// 已发出或收藏的链接不会变成空页面，只是落到合并后的新分组里。
const PATH_TABS: Record<string, TabKey> = {
  '/knowledge': 'knowledge',
  '/service/agents': 'support',
}

// 旧 ?tab= 参数同样收敛到新分组。
const LEGACY_TABS: Record<string, TabKey> = {
  service: 'support',
  ticket: 'support',
  order: 'shop',
  product: 'shop',
  activity: 'shop',
  site: 'shop',
  risk: 'notify',
  retention: 'notify',
  upload: 'notify',
}

function normaliseTab(value: unknown): TabKey {
  const key = typeof value === 'string' ? value : ''
  const found = TABS.find((item) => item.key === key)
  if (found) return found.key
  const legacy = LEGACY_TABS[key]
  if (legacy) return legacy
  return PATH_TABS[route.path] ?? 'ai'
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
    if (tab.value === 'notify') {
      const page = await listNotificationLogs({ status: logStatus.value || undefined, limit: 50 }).catch(() => ({ data: [], total: 0 }))
      logs.value = page.data
      logTotal.value = page.total
    }
  } catch (err) {
    error.value = errorMessage(err, '加载配置中心失败')
  } finally {
    loading.value = false
  }
}

async function switchTab(next: TabKey) {
  tab.value = next
  await router.replace({ path: '/config', query: { tab: next } })
  if (next === 'notify') await load()
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
    notice.value = '通知模板已保存。'
    templateDraft.value = {
      key: '', name: '', event: '', category: 'ticket', is_enabled: true, in_app: true,
      mail: false, title_template: '', content_template: '', retry_limit: 3, sort_order: 0,
    }
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存模板失败')
  } finally {
    busy.value = false
  }
}

function editTemplate(template: NotificationTemplate) {
  templateDraft.value = { ...template }
}

async function removeTemplate(template: NotificationTemplate) {
  if (!template.id || !window.confirm('删除模板「' + template.name + '」？')) return
  try {
    await deleteTemplate(template.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除模板失败')
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
      <h1 class="text-lg font-bold">配置中心</h1>
      <p class="quiet mt-1 text-xs">
        店铺运转方式都在这里：AI 接待策略、知识库、客服坐席、工单规则、通知与风控。
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

      <div class="min-w-0">
        <div v-if="loading" class="card space-y-3">
          <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
        </div>

        <AIConfigPanel v-else-if="tab === 'ai'" />
        <KnowledgePanel v-else-if="tab === 'knowledge'" />

        <!-- 工单与客服：上半屏是「规则」，下半屏是「谁来处理」，
             两件事本来就该一起看，所以放在同一页。 -->
        <div v-else-if="tab === 'support'" class="space-y-4">
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
        </div>

        <div v-else-if="tab === 'notify'" class="space-y-4">
        <SettingsSection
          title="通知模板"
          description="每一种通知的标题、正文与渠道都可以改；发送结果记在下方日志里。"
          resource="notification_templates"
        >
          <div class="table-container">
            <table class="table">
              <thead>
                <tr><th>模板</th><th>事件</th><th>渠道</th><th>标题模板</th><th>状态</th><th class="text-right">操作</th></tr>
              </thead>
              <tbody>
                <tr v-if="!templates.length">
                  <td colspan="6" class="py-10 text-center text-[var(--text-quiet)]">还没有通知模板</td>
                </tr>
                <tr v-for="item in templates" :key="item.id">
                  <td>
                    <p class="font-semibold">{{ item.name }}</p>
                    <p class="mono quiet text-[11px]">{{ item.key }}</p>
                  </td>
                  <td class="mono text-xs">{{ item.event }}</td>
                  <td class="text-xs">
                    <span v-if="item.in_app" class="badge-info mr-1">站内</span>
                    <span v-if="item.mail" class="badge-warning">邮件</span>
                  </td>
                  <td class="quiet max-w-[240px] truncate text-xs">{{ item.title_template }}</td>
                  <td><span :class="item.is_enabled ? 'badge-success' : 'badge'">{{ item.is_enabled ? '启用' : '停用' }}</span></td>
                  <td class="text-right">
                    <div class="flex justify-end gap-1.5">
                      <button v-if="canManageTemplates" class="btn btn-quiet btn-sm" @click="editTemplate(item)">编辑</button>
                      <button v-if="canManageTemplates" class="btn btn-danger btn-sm" @click="removeTemplate(item)">删除</button>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>

          <div v-if="canManageTemplates" class="card space-y-3">
            <p class="eyebrow">{{ templateDraft.id ? '编辑模板' : '新增模板' }}</p>
            <div class="grid gap-3 md:grid-cols-3">
              <input v-model="templateDraft.key" class="input mono text-xs" placeholder="模板标识，如 ticket.created" />
              <input v-model="templateDraft.name" class="input" placeholder="模板名称" />
              <input v-model="templateDraft.event" class="input mono text-xs" placeholder="事件，如 ticket_created" />
              <select v-model="templateDraft.category" class="input">
                <option value="ticket">工单</option>
                <option value="activity">活动</option>
                <option value="coupon">优惠券</option>
                <option value="order">订单</option>
                <option value="system">系统</option>
              </select>
              <input v-model.number="templateDraft.sort_order" class="input nums" type="number" placeholder="排序" />
              <input v-model="templateDraft.variables" class="input mono text-xs" placeholder="可用变量，逗号分隔" />
            </div>
            <input v-model="templateDraft.title_template" class="input" placeholder="标题模板，例如 工单 {{ '{{' }}ticket_no{{ '}}' }} 已创建" />
            <textarea v-model="templateDraft.content_template" class="input min-h-[100px]" placeholder="内容模板" />
            <div class="flex flex-wrap items-center gap-4">
              <label class="flex items-center gap-2 text-sm"><input v-model="templateDraft.is_enabled" type="checkbox" />启用</label>
              <label class="flex items-center gap-2 text-sm"><input v-model="templateDraft.in_app" type="checkbox" />站内通知</label>
              <label class="flex items-center gap-2 text-sm"><input v-model="templateDraft.mail" type="checkbox" />邮件通知</label>
              <input v-model.number="templateDraft.retry_limit" class="input nums !w-32" type="number" placeholder="重试次数" />
              <button class="btn btn-primary btn-sm ml-auto" :disabled="busy" @click="saveTemplateDraft">保存模板</button>
            </div>
          </div>

          <div class="card space-y-3">
            <div class="flex flex-wrap items-center gap-2">
              <p class="text-sm font-semibold">发送日志</p>
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
                  <tr><th>模板</th><th>渠道</th><th>收件人</th><th>标题</th><th>状态</th><th>时间</th></tr>
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
          </div>
        </SettingsSection>

        <SettingsSection
          title="风控与数据"
          description="限频、二次确认与各类记录保留多久。"
          resource="config_center"
        >
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
        </div>

        <SettingsSection
          v-else
          :title="tabTitle"
          description="这些参数直接决定店铺怎么运转。改动会写入操作日志，出问题可以追溯到人和时间。"
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
                    <button
                      v-if="canManageSystem"
                      class="btn btn-secondary btn-sm"
                      :disabled="busy"
                      @click="saveConfig(config)"
                    >
                      保存
                    </button>
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
/* 配置中心左侧分组：和侧栏保持同一套视觉，但作用域留在这里，
   这样后台其它页面不会被这份样式影响。 */
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
.config-nav-item:hover {
  background: var(--surface-hi);
  color: var(--text);
}
.config-nav-active {
  background: var(--accent-soft);
  color: var(--accent);
  font-weight: 650;
}
.config-nav-active::before { height: 20px; }

/* ── 配置分组 ─────────────────────────────────────────────────────
   一个分组一张卡：组名在卡头上，逐行列出「名称 + 说明 + 控件」。
   以前每项之间没有边界，十几个输入框连成一片，改哪一项全凭眼力。 */
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
.config-row-control { display: flex; align-items: center; gap: 8px; margin-left: auto; }
.config-row-control .input { max-width: 220px; }
.config-switch { display: flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; }
.config-switch span { color: var(--text-dim); }
@media (max-width: 640px) {
  .config-row-control { margin-left: 0; width: 100%; }
  .config-row-control .input { max-width: none; flex: 1; }
}
</style>
