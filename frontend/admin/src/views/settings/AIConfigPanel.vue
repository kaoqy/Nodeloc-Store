<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import {
  getAIConfig,
  saveAIConfig,
  saveAIWorkflow,
  listQuickQuestions,
  saveQuickQuestion,
  deleteQuickQuestion,
  listAIConversations,
  listAIFeedback,
  listAIToolCalls,
  type AIConfig,
  type AIWorkflow,
  type AIQuickQuestion,
  type AIConversationRow,
  type AIFeedbackRow,
  type AIToolCall,
} from '../../api/support'
import { errorMessage, when } from '../../utils/format'
import SaveBar from '../../components/SaveBar.vue'
import { useAuthStore } from '../../stores/auth'

/**
 * AI 客服配置。
 *
 * 结构上分成五件事：能不能跑起来（基础）、说什么（提示词）、什么时候转人工
 * （工作流）、买家常点的问题（快捷问题），以及事后可查的运行记录（运营数据）。
 * 工具清单不在这里配置——AI 能调哪些工具由代码内置并由服务端逐次校验，
 * 店家只需要在「运营数据」里核对它实际调过什么。
 */

const auth = useAuthStore()
const canManage = computed(() => auth.allows('ai', 'manage'))

type TabKey = 'basic' | 'prompt' | 'workflow' | 'quick' | 'ops'
const TABS: { key: TabKey; label: string; hint: string }[] = [
  { key: 'basic', label: '基础配置', hint: '模型、密钥与频次' },
  { key: 'prompt', label: '提示词与文案', hint: 'AI 怎么说话' },
  { key: 'workflow', label: '工作流与转人工', hint: '什么时候交给人工' },
  { key: 'quick', label: '快捷问题', hint: '买家一键提问' },
  { key: 'ops', label: '运营数据', hint: '会话、评价与工具调用' },
]

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const tab = ref<TabKey>('basic')
const hasKey = ref(false)
const apiKey = ref('')

const config = ref<Partial<AIConfig>>({})
const workflow = ref<Partial<AIWorkflow>>({})
const questions = ref<AIQuickQuestion[]>([])
const questionDraft = ref<Partial<AIQuickQuestion>>({
  title: '', content: '', position: 'widget', sort_order: 0, is_enabled: true, require_login: false,
})

const conversations = ref<AIConversationRow[]>([])
const feedback = ref<AIFeedbackRow[]>([])
const toolCalls = ref<AIToolCall[]>([])
const conversationTotal = ref(0)
const opsLoading = ref(false)
const opsError = ref('')

// 未保存状态：加载时给配置拍一张快照，任何字段改动都会让 dirty 变真。
const snapshot = ref('')
const dirty = computed(() => {
  if (apiKey.value.trim()) return true
  return snapshot.value !== '' && JSON.stringify({ c: config.value, w: workflow.value }) !== snapshot.value
})

function takeSnapshot() {
  snapshot.value = JSON.stringify({ c: config.value, w: workflow.value })
}

// 启用 AI 需要三样东西齐全，缺一样就先告诉店家缺什么，而不是等保存时报错。
const missing = computed(() => {
  const list: string[] = []
  if (!config.value.base_url?.trim()) list.push('API 地址')
  if (!config.value.model?.trim()) list.push('模型名称')
  if (!hasKey.value && !apiKey.value.trim()) list.push('API Key')
  return list
})
const ready = computed(() => missing.value.length === 0)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const payload = await getAIConfig()
    config.value = { ...payload.config }
    workflow.value = { ...payload.workflow }
    hasKey.value = payload.has_key
    questions.value = await listQuickQuestions().catch(() => [])
    takeSnapshot()
  } catch (err) {
    error.value = errorMessage(err, '加载 AI 配置失败')
  } finally {
    loading.value = false
  }
}

