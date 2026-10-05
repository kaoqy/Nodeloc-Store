<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  deleteTemplate,
  listNotificationLogs,
  listSystemConfigs,
  listTemplates,
  saveSystemConfig,
  savedConfigValue,
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

type TabKey = 'support' | 'commerce' | 'activity' | 'site' | 'remind' | 'legacy'

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
  { key: 'legacy', label: '历史兼容', hint: '旧值仅核对，不参与当前运行', resources: ['config_center'] },
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
  /** 数值型字段后面的单位徽标。 */
  unit?: string
  /** 一句话说明这项配置改了什么、影响到谁。 */
  hint?: string | ((config: SystemConfig) => string)
  /** 当前取值会产生的实际结果，能算出来的都算给店家看。 */
  preview?: (config: SystemConfig) => string
  /** 输入框里的示例值。 */
  placeholder?: string
  /** 建议取值范围；越界时在卡片上给出一条提醒。 */
  range?: { min?: number; max?: number }
  /** 开关打开 / 关闭后的实际含义。 */
  onLabel?: string
  offLabel?: string
  /** 这一项在系统里管的是什么，用一句话点明，便于对照排查。 */
  affects?: string
  /** 控件类型；默认按 value_type 与单位推断。 */
  control?: 'switch' | 'number' | 'text' | 'textarea' | 'select' | 'color' | 'password'
  /** 选择型配置的选项；后端仍保存原始字符串，不改变数据结构。 */
  options?: { value: string; label: string }[]
  /** 保存前的附加校验，返回非空字符串则阻止保存。 */
  validate?: (config: SystemConfig) => string
  /** 该配置当前是否有实际执行路径。false 表示只保留历史值，不提供假开关。 */
  effective?: boolean
}

/** 与后端 defaultSystemConfigs 对齐的出厂值，仅用于在卡片上提示「默认是多少」。 */
const DEFAULT_VALUES: Record<string, string> = {
  'ticket/ticket_no_prefix': 'TK',
  'ticket/human_timeout_hours': '24',
  'ticket/allow_reopen': 'true',
  'ticket/allow_cancel': 'true',
  'ticket/enable_rating': 'true',
  'product/show_sold_count': 'true',
  'activity/default_per_user_limit': '1',
  'activity/block_activity_stacking': 'true',
  'site/footer_show_version': 'true',
}

