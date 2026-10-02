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
import AIToolPanel from './settings/AIToolPanel.vue'
import KnowledgePanel from './settings/KnowledgePanel.vue'
import AgentPanel from './settings/AgentPanel.vue'

// 配置中心是后台所有「怎么运作」设置的家：AI、工具、知识库、客服人员、
// 工单、通知模板、风控与数据保留都从这里进出。左侧是分组，右侧是当前分组，
// 地址栏带 ?tab= 所以每个分组都可以直接分享或收藏。
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

type TabKey =
  | 'ai' | 'tools' | 'knowledge' | 'service'
  | 'ticket' | 'notify' | 'risk'
  | 'order' | 'product' | 'activity' | 'site'
  | 'retention' | 'upload'

interface TabDef {
  key: TabKey
  label: string
  hint: string
  resources: string[]
}

const TABS: TabDef[] = [
  { key: 'ai', label: 'AI 基础与工作流', hint: '模型、提示词、转人工策略', resources: ['ai'] },
  { key: 'tools', label: 'AI 工具与权限', hint: '受控工具与角色授权', resources: ['ai_tools'] },
  { key: 'knowledge', label: '知识库', hint: 'AI 参考的回答依据', resources: ['knowledge'] },
  { key: 'service', label: '客服与快捷回复', hint: '坐席、分配与回复模板', resources: ['agents'] },
  { key: 'ticket', label: '工单配置', hint: '编号、超时与关闭策略', resources: ['config_center'] },
  { key: 'notify', label: '通知模板', hint: '站内与邮件的文案与渠道', resources: ['notification_templates'] },
  { key: 'order', label: '订单配置', hint: '待支付保留与自动重试', resources: ['config_center'] },
  { key: 'product', label: '商品配置', hint: '库存预警与销量展示', resources: ['config_center'] },
  { key: 'activity', label: '活动配置', hint: '默认限次与叠加策略', resources: ['config_center'] },
  { key: 'site', label: '站点展示', hint: '公告位置与页脚版本号', resources: ['config_center'] },
  { key: 'risk', label: '风控配置', hint: '限频与二次确认', resources: ['config_center'] },
  { key: 'retention', label: '数据保留', hint: '日志与对话保留天数', resources: ['config_center'] },
  { key: 'upload', label: '文件上传', hint: '图片与附件上限', resources: ['config_center'] },
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
const groupFilter = ref('')

const templateDraft = ref<Partial<NotificationTemplate>>({
  key: '', name: '', event: '', category: 'ticket', is_enabled: true, in_app: true,
  mail: false, title_template: '', content_template: '', retry_limit: 3, sort_order: 0,
})

const groupLabels: Record<string, string> = {
  ticket: '工单配置',
  risk: '风控配置',
  retention: '数据保留策略',
  upload: '文件上传配置',
  order: '订单配置',
  product: '商品配置',
  activity: '活动配置',
  site: '站点展示',
}

const canManageTemplates = computed(() => auth.allows('notification_templates', 'manage'))
const canManageSystem = computed(() => auth.allows('config_center', 'manage'))

const grouped = computed(() => {
  const map = new Map<string, SystemConfig[]>()
  for (const config of configs.value) {
    const list = map.get(config.group) ?? []
    list.push(config)
    map.set(config.group, list)
  }
  return Array.from(map.entries()).filter(([group]) => {
    if (groupFilter.value) return group === groupFilter.value
    // 选项卡与配置分组一一对应，避免一个页面里堆所有配置。
    // 除 AI/知识库/客服这些专用面板外，其余分组都直接映射同名配置组。
    if (['ticket', 'risk', 'retention', 'upload', 'order', 'product', 'activity', 'site'].includes(tab.value)) {
      return group === tab.value
    }
    return false
  })
})

// 旧地址（/knowledge、/service/agents）仍然可用：路径本身就能决定落在哪个分组，
// 已发出链接的管理员不会点进一个空页面。
const PATH_TABS: Record<string, TabKey> = {
  '/knowledge': 'knowledge',
  '/service/agents': 'service',
}

function normaliseTab(value: unknown): TabKey {
  const found = TABS.find((item) => item.key === value)
  if (found) return found.key
  const byPath = PATH_TABS[route.path]
  return byPath ?? 'ai'
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
        <AIToolPanel v-else-if="tab === 'tools'" />
        <KnowledgePanel v-else-if="tab === 'knowledge'" />
        <AgentPanel v-else-if="tab === 'service'" />

        <SettingsSection
          v-else-if="tab === 'notify'"
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
          v-else
          :title="groupLabels[tab] || '系统配置'"
          description="这些参数直接决定店铺的运行规则，保存后会写入审计日志。"
          resource="config_center"
        >
          <div class="flex flex-wrap items-center gap-2">
            <select v-model="groupFilter" class="input !w-auto">
              <option value="">全部分组</option>
              <option v-for="[group] in grouped" :key="group" :value="group">{{ groupLabels[group] || group }}</option>
            </select>
          </div>
          <p v-if="!grouped.length" class="card py-12 text-center text-sm text-[var(--text-quiet)]">
            这个分组暂无配置项。
          </p>
          <div v-for="[group, items] in grouped" :key="group" class="card space-y-3">
            <div v-for="config in items" :key="config.key" class="flex flex-wrap items-center gap-3">
              <div class="min-w-[200px] flex-1">
                <p class="text-sm font-semibold">{{ config.label || config.key }}</p>
                <p class="quiet mono text-[11px]">{{ config.key }}</p>
              </div>
              <label v-if="config.value_type === 'bool'" class="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  :checked="config.value === 'true' || config.value === '1'"
                  :disabled="!canManageSystem"
                  @change="toggleBool(config)"
                />
                {{ config.value === 'true' || config.value === '1' ? '启用' : '停用' }}
              </label>
              <input
                v-else
                v-model="config.value"
                class="input max-w-[280px]"
                :type="config.value_type === 'int' ? 'number' : 'text'"
                :disabled="!canManageSystem"
              />
              <button
                v-if="canManageSystem && config.value_type !== 'bool'"
                class="btn btn-secondary btn-sm"
                :disabled="busy"
                @click="saveConfig(config)"
              >
                保存
              </button>
            </div>
          </div>
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
</style>