function toggleEnabled() {
  if (!config.value.is_enabled && !ready.value) {
    error.value = '启用前先把这几项填好：' + missing.value.join('、')
    return
  }
  error.value = ''
  config.value.is_enabled = !config.value.is_enabled
}

async function saveBasic() {
  if (config.value.is_enabled && !ready.value) {
    error.value = '启用前先把这几项填好：' + missing.value.join('、')
    return
  }
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload: Partial<AIConfig> & { api_key?: string } = { ...config.value }
    if (apiKey.value.trim()) payload.api_key = apiKey.value.trim()
    await saveAIConfig(payload)
    apiKey.value = ''
    notice.value = 'AI 配置已保存。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存 AI 配置失败')
  } finally {
    saving.value = false
  }
}

async function clearKey() {
  if (!confirm('清除已保存的 API Key？清除后 AI 客服会停用。')) return
  saving.value = true
  try {
    await saveAIConfig({ ...config.value, api_key: '__clear__' })
    notice.value = 'API Key 已清除。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '清除密钥失败')
  } finally {
    saving.value = false
  }
}

async function saveFlow() {
  saving.value = true
  error.value = ''
  try {
    workflow.value = await saveAIWorkflow(workflow.value)
    notice.value = '工作流配置已保存。'
    takeSnapshot()
  } catch (err) {
    error.value = errorMessage(err, '保存工作流失败')
  } finally {
    saving.value = false
  }
}

async function saveQuestion() {
  if (!questionDraft.value.title?.trim()) {
    error.value = '快捷问题标题要填。'
    return
  }
  saving.value = true
  error.value = ''
  try {
    await saveQuickQuestion(questionDraft.value)
    questionDraft.value = { title: '', content: '', position: 'widget', sort_order: 0, is_enabled: true, require_login: false }
    questions.value = await listQuickQuestions()
    notice.value = '快捷问题已保存。'
  } catch (err) {
    error.value = errorMessage(err, '保存快捷问题失败')
  } finally {
    saving.value = false
  }
}

function editQuestion(item: AIQuickQuestion) {
  questionDraft.value = { ...item }
}

async function removeQuestion(item: AIQuickQuestion) {
  if (!item.id || !confirm('删除快捷问题「' + item.title + '」？')) return
  try {
    await deleteQuickQuestion(item.id)
    questions.value = await listQuickQuestions()
    notice.value = '快捷问题已删除。'
  } catch (err) {
    error.value = errorMessage(err, '删除快捷问题失败')
  }
}

const handoffCount = computed(() => conversations.value.filter((item) => item.handed_to_human).length)
const handoffRate = computed(() =>
  conversations.value.length ? Math.round((handoffCount.value / conversations.value.length) * 100) : 0,
)
const ratingAverage = computed(() => {
  if (!feedback.value.length) return 0
  return feedback.value.reduce((sum, item) => sum + item.rating, 0) / feedback.value.length
})
const upCount = computed(() => feedback.value.filter((item) => item.rating > 0).length)
const downCount = computed(() => feedback.value.filter((item) => item.rating < 0).length)
const failedCalls = computed(() => toolCalls.value.filter((item) => item.status !== 'ok').length)

async function loadOps() {
  opsLoading.value = true
  opsError.value = ''
  try {
    const [page, rows, calls] = await Promise.all([
      listAIConversations({ limit: 50 }),
      listAIFeedback(100).catch(() => []),
      listAIToolCalls({ limit: 50 }).catch(() => ({ data: [], total: 0 })),
    ])
    conversations.value = page.data
    conversationTotal.value = page.total
    feedback.value = rows
    toolCalls.value = calls.data
  } catch (err) {
    opsError.value = errorMessage(err, '加载 AI 运营数据失败')
  } finally {
    opsLoading.value = false
  }
}

const channelLabels: Record<string, string> = { widget: '网页客服', ticket: '工单', admin: '后台' }