const FIELD_META: Record<string, FieldMeta> = {
  'ticket/ticket_no_prefix': {
    hint: '买家在工单列表看到的编号前缀，工单创建时写入编号，之后不再变化。',
    affects: '新建工单的编号',
    placeholder: 'TK',
    preview: (c) => `${(c.value || 'TK').trim() || 'TK'}0001`,
  },
  'ticket/human_timeout_hours': {
    unit: '小时',
    range: { min: 1, max: 720 },
    affects: '工单列表的超时标记',
    hint: (c) => `转人工后超过 ${c.value || '—'} 小时仍未处理，工单会标记为「即将超时」，并在客服工作台排到前面。`,
    preview: (c) => {
      const hours = Number(c.value)
      if (!Number.isFinite(hours) || hours <= 0) return ''
      return hours < 24 ? `约 ${hours} 小时后进入超时预警` : `约 ${Math.round((hours / 24) * 10) / 10} 天后进入超时预警`
    },
  },
  'ticket/allow_reopen': {
    onLabel: '允许重开',
    offLabel: '不允许重开',
    affects: '买家在「我的工单」里的可用操作',
    hint: '开启后，已解决或已关闭的工单，买家可以重新打开继续追问；关闭后买家只能新建一张工单。',
    preview: (c) => (isTruthy(c.value) ? '买家会看到「重新打开」按钮' : '买家只能新建工单'),
  },
  'ticket/allow_cancel': {
    onLabel: '允许撤销',
    offLabel: '不允许撤销',
    affects: '买家在「我的工单」里的可用操作',
    hint: '开启后，买家可以自己撤销还没解决的工单；关闭后只能由客服结单或撤销。',
    preview: (c) => (isTruthy(c.value) ? '买家会看到「撤销工单」按钮' : '撤销需要客服处理'),
  },
  'ticket/enable_rating': {
    onLabel: '邀请评价',
    offLabel: '不邀请评价',
    affects: '工单结单流程与满意度统计',
    hint: '开启后工单结束会请买家打分，分数进入总览页的满意度统计。',
    preview: (c) => (isTruthy(c.value) ? '结单后请买家打分' : '结单直接结束，不打扰买家'),
  },
  'product/show_sold_count': {
    onLabel: '显示销量',
    offLabel: '隐藏销量',
    affects: '前台商品卡片',
    hint: '在前台商品卡片上显示已售数量；关闭后只显示库存状态。',
    preview: (c) => (isTruthy(c.value) ? '卡片上出现「已售 N」' : '卡片上不出现销量'),
  },
  'activity/default_per_user_limit': {
    unit: '次/人',
    range: { min: 0, max: 9999 },
    affects: '新建活动时预填的参与上限',
    hint: '新建活动时预填的参与上限；填 0 表示不限次，任何买家都可以反复参与。',
    preview: (c) => {
      if (!c.value || c.value === '0') return '默认不限次'
      const n = Number(c.value)
      return Number.isFinite(n) ? `新建活动默认限 ${n} 次/人` : ''
    },
  },
  'activity/block_activity_stacking': {
    onLabel: '禁止叠加',
    offLabel: '允许叠加',
    affects: '结算时的优惠计算',
    hint: '开启后一条订单只应用优惠最大的一项活动，避免折上折算出异常低价；关闭则按活动各自的叠加规则合并。',
    preview: (c) => (isTruthy(c.value) ? '一单只取最优的一项活动' : '多项活动可以同时抵扣'),
  },
  'site/footer_show_version': {
    onLabel: '显示版本号',
    offLabel: '隐藏版本号',
    affects: '店铺前台页脚',
    hint: '在店铺页脚展示当前版本，方便核对线上是否是最新构建。',
    preview: (c) => (isTruthy(c.value) ? '页脚显示一行版本号' : '页脚不显示版本号'),
  },
  'order/unpaid_cancel_hours': {
    effective: false,
    unit: '小时',
    affects: '暂无当前执行路径',
    hint: '历史兼容项。后端会保留这个值，但当前版本没有自动关单任务读取它；请勿把它当成已生效的自动取消规则。',
  },
  'order/auto_deliver_retry': {
    effective: false,
    affects: '暂无当前执行路径',
    hint: '历史兼容项。交付失败重试由现有订单队列与后台扫描处理，这个开关当前不会被运行时读取。',
  },
  'product/default_stock_alert': {
    effective: false,
    unit: '件',
    affects: '请改用「系统设置 → 内容与功能 → 库存预警阈值」',
    hint: '历史兼容项。当前库存预警阈值来自系统设置中的功能配置，这个旧值不会被读取。',
  },
  'risk/coupon_max_attempts': {
    effective: false,
    unit: '次/分钟',
    affects: '暂无当前执行路径',
    hint: '历史兼容项。当前版本没有优惠码尝试限流计数器，这个数值不会被运行时读取。',
  },
  'risk/require_second_confirm': {
    effective: false,
    affects: '暂无当前执行路径',
    hint: '历史兼容项。高风险操作的确认目前由各业务页面直接处理，这个开关不会被运行时读取。',
  },
  'retention/audit_log_days': {
    effective: false,
    unit: '天',
    affects: '暂无当前执行路径',
    hint: '历史兼容项。当前版本没有审计日志自动清理任务，这个保留天数不会被读取。',
  },
  'retention/ticket_days': {
    effective: false,
    unit: '天',
    affects: '暂无当前执行路径',
    hint: '历史兼容项。当前版本没有工单自动清理任务，这个保留天数不会被读取。',
  },
  'upload/max_image_mb': {
    effective: false,
    unit: 'MB',
    affects: '当前上传上限固定为 2 MB',
    hint: '历史兼容项。图片上传上限由上传服务固定校验，这个旧值不会改变实际上限。',
  },
}

/** 后端已经保留但当前没有执行路径的配置，只在“历史兼容”区展示。 */
const HISTORICAL_KEYS = new Set([
  'order/unpaid_cancel_hours',
  'order/auto_deliver_retry',
  'product/default_stock_alert',
  'risk/coupon_max_attempts',
  'risk/require_second_confirm',
  'retention/audit_log_days',
  'retention/ticket_days',
  'upload/max_image_mb',
])

