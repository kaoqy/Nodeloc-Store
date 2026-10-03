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

type TabKey = 'support' | 'commerce' | 'activity' | 'site' | 'remind' | 'safety'

interface TabDef {
  key: TabKey
  label: string
  hint: string
  resources: string[]
}

const TABS: TabDef[] = [
  { key: 'support', label: '工单与客服', hint: '编号、时效与买家操作', resources: ['config_center', 'agents'] },
  { key: 'commerce', label: '订单与商品', hint: '关单、交付与库存预警', resources: ['config_center'] },
  { key: 'activity', label: '营销规则', hint: '参与限制与优惠叠加', resources: ['config_center', 'activities'] },
  { key: 'site', label: '站点展示', hint: '前台露出的品牌信息', resources: ['config_center'] },
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

/**
 * 每一项配置的专属呈现方式。
 *
 * 配置中心不再把所有项渲染成同一种输入框：带单位的值拆出单位徽标，
 * 开关给出开启/关闭后的实际含义，编号前缀直接预览生成结果。
 */
interface FieldMeta {
  unit?: string
  hint?: string | ((config: SystemConfig) => string)
  preview?: (config: SystemConfig) => string
  onLabel?: string
  offLabel?: string
}

const FIELD_META: Record<string, FieldMeta> = {
  'ticket/ticket_no_prefix': {
    hint: '买家在工单列表看到的编号前缀。',
    preview: (c) => `${(c.value || 'TK').trim() || 'TK'}0001`,
  },
  'ticket/human_timeout_hours': {
    unit: '小时',
    hint: (c) => `转人工后超过 ${c.value || '—'} 小时仍未处理，工单会标记为「即将超时」。`,
  },
  'ticket/allow_reopen': {
    onLabel: '允许重开',
    offLabel: '不允许重开',
    hint: '开启后，已解决或已关闭的工单买家可以重新打开继续追问。',
  },
  'ticket/allow_cancel': {
    onLabel: '允许撤销',
    offLabel: '不允许撤销',
    hint: '开启后，买家可以自己撤销还没解决的工单。',
  },
  'ticket/enable_rating': {
    onLabel: '邀请评价',
    offLabel: '不邀请评价',
    hint: '开启后工单结束会邀请买家打分，分数进入满意度统计。',
  },
  'order/unpaid_cancel_hours': {
    unit: '小时',
    hint: (c) => `下单后 ${c.value || '—'} 小时内未支付，订单自动关闭并释放库存。`,
  },
  'order/auto_deliver_retry': {
    onLabel: '自动重试',
    offLabel: '不自动重试',
    hint: '自动发货失败时后台定期重试，直到成功或需要人工介入。',
  },
  'product/default_stock_alert': {
    unit: '张',
    hint: (c) => `卡密剩余低于 ${c.value || '—'} 张时，总览页列入库存预警。`,
  },
  'product/show_sold_count': {
    onLabel: '显示销量',
    offLabel: '隐藏销量',
    hint: '在前台商品卡片上显示已售数量。',
  },
  'activity/default_per_user_limit': {
    unit: '次/人',
    hint: (c) => (c.value === '0' ? '当前不限次，任何买家都可以反复参与。' : `每位买家最多参与 ${c.value || '—'} 次；填 0 表示不限次。`),
  },
  'activity/block_activity_stacking': {
    onLabel: '禁止叠加',
    offLabel: '允许叠加',
    hint: '一条订单只应用优惠最大的一项活动，避免折上折算出异常低价。',
  },
  'site/footer_show_version': {
    onLabel: '显示版本号',
    offLabel: '隐藏版本号',
    hint: '在店铺页脚展示当前版本，方便核对线上是否是最新构建。',
  },
  'risk/coupon_max_attempts': {
    unit: '次/分钟',
    hint: (c) => `同一来源每分钟最多尝试 ${c.value || '—'} 次优惠码，超出会被限流。`,
  },
  'risk/require_second_confirm': {
    onLabel: '需要二次确认',
    offLabel: '无需二次确认',
    hint: '退款、发券这类写操作需要买家本人再确认一次。',
  },
  'retention/audit_log_days': {
    unit: '天',
    hint: (c) => `后台操作记录保留 ${c.value || '—'} 天，超期可由清理任务删除。`,
  },
  'retention/ticket_days': {
    unit: '天',
    hint: (c) => `已关闭工单保留 ${c.value || '—'} 天，便于日后复查。`,
  },
  'upload/max_image_mb': {
    unit: 'MB',
    hint: (c) => `商品封面与客服头像单张图片不超过 ${c.value || '—'} MB。`,
  },
}

/** 每个标签页由哪些分组组成，以及分组内的标题与说明。 */
interface GroupBlock {
  group: string
  title: string
  intro: string
  parts: { title: string; keys: string[] }[]
}

const TAB_SECTIONS: Record<TabKey, GroupBlock[]> = {
  support: [
    {
      group: 'ticket', title: '工单规则',
      intro: '工单号怎么生成、多久算超时，以及买家能对工单做哪些操作。',
      parts: [
        { title: '编号方式', keys: ['ticket_no_prefix'] },
        { title: '响应时效', keys: ['human_timeout_hours'] },
        { title: '买家操作', keys: ['allow_reopen', 'allow_cancel'] },
        { title: '满意度评价', keys: ['enable_rating'] },
      ],
    },
  ],
  commerce: [
    {
      group: 'order', title: '订单与交付',
      intro: '未支付订单的保留时长，以及自动发货失败后的处理方式。',
      parts: [
        { title: '未支付订单', keys: ['unpaid_cancel_hours'] },
        { title: '自动交付', keys: ['auto_deliver_retry'] },
      ],
    },
    {
      group: 'product', title: '商品规则',
      intro: '库存预警阈值与前台展示方式。',
      parts: [
        { title: '库存预警', keys: ['default_stock_alert'] },
        { title: '前台展示', keys: ['show_sold_count'] },
      ],
    },
  ],
  activity: [
    {
      group: 'activity', title: '营销规则',
      intro: '新建活动时的默认值，以及一张订单能叠加多少优惠。',
      parts: [
        { title: '参与限制', keys: ['default_per_user_limit'] },
        { title: '优惠叠加', keys: ['block_activity_stacking'] },
      ],
    },
  ],
  site: [
    {
      group: 'site', title: '站点展示',
      intro: '店铺前台公开露出的信息。',
      parts: [
        { title: '页脚', keys: ['footer_show_version'] },
      ],
    },
  ],
  remind: [],
  safety: [
    {
      group: 'risk', title: '风控与限频',
      intro: '防止优惠码被暴力猜测，以及高风险写操作的确认策略。',
      parts: [
        { title: '优惠码防刷', keys: ['coupon_max_attempts'] },
        { title: '高风险操作', keys: ['require_second_confirm'] },
      ],
    },
    {
      group: 'retention', title: '数据保留',
      intro: '各类记录保留多久，超期可由清理任务删除。',
      parts: [
        { title: '保留期限', keys: ['audit_log_days', 'ticket_days'] },
      ],
    },
    {
      group: 'upload', title: '文件上传',
      intro: '图片上传的体积上限。',
      parts: [
        { title: '图片限制', keys: ['max_image_mb'] },
      ],
    },
  ],
}

function metaOf(config: SystemConfig): FieldMeta {
  return FIELD_META[`${config.group}/${config.key}`] ?? {}
}

function hintOf(config: SystemConfig): string {
  const meta = metaOf(config)
  if (typeof meta.hint === 'function') return meta.hint(config)
  return meta.hint || config.description || ''
}

function previewOf(config: SystemConfig): string {
  return metaOf(config).preview?.(config) ?? ''
}

function unitOf(config: SystemConfig): string {
  return metaOf(config).unit ?? ''
}

function onLabelOf(config: SystemConfig): string {
  return metaOf(config).onLabel ?? '已启用'
}

function offLabelOf(config: SystemConfig): string {
  return metaOf(config).offLabel ?? '已停用'
}

function isOn(config: SystemConfig): boolean {
  return config.value === 'true' || config.value === '1'
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

/**
 * 当前标签页的分组，按配置清单解析成可直接渲染的结构。
 * 库里没有的配置项会被跳过，不会留下空壳分组。
 */
const currentBlocks = computed(() => {
  const blocks = TAB_SECTIONS[tab.value] ?? []
  return blocks
    .map((block) => ({
      group: block.group,
      title: block.title,
      intro: block.intro,
      parts: block.parts
        .map((part) => ({
          title: part.title,
          items: part.keys
            .map((key) => configs.value.find((item) => item.group === block.group && item.key === key))
            .filter((item): item is SystemConfig => Boolean(item)),
        }))
        .filter((part) => part.items.length > 0),
    }))
    .filter((block) => block.parts.length > 0)
})

// 旧地址继续可用，落到合并后的新分组。
const LEGACY_TABS: Record<string, TabKey> = {
  service: 'support',
  ticket: 'support',
  ai: 'support',
  knowledge: 'support',
  order: 'commerce',
  product: 'commerce',
  activity: 'activity',
  site: 'site',
  notify: 'remind',
  remind: 'remind',
  risk: 'safety',
  retention: 'safety',
  upload: 'safety',
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

        <!-- 规则类配置：工单 / 订单商品 / 营销 / 站点 / 风控保留 -->
        <!-- 每个分组单独成卡，字段按用途分区，值以「徽标 + 预览」呈现，不再是统一的表单列表。 -->
        <template v-else>
          <SettingsSection
            v-for="block in currentBlocks"
            :key="block.group"
            :title="block.title"
            :description="block.intro"
            resource="config_center"
          >
            <div class="config-parts">
              <section v-for="part in block.parts" :key="part.title" class="config-part">
                <p class="config-part-title">{{ part.title }}</p>
                <div class="config-cards">
                  <article
                    v-for="config in part.items"
                    :key="config.key"
                    class="config-card"
                    :class="config.value_type === 'bool' ? 'config-card-toggle' : ''"
                  >
                    <div class="config-card-body">
                      <p class="config-card-label">{{ config.label || config.key }}</p>
                      <p class="config-card-hint">{{ hintOf(config) }}</p>
                      <p v-if="previewOf(config)" class="config-preview">
                        <span class="quiet text-[10.5px] tracking-wide">预览</span>
                        <span class="mono">{{ previewOf(config) }}</span>
                      </p>
                    </div>
                    <div class="config-card-control">
                      <template v-if="config.value_type === 'bool'">
                        <button
                          type="button"
                          class="switch"
                          :class="isOn(config) ? 'switch-on' : ''"
                          :disabled="!canManageSystem || busy"
                          :aria-pressed="isOn(config)"
                          :aria-label="config.label || config.key"
                          @click="toggleBool(config)"
                        />
                        <span class="config-state">{{ isOn(config) ? onLabelOf(config) : offLabelOf(config) }}</span>
                      </template>
                      <template v-else>
                        <div class="config-input">
                          <input
                            v-model="config.value"
                            class="input"
                            :type="config.value_type === 'int' ? 'number' : 'text'"
                            :disabled="!canManageSystem"
                            @keyup.enter="saveConfig(config)"
                          />
                          <span v-if="unitOf(config)" class="config-unit">{{ unitOf(config) }}</span>
                        </div>
                        <button v-if="canManageSystem" class="btn btn-secondary btn-sm" :disabled="busy" @click="saveConfig(config)">
                          保存
                        </button>
                      </template>
                    </div>
                  </article>
                </div>
              </section>
            </div>
          </SettingsSection>
          <AgentPanel v-if="tab === 'support'" />
        </template>

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
/* 提醒事件列表仍在用的紧凑行。 */
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
@media (max-width: 640px) {
  .config-row-control { margin-left: 0; width: 100%; }
}

/* 提醒事件编辑器仍在用的紧凑行内开关。 */
.config-switch { display: flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; }
.config-switch span { color: var(--text-dim); }

/* ── 规则类配置的专属卡片 ──
   每一项是一张带说明与当前取值预览的卡片；开关型排成紧凑网格，
   数值型留出输入区与单位，避免所有字段长成同一个输入框。 */
.config-parts { display: flex; flex-direction: column; gap: 18px; }
.config-part-title {
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--text-quiet);
  margin-bottom: 8px;
}
.config-cards {
  display: grid;
  gap: 10px;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}
.config-card {
  display: flex;
  flex-direction: column;
  gap: 12px;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-sm);
  background: var(--surface-sunken);
  padding: 13px 14px;
  transition: border-color var(--fast), background var(--fast);
}
.config-card:hover { border-color: var(--stroke); background: var(--surface-hi); }
.config-card-body { flex: 1; min-width: 0; }
.config-card-label { font-size: 13.5px; font-weight: 650; }
.config-card-hint { margin-top: 3px; font-size: 12px; line-height: 1.6; color: var(--text-quiet); }
.config-preview {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  margin-top: 9px;
  padding: 3px 9px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke-quiet);
  background: var(--surface);
  font-size: 12px;
}
.config-card-control {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 9px;
}
.config-card-toggle .config-card-control { gap: 10px; }
.config-state { font-size: 12.5px; font-weight: 600; color: var(--text-dim); }
.config-input {
  position: relative;
  display: flex;
  align-items: center;
  flex: 1;
  min-width: 140px;
}
.config-input .input { padding-right: 58px; }
.config-unit {
  position: absolute;
  right: 10px;
  font-size: 11.5px;
  color: var(--text-quiet);
  pointer-events: none;
}
@media (max-width: 640px) {
  .config-cards { grid-template-columns: 1fr; }
  .config-card-control .btn { margin-left: auto; }
}
</style>