watch(tab, (value) => {
  if (value === 'ops') void loadOps()
})

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <p v-if="!canManage" class="alert" role="status">当前角色只能查看 AI 配置，修改需要「AI 客服」管理权限。</p>

    <SaveBar
      v-if="tab !== 'ops'"
      resource="ai"
      :saving="saving"
      :dirty="dirty"
      :save-label="tab === 'workflow' ? '保存工作流' : tab === 'quick' ? '保存快捷问题' : '保存配置'"
      @save="tab === 'workflow' ? saveFlow() : tab === 'quick' ? saveQuestion() : saveBasic()"
    />

    <div class="flex flex-wrap gap-1.5" role="tablist" aria-label="AI 客服配置分区">
      <button
        v-for="item in TABS"
        :key="item.key"
        class="chip"
        :class="tab === item.key ? 'chip-active' : ''"
        role="tab"
        :aria-selected="tab === item.key"
        @click="tab = item.key"
      >
        {{ item.label }}
      </button>
    </div>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 6" :key="i" class="skeleton h-9 w-full" />
    </div>

    <template v-else>
      <!-- 基础配置 -->
      <div v-if="tab === 'basic'" class="space-y-4">
        <div class="card space-y-3">
          <div class="flex flex-wrap items-center gap-3">
            <div class="min-w-0 flex-1">
              <p class="text-sm font-semibold">AI 客服</p>
              <p class="quiet mt-0.5 text-xs">
                {{ config.is_enabled ? '已启用：买家发起工单后由 AI 先接待。' : '已停用：工单会直接进入待人工队列。' }}
              </p>
            </div>
            <button
              class="btn btn-sm"
              :class="config.is_enabled ? 'btn-secondary' : 'btn-primary'"
              :disabled="!canManage"
              @click="toggleEnabled"
            >
              {{ config.is_enabled ? '停用' : '启用' }}
            </button>
          </div>
          <p v-if="!ready" class="alert alert-warning text-xs">
            启用前还需要：{{ missing.join('、') }}。填写后点「启用」，再点右上角保存。
          </p>
        </div>

        <div class="card space-y-4">
          <p class="eyebrow">模型连接</p>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="label" for="ai-provider">服务商</label>
              <input id="ai-provider" v-model="config.provider" class="input" placeholder="openai-compatible" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-model">模型名称</label>
              <input id="ai-model" v-model="config.model" class="input" placeholder="gpt-4o-mini" :disabled="!canManage" />
            </div>
            <div class="md:col-span-2">
              <label class="label" for="ai-url">API 地址（OpenAI 兼容）</label>
              <input id="ai-url" v-model="config.base_url" class="input mono text-xs" placeholder="https://api.openai.com/v1" :disabled="!canManage" />
            </div>
            <div class="md:col-span-2">
              <label class="label" for="ai-key">API Key</label>
              <div class="flex gap-2">
                <input
                  id="ai-key"
                  v-model="apiKey"
                  class="input mono text-xs"
                  type="password"
                  autocomplete="new-password"
                  :placeholder="hasKey ? '已保存，留空表示保持不变' : 'sk-…'"
                  :disabled="!canManage"
                />
                <button v-if="canManage && hasKey" class="btn btn-danger btn-sm" type="button" @click="clearKey">清除</button>
              </div>
              <p class="hint mt-1">密钥加密存储，页面上只显示「已配置」，不会回传原文。</p>
            </div>
          </div>
        </div>

        <div class="card space-y-4">
          <p class="eyebrow">生成参数</p>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class="label" for="ai-timeout">请求超时（毫秒）</label>
              <input id="ai-timeout" v-model.number="config.timeout_ms" class="input nums" type="number" min="1000" max="120000" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-context">最大上下文条数</label>
              <input id="ai-context" v-model.number="config.max_context" class="input nums" type="number" min="1" max="60" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-reply">最大回复长度</label>
              <input id="ai-reply" v-model.number="config.max_reply_len" class="input nums" type="number" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-temperature">温度参数</label>
              <input id="ai-temperature" v-model.number="config.temperature" class="input nums" type="number" step="0.1" min="0" max="2" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-topp">Top P</label>
              <input id="ai-topp" v-model.number="config.top_p" class="input nums" type="number" step="0.05" min="0" max="1" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-maxlen">单条消息最大长度</label>
              <input id="ai-maxlen" v-model.number="config.max_message_len" class="input nums" type="number" :disabled="!canManage" />
            </div>
          </div>
        </div>

        <div class="card space-y-4">
          <p class="eyebrow">身份与频次</p>
          <div class="grid gap-4 md:grid-cols-2">
            <div>
              <label class="label" for="ai-name">客服名称</label>
              <input id="ai-name" v-model="config.agent_name" class="input" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-avatar">客服头像地址</label>
              <input id="ai-avatar" v-model="config.avatar" class="input mono text-xs" placeholder="https://…" :disabled="!canManage" />
            </div>
          </div>
          <div class="grid gap-3 md:grid-cols-3">
            <label class="flex items-center gap-2 text-sm">
              <input v-model="config.guest_allowed" type="checkbox" :disabled="!canManage" />
              允许游客使用
            </label>
            <label class="flex items-center gap-2 text-sm">
              <input v-model="config.rating_enabled" type="checkbox" :disabled="!canManage" />
              启用满意度评价
            </label>
            <label class="flex items-center gap-2 text-sm">
              <input v-model="config.log_enabled" type="checkbox" :disabled="!canManage" />
              记录对话日志
            </label>
          </div>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class="label" for="ai-guest">游客每日次数</label>
              <input id="ai-guest" v-model.number="config.guest_daily_limit" class="input nums" type="number" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-user">用户每日次数</label>
              <input id="ai-user" v-model.number="config.user_daily_limit" class="input nums" type="number" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="ai-ip">单 IP 每小时限制</label>
              <input id="ai-ip" v-model.number="config.ip_rate_limit" class="input nums" type="number" :disabled="!canManage" />
            </div>
          </div>
        </div>
      </div>

      <!-- 提示词 -->
      <div v-else-if="tab === 'prompt'" class="space-y-4">
        <div class="card space-y-3">
          <label class="label" for="ai-system">系统提示词</label>
          <p class="hint">AI 的角色设定与硬性规则。留空会用内置的保守提示词。</p>
          <textarea id="ai-system" v-model="config.system_prompt" class="input min-h-[220px] mono text-xs" :disabled="!canManage" />
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div class="card space-y-2">
            <label class="label" for="ai-greeting">开场白</label>
            <textarea id="ai-greeting" v-model="config.greeting" class="input min-h-[100px]" :disabled="!canManage" />
          </div>
          <div class="card space-y-2">
            <label class="label" for="ai-fallback">无法回答时的默认回复</label>
            <textarea id="ai-fallback" v-model="config.fallback_reply" class="input min-h-[100px]" :disabled="!canManage" />
          </div>
          <div class="card space-y-2">
            <label class="label" for="ai-transfer">转人工提示语</label>
            <textarea id="ai-transfer" v-model="config.transfer_tip" class="input min-h-[80px]" :disabled="!canManage" />
          </div>
          <div class="card space-y-2">
            <label class="label" for="ai-ticket">工单创建提示语</label>
            <textarea id="ai-ticket" v-model="config.ticket_tip" class="input min-h-[80px]" :disabled="!canManage" />
          </div>
        </div>
        <div class="card space-y-2">
          <label class="label" for="ai-sensitive">敏感问题提示语</label>
          <p class="hint">涉及账户与资金安全时，AI 会说这句话并建议转人工。</p>
          <textarea id="ai-sensitive" v-model="config.sensitive_tip" class="input min-h-[80px]" :disabled="!canManage" />
        </div>
      </div>

      <!-- 工作流 -->
      <div v-else-if="tab === 'workflow'" class="space-y-4">
        <div class="card space-y-4">
          <p class="eyebrow">处理时长</p>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class="label" for="wf-handle">AI 默认处理时长（分钟）</label>
              <input id="wf-handle" v-model.number="workflow.default_handle_minutes" class="input nums" type="number" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="wf-estimate">预计响应时间（分钟）</label>
              <input id="wf-estimate" v-model.number="workflow.estimate_reply_minutes" class="input nums" type="number" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="wf-hours">人工客服工作时间</label>
              <input id="wf-hours" v-model="workflow.working_hours" class="input" placeholder="每天 09:00 - 21:00" :disabled="!canManage" />
            </div>
          </div>
        </div>

        <div class="card space-y-4">
          <p class="eyebrow">什么时候转人工</p>
          <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_on_explicit" type="checkbox" :disabled="!canManage" />用户要求人工时立即转</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_high_amount" type="checkbox" :disabled="!canManage" />高金额订单直接转</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_card_dispute" type="checkbox" :disabled="!canManage" />卡密争议直接转</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_payment_issue" type="checkbox" :disabled="!canManage" />支付异常直接转</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_abuse" type="checkbox" :disabled="!canManage" />账号封禁类问题转</label>
            <label class="flex items-center gap-2 text-sm">
              <input v-model="workflow.require_human_refund" type="checkbox" :disabled="!canManage" />
              退款一律转人工
            </label>
          </div>
          <p class="hint">「退款一律转人工」关闭时，AI 会自行核对并把退款原路退回买家本人。</p>
          <div class="grid gap-4 md:grid-cols-3">
            <div>
              <label class="label" for="wf-fail">AI 连续失败次数上限</label>
              <input id="wf-fail" v-model.number="workflow.max_failures" class="input nums" type="number" min="1" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="wf-transfer-fail">几次无法回答后建议转人工</label>
              <input id="wf-transfer-fail" v-model.number="workflow.transfer_after_failures" class="input nums" type="number" min="1" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="wf-downvote">几次点踩后转人工</label>
              <input id="wf-downvote" v-model.number="workflow.transfer_after_downvotes" class="input nums" type="number" min="1" :disabled="!canManage" />
            </div>
            <div>
              <label class="label" for="wf-amount">高金额订单阈值</label>
              <input id="wf-amount" v-model.number="workflow.high_amount_threshold" class="input nums" type="number" :disabled="!canManage" />
            </div>
            <div class="md:col-span-2">
              <label class="label" for="wf-notice">转人工提示语</label>
              <input id="wf-notice" v-model="workflow.transfer_notice" class="input" :disabled="!canManage" />
            </div>
          </div>
        </div>

        <div class="card space-y-4">
          <p class="eyebrow">AI 能做什么</p>
          <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_create_ticket" type="checkbox" :disabled="!canManage" />可以创建工单</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_query_order" type="checkbox" :disabled="!canManage" />可以查询订单</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_query_shipping" type="checkbox" :disabled="!canManage" />可以查询发货状态</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_recommend_activity" type="checkbox" :disabled="!canManage" />可以推荐活动</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_refund" type="checkbox" :disabled="!canManage" />可以直接退款</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_notify" type="checkbox" :disabled="!canManage" />可以发送通知</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_update_ticket" type="checkbox" :disabled="!canManage" />可以修改工单状态</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_grant_coupon" type="checkbox" :disabled="!canManage" />可以发放优惠券</label>
          </div>
          <p class="hint">
            AI 只能调用系统内置并逐次校验的工具；任何写操作都绑定当前用户本人，不会碰到别人的数据。
          </p>
        </div>
      </div>

      <!-- 快捷问题 -->
      <div v-else-if="tab === 'quick'" class="space-y-4">
        <div v-if="canManage" class="card space-y-3">
          <p class="eyebrow">{{ questionDraft.id ? '编辑快捷问题' : '新增快捷问题' }}</p>
          <div class="grid gap-3 md:grid-cols-2">
            <input v-model="questionDraft.title" class="input" placeholder="标题，例如 怎么查订单" />
            <input v-model="questionDraft.content" class="input" placeholder="点击后发送的内容" />
            <select v-model="questionDraft.position" class="input">
              <option value="widget">客服窗口</option>
              <option value="home">首页</option>
              <option value="product">商品页</option>
              <option value="order">订单页</option>
              <option value="all">全部位置</option>
            </select>
            <input v-model.number="questionDraft.sort_order" class="input nums" type="number" placeholder="排序" />
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <label class="flex items-center gap-2 text-sm"><input v-model="questionDraft.is_enabled" type="checkbox" />启用</label>
            <label class="flex items-center gap-2 text-sm"><input v-model="questionDraft.require_login" type="checkbox" />仅登录后显示</label>
            <button class="btn btn-primary btn-sm ml-auto" :disabled="saving" @click="saveQuestion">保存</button>
          </div>
        </div>

        <div v-if="!questions.length" class="card py-12 text-center text-sm text-[var(--text-quiet)]">
          还没有配置快捷问题
        </div>
        <div v-else class="table-container">
          <table class="table">
            <thead>
              <tr><th>标题</th><th>内容</th><th>位置</th><th class="nums">排序</th><th>状态</th><th class="text-right">操作</th></tr>
            </thead>
            <tbody>
              <tr v-for="item in questions" :key="item.id">
                <td class="font-semibold">{{ item.title }}</td>
                <td class="quiet max-w-[280px] truncate text-xs">{{ item.content }}</td>
                <td class="text-xs">{{ item.position }}</td>
                <td class="nums">{{ item.sort_order }}</td>
                <td><span :class="item.is_enabled ? 'badge-success' : 'badge'">{{ item.is_enabled ? '启用' : '停用' }}</span></td>
                <td class="text-right">
                  <button v-if="canManage" class="btn btn-quiet btn-sm" @click="editQuestion(item)">编辑</button>
                  <button v-if="canManage" class="btn btn-danger btn-sm" @click="removeQuestion(item)">删除</button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <!-- 运营数据 -->
      <div v-else class="space-y-4">
        <div class="flex flex-wrap items-center gap-2">
          <button class="btn btn-secondary btn-sm" :disabled="opsLoading" @click="loadOps">
            <span v-if="opsLoading" class="spinner !size-3.5" />
            {{ opsLoading ? '加载中…' : '刷新数据' }}
          </button>
          <span class="quiet text-xs">最近 50 场会话、100 条评价与 50 次工具调用</span>
        </div>
        <p v-if="opsError" class="alert alert-danger" role="alert">{{ opsError }}</p>

        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
          <div class="card-quiet">
            <p class="quiet text-xs">会话总数</p>
            <p class="nums mt-1 text-xl font-bold">{{ conversationTotal }}</p>
          </div>
          <div class="card-quiet">
            <p class="quiet text-xs">转人工率</p>
            <p class="nums mt-1 text-xl font-bold">{{ handoffRate }}%</p>
            <p class="hint mt-0.5">{{ handoffCount }} / {{ conversations.length }} 场</p>
          </div>
          <div class="card-quiet">
            <p class="quiet text-xs">平均评价</p>
            <p class="nums mt-1 text-xl font-bold">{{ ratingAverage ? ratingAverage.toFixed(1) : '—' }}</p>
            <p class="hint mt-0.5">有用 {{ upCount }} · 没用 {{ downCount }}</p>
          </div>
          <div class="card-quiet">
            <p class="quiet text-xs">工具调用</p>
            <p class="nums mt-1 text-xl font-bold">{{ toolCalls.length }}</p>
            <p class="hint mt-0.5">失败 {{ failedCalls }} 次</p>
          </div>
        </div>

        <div v-if="opsLoading" class="card space-y-2">
          <div v-for="i in 4" :key="i" class="skeleton h-9 w-full" />
        </div>

        <template v-else>
          <div class="card space-y-3">
            <p class="eyebrow">最近会话</p>
            <div v-if="!conversations.length" class="quiet py-10 text-center text-sm">还没有 AI 会话记录</div>
            <div v-else class="table-container">
              <table class="table">
                <thead>
                  <tr><th>会话</th><th>用户</th><th>渠道</th><th class="nums">消息</th><th>处理方</th><th>最后活跃</th></tr>
                </thead>
                <tbody>
                  <tr v-for="item in conversations" :key="item.id">
                    <td class="mono text-xs">#{{ item.id }}</td>
                    <td class="text-xs">{{ item.user_id ? '#' + item.user_id : '游客' }}</td>
                    <td class="text-xs">{{ channelLabels[item.channel] || item.channel }}</td>
                    <td class="nums text-xs">{{ item.message_count }}</td>
                    <td>
                      <span :class="item.handed_to_human ? 'badge-warning' : 'badge-success'">
                        {{ item.handed_to_human ? '已转人工' : 'AI 处理' }}
                      </span>
                    </td>
                    <td class="quiet text-xs">{{ when(item.last_message_at || item.created_at) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card space-y-3">
            <p class="eyebrow">AI 工具调用</p>
            <p class="quiet text-xs">AI 能调用哪些工具由代码内置并受服务端校验；这里显示每一次实际调用与结果。</p>
            <div v-if="!toolCalls.length" class="quiet py-8 text-center text-sm">还没有工具调用记录</div>
            <div v-else class="table-container">
              <table class="table">
                <thead>
                  <tr><th>工具</th><th>会话</th><th>工单</th><th>用户</th><th>状态</th><th class="nums">耗时</th><th>时间</th></tr>
                </thead>
                <tbody>
                  <tr v-for="call in toolCalls" :key="call.id">
                    <td>
                      <p class="text-xs font-semibold">{{ call.tool_name || call.tool_key }}</p>
                      <p class="mono quiet text-[11px]">{{ call.tool_key }}</p>
                    </td>
                    <td class="mono text-xs">{{ call.conversation_id ? '#' + call.conversation_id : '—' }}</td>
                    <td class="mono text-xs">{{ call.ticket_id ? '#' + call.ticket_id : '—' }}</td>
                    <td class="mono text-xs">{{ call.user_id ? '#' + call.user_id : '游客' }}</td>
                    <td>
                      <span :class="call.status === 'ok' ? 'badge-success' : 'badge-danger'">{{ call.status === 'ok' ? '成功' : '失败' }}</span>
                      <p v-if="call.error" class="quiet mt-0.5 max-w-[180px] truncate text-[11px]">{{ call.error }}</p>
                    </td>
                    <td class="nums text-xs">{{ call.duration_ms }} ms</td>
                    <td class="quiet text-xs">{{ when(call.created_at) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div class="card space-y-3">
            <p class="eyebrow">买家评价</p>
            <div v-if="!feedback.length" class="quiet py-8 text-center text-sm">还没有买家评价</div>
            <div v-else class="table-container">
              <table class="table">
                <thead>
                  <tr><th>评价</th><th>会话</th><th>工单</th><th>说明</th><th>时间</th></tr>
                </thead>
                <tbody>
                  <tr v-for="item in feedback" :key="item.id">
                    <td><span :class="item.rating > 0 ? 'badge-success' : 'badge-danger'">{{ item.rating > 0 ? '有用' : '没用' }}</span></td>
                    <td class="mono text-xs">#{{ item.conversation_id }}</td>
                    <td class="mono text-xs">{{ item.ticket_id ? '#' + item.ticket_id : '—' }}</td>
                    <td class="quiet max-w-[320px] truncate text-xs">{{ item.comment || item.reason || '—' }}</td>
                    <td class="quiet text-xs">{{ when(item.created_at) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </template>
      </div>
    </template>
  </div>
</template>