function isHistorical(config: SystemConfig): boolean {
  return HISTORICAL_KEYS.has(`${config.group}/${config.key}`)
}

function isEffective(config: SystemConfig): boolean {
  return metaOf(config).effective !== false && !isHistorical(config)
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
      group: 'product', title: '商品展示',
      intro: '控制前台商品卡片是否显示销量。',
      parts: [
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
  legacy: [],
}

function metaOf(config: SystemConfig): FieldMeta {
  return FIELD_META[`${config.group}/${config.key}`] ?? {}
}

/** 库里存的是 'true' / '1' 这类字符串，统一在这里判断。 */
function isTruthy(value?: string): boolean {
  return value === 'true' || value === '1'
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
  return isTruthy(config.value)
}

function placeholderOf(config: SystemConfig): string {
  return metaOf(config).placeholder ?? ''
}

function affectsOf(config: SystemConfig): string {
  return metaOf(config).affects ?? ''
}

function controlOf(config: SystemConfig): NonNullable<FieldMeta['control']> {
  const meta = metaOf(config)
  if (meta.control) return meta.control
  if (config.value_type === 'bool') return 'switch'
  if (config.value_type === 'int') return 'number'
  return 'text'
}

function validationMessage(config: SystemConfig): string {
  const meta = metaOf(config)
  if (meta.validate) return meta.validate(config)
  if (config.value_type !== 'int') return ''
  const value = Number(config.value)
  if (!Number.isFinite(value) || !Number.isInteger(value)) return '请填写整数'
  if (meta.range?.min !== undefined && value < meta.range.min) return `不能小于 ${meta.range.min}`
  if (meta.range?.max !== undefined && value > meta.range.max) return `不能大于 ${meta.range.max}`
  return ''
}

/**
 * 出厂的默认值。只在当前取值确实被改过时才显示，
 * 免得每张卡片都挂一个「默认」，反而看不到哪几项被人动过。
 */
function defaultNoteOf(config: SystemConfig): string {
  const shipped = DEFAULT_VALUES[`${config.group}/${config.key}`]
  if (shipped === undefined) return ''
  if ((config.value ?? '') === shipped) return ''
  const shown = config.value_type === 'bool' ? (isTruthy(shipped) ? '开启' : '关闭') : shipped
  return `默认 ${shown}`
}

/**
 * 数值越界提醒。后端仍会保存，但这类值通常意味着打错了一个数量级，
 * 与其悄悄生效，不如在卡片上点出来。
 */
function rangeWarnOf(config: SystemConfig): string {
  const meta = metaOf(config)
  if (!meta.range) return ''
  const value = Number(config.value)
  if (!Number.isFinite(value)) return ''
  const { min, max } = meta.range
  if (typeof min === 'number' && value < min) return `低于建议下限 ${min}`
  if (typeof max === 'number' && value > max) return `高于建议上限 ${max}`
  return ''
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
            .filter((item): item is SystemConfig => item !== undefined && isEffective(item)),
        }))
        .filter((part) => part.items.length > 0),
    }))
    .filter((block) => block.parts.length > 0)
})

/**
 * 旧版本留下、当前没有执行路径的配置。它们不混进正常编辑区，
 * 避免管理员看到一个能保存但不会生效的开关。
 */
const historicalConfigs = computed(() => configs.value.filter((item) => isHistorical(item)))

/** 合并过的旧地址仍然认得，直接落到现在的新分组。 */
const LEGACY_TABS: Record<string, TabKey> = {
  service: 'support',
  ticket: 'support',
  order: 'commerce',
  product: 'commerce',
  activity: 'activity',
  site: 'site',
  notify: 'remind',
  remind: 'remind',
}

/** 已经彻底下线的后台地址：老书签不再映射到任何分组。 */
const RETIRED_TABS = new Set(['ai', 'knowledge', 'chat', 'chatbot', 'assistant'])

function normaliseTab(value: unknown): TabKey {
  const key = typeof value === 'string' ? value : ''
  const found = TABS.find((item) => item.key === key)
  if (found) return found.key
  if (RETIRED_TABS.has(key)) return 'support'
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
    if (!templateList.length && !configList.length) {
      error.value = '配置暂时没有可读取的内容，请检查服务端配置后重试。'
    }
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

/**
 * 收件人口径与后端 normalizeRecipients 对齐，并在文案里写清站内 / 邮件分别发到哪，
 * 避免店主以为「订单异常」会发到买家，或者以为填了邮箱就等于加了收件人。
 */
function recipientsLabel(value?: string): string {
  const raw = (value || '').trim()
  if (!raw) return '买家本人（站内信 + 买家邮箱）'
  if (raw === 'staff') return '客服团队（员工站内信 + 员工邮箱）'
  if (raw === 'user') return '买家本人（站内信 + 买家邮箱）'
  if (raw === 'custom') return '指定邮箱（只发邮件，不产生站内信）'
  return raw
}

/** 只有「指定邮箱、纯邮件」的提醒事件才在卡片上提示去设置页确认 SMTP。 */
function mailOnlyOf(item: NotificationTemplate): boolean {
  const raw = (item.recipients || '').trim()
  if (!raw || raw === 'staff' || raw === 'user') return false
  return !item.in_app && item.mail
}

/** 事件标识（order_paid 之类）翻译成人话，店主不需要记内部事件名。 */
const EVENT_LABELS: Record<string, string> = {
  ticket_created: '买家提交工单时',
  ticket_transfer: '工单转人工时',
  agent_reply: '人工客服回复时',
  ticket_status: '工单状态变化时',
  ticket_timeout: '工单接近超时阈值时',
  activity_started: '活动开始时',
  coupon_claimed: '买家领取优惠券时',
  coupon_expiring: '优惠券即将过期时',
  order_exception: '订单支付或交付异常时',
}

function eventLabel(event?: string): string {
  const raw = (event || '').trim()
  if (!raw) return '手动触发或由后台任务触发'
  return EVENT_LABELS[raw] || `事件 ${raw}`
}

/** 事件当前的实际触发方式，让「站内 / 邮件」两个开关有对照。 */
function channelsLabel(item: NotificationTemplate): string {
  const parts: string[] = []
  if (item.in_app) parts.push('站内信')
  if (item.mail) parts.push('邮件')
  if (!parts.length) return '未开启任何渠道'
  return `当前发出：${parts.join(' + ')}`
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

async function saveConfig(config: SystemConfig): Promise<boolean> {
  const invalid = validationMessage(config)
  if (invalid) {
    error.value = '配置「' + (config.label || config.key) + '」' + invalid + '。'
    return false
  }
  busy.value = true
  error.value = ''
  try {
    const saved = await saveSystemConfig(config)
    config.value = savedConfigValue(saved, config.value)
    notice.value = '配置「' + (config.label || config.key) + '」已保存，运行时立即生效。'
    return true
  } catch (err) {
    error.value = '配置「' + (config.label || config.key) + '」保存失败：' + errorMessage(err, '请检查填写内容')
    return false
  } finally {
    busy.value = false
  }
}

function toggleBool(config: SystemConfig) {
  const previous = config.value
  config.value = previous === 'true' || previous === '1' ? 'false' : 'true'
  void saveConfig(config).then((saved) => {
    if (!saved) config.value = previous
  })
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
                <div
                  v-for="item in items"
                  :key="item.id"
                  class="config-row"
                  :class="item.is_enabled ? '' : 'config-row-off'"
                >
                  <div class="config-row-label">
                    <div class="config-row-head">
                      <p class="text-sm font-semibold">{{ item.name }}</p>
                      <span v-if="!item.is_enabled" class="config-kind">已停用</span>
                      <span v-else-if="!item.in_app && !item.mail" class="config-kind">无渠道</span>
                    </div>
                    <p class="config-row-when">
                      <span class="config-affects-tag">触发</span>
                      <span>{{ eventLabel(item.event) }}</span>
                    </p>
                    <p class="quiet mt-1.5 text-[12px] leading-relaxed">
                      {{ item.title_template || item.content_template || '未设置文案' }}
                    </p>
                    <p class="config-row-meta">{{ channelsLabel(item) }}　·　收件人：{{ recipientsLabel(item.recipients) }}</p>
                    <p v-if="item.variables" class="mono quiet mt-1 text-[11px]">可用变量：{{ item.variables }}</p>
                    <p v-if="mailOnlyOf(item)" class="config-warn">
                      只发邮件，请先在「邮件通知」里配好 SMTP 并发一封测试邮件。
                    </p>
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
                      <div class="config-card-head">
                        <p class="config-card-label">{{ config.label || config.key }}</p>
                        <span v-if="config.value_type === 'bool'" class="config-kind">开关</span>
                        <span v-else-if="controlOf(config) === 'number'" class="config-kind">数值</span>
                        <span v-else-if="controlOf(config) === 'text'" class="config-kind">文本</span>
                        <span v-else-if="unitOf(config)" class="config-kind">{{ unitOf(config) }}</span>
                        <span v-else class="config-kind">文本</span>
                        <span v-if="defaultNoteOf(config)" class="config-kind config-kind-default">{{ defaultNoteOf(config) }}</span>
                        <span :class="['config-kind', isEffective(config) ? 'config-kind-live' : 'config-kind-legacy']">
                          {{ isEffective(config) ? '立即生效' : '仅保留旧值' }}
                        </span>
                      </div>
                      <p v-if="affectsOf(config)" class="config-affects">
                        <span class="config-affects-tag">影响</span>
                        <span>{{ affectsOf(config) }}</span>
                      </p>
                      <p class="config-card-hint">{{ hintOf(config) }}</p>
                      <p v-if="previewOf(config)" class="config-preview">
                        <span class="quiet text-[10.5px] tracking-wide">预览</span>
                        <span class="mono">{{ previewOf(config) }}</span>
                      </p>
                      <p v-if="rangeWarnOf(config)" class="config-warn">
                        {{ rangeWarnOf(config) }}，请确认这是有意为之。
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
                      <template v-else-if="controlOf(config) === 'select'">
                        <div class="config-input">
                          <select
                            v-model="config.value"
                            class="input"
                            :disabled="!canManageSystem"
                            :aria-label="config.label || config.key"
                          >
                            <option v-for="option in metaOf(config).options ?? []" :key="option.value" :value="option.value">
                              {{ option.label }}
                            </option>
                          </select>
                        </div>
                        <button v-if="canManageSystem" class="btn btn-secondary btn-sm" :disabled="busy" @click="saveConfig(config)">
                          保存
                        </button>
                      </template>
                      <template v-else-if="controlOf(config) === 'textarea'">
                        <div class="config-input">
                          <textarea
                            v-model="config.value"
                            class="input min-h-[84px]"
                            :placeholder="placeholderOf(config)"
                            :disabled="!canManageSystem"
                            :aria-label="config.label || config.key"
                          />
                        </div>
                        <button v-if="canManageSystem" class="btn btn-secondary btn-sm" :disabled="busy" @click="saveConfig(config)">
                          保存
                        </button>
                      </template>
                      <template v-else>
                        <div class="config-input">
                          <input
                            v-model="config.value"
                            class="input"
                            :class="{ 'input-error': validationMessage(config) }"
                            :type="controlOf(config) === 'number' ? 'number' : controlOf(config) === 'color' ? 'color' : 'text'"
                            :placeholder="placeholderOf(config)"
                            :disabled="!canManageSystem"
                            :min="metaOf(config).range?.min"
                            :max="metaOf(config).range?.max"
                            :aria-invalid="Boolean(validationMessage(config))"
                            @keyup.enter="saveConfig(config)"
                          />
                          <span v-if="unitOf(config)" class="config-unit">{{ unitOf(config) }}</span>
                        </div>
                        <button v-if="canManageSystem" class="btn btn-secondary btn-sm" :disabled="busy" @click="saveConfig(config)">
                          保存
                        </button>
                      </template>
                      <p v-if="validationMessage(config)" class="config-card-hint text-[var(--danger)]" role="alert">
                        {{ validationMessage(config) }}
                      </p>
                    </div>
                  </article>
                </div>
              </section>
            </div>
          </SettingsSection>
          <AgentPanel v-if="tab === 'support'" />
        </template>

        <SettingsSection
          v-if="tab === 'legacy' && historicalConfigs.length"
          title="历史兼容配置"
          description="这些键是旧版本留下的数据，当前版本没有运行时读取它们。此处只用于核对和保留原值，不代表配置已经生效。"
          resource="config_center"
        >
          <div class="config-cards">
            <article v-for="config in historicalConfigs" :key="config.group + '/' + config.key" class="config-card config-card-legacy">
              <div class="config-card-body">
                <div class="config-card-head">
                  <p class="config-card-label">{{ config.label || config.key }}</p>
                  <span class="config-kind">旧配置</span>
                  <span v-if="unitOf(config)" class="config-kind">{{ unitOf(config) }}</span>
                </div>
                <p class="config-card-hint">{{ hintOf(config) }}</p>
                <p class="config-preview">
                  <span class="quiet text-[10.5px] tracking-wide">当前保留值</span>
                  <span class="mono">{{ config.value || '空' }}</span>
                </p>
              </div>
            </article>
          </div>
        </SettingsSection>
        <div v-else-if="tab === 'legacy'" class="card py-12 text-center">
          <p class="font-semibold">没有历史兼容配置</p>
          <p class="quiet mt-1 text-sm">当前库中没有需要保留的旧配置键。</p>
        </div>

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
.config-row-off { opacity: 0.62; }
.config-row-head { display: flex; align-items: baseline; gap: 8px; }
.config-row-when {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-top: 6px;
  font-size: 12px;
  color: var(--text-dim);
}
.config-row-meta { margin-top: 6px; font-size: 11.5px; line-height: 1.6; color: var(--text-dim); }
.config-row-control { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; margin-left: auto; }
@media (max-width: 640px) {
  .config-row-control { margin-left: 0; width: 100%; }
}

/* 提醒事件编辑器仍在用的紧凑行内开关。 */
.config-switch { display: flex; align-items: center; gap: 7px; font-size: 13px; cursor: pointer; }
.config-switch span { color: var(--text-dim); }

/* ── 规则类配置的专属卡片 ──
   每一项是一张带说明、影响范围与当前取值预览的卡片；开关型排成紧凑网格，
   数值型留出输入区与单位，越界时在卡片上给出建议，避免长成一片同款输入框。 */
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
.config-card-legacy {
  border-style: dashed;
  background: color-mix(in srgb, var(--surface-sunken) 88%, var(--warning-soft));
}
.config-card-legacy:hover {
  border-color: color-mix(in srgb, var(--warning) 30%, var(--stroke));
}
.config-card-body { flex: 1; min-width: 0; }
.config-card-head { display: flex; align-items: baseline; gap: 8px; }
.config-card-label { font-size: 13.5px; font-weight: 650; }
.config-kind {
  flex-shrink: 0;
  padding: 1px 7px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke-quiet);
  background: var(--surface);
  font-size: 10.5px;
  letter-spacing: 0.04em;
  color: var(--text-quiet);
}
.config-kind-default { border-style: dashed; color: var(--text-dim); }
.config-kind-live {
  border-color: color-mix(in srgb, var(--success) 34%, transparent);
  background: var(--success-soft);
  color: var(--success);
}
.config-kind-legacy {
  border-style: dashed;
  color: var(--warning);
  border-color: color-mix(in srgb, var(--warning) 34%, transparent);
}
.config-affects {
  display: flex;
  align-items: baseline;
  gap: 6px;
  margin-top: 8px;
  font-size: 11.5px;
  color: var(--text-dim);
}
.config-affects-tag {
  flex-shrink: 0;
  padding: 0 6px;
  border-radius: var(--radius-pill);
  background: var(--accent-soft);
  color: var(--accent);
  font-size: 10.5px;
  font-weight: 650;
}
.config-warn {
  margin-top: 7px;
  font-size: 11.5px;
  color: var(--warn, #b45309);
}
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
.config-input .input { padding-right: 12px; }
.config-unit {
  position: absolute;
  right: 12px;
  color: var(--text-quiet);
  font-size: 11.5px;
  pointer-events: none;
}
.config-input:has(.config-unit) .input { padding-right: 52px; }
.input-error {
  border-color: var(--danger);
}
.input-error:focus {
  border-color: var(--danger);
  box-shadow: 0 0 0 3px var(--danger-soft);
}
@media (max-width: 640px) {
  .config-cards { grid-template-columns: 1fr; }
  .config-card-control .btn { margin-left: auto; }
}
</style>
