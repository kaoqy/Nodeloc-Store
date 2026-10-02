<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  getAIConfig,
  saveAIConfig,
  saveAIWorkflow,
  listQuickQuestions,
  saveQuickQuestion,
  deleteQuickQuestion,
  type AIConfig,
  type AIWorkflow,
  type AIQuickQuestion,
} from '../../api/support'
import { errorMessage } from '../../utils/format'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const canManage = auth.allows('ai', 'manage')

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const tab = ref<'basic' | 'prompt' | 'workflow' | 'quick'>('basic')
const hasKey = ref(false)
const apiKey = ref('')

const config = ref<Partial<AIConfig>>({})
const workflow = ref<Partial<AIWorkflow>>({})
const questions = ref<AIQuickQuestion[]>([])
const questionDraft = ref<Partial<AIQuickQuestion>>({
  title: '',
  content: '',
  position: 'widget',
  sort_order: 0,
  is_enabled: true,
  require_login: false,
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    const payload = await getAIConfig()
    config.value = { ...payload.config }
    workflow.value = { ...payload.workflow }
    hasKey.value = payload.has_key
    questions.value = await listQuickQuestions().catch(() => [])
  } catch (err) {
    error.value = errorMessage(err, '加载 AI 配置失败')
  } finally {
    loading.value = false
  }
}

async function saveBasic() {
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
  try {
    workflow.value = await saveAIWorkflow(workflow.value)
    notice.value = '工作流配置已保存。'
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

async function editQuestion(item: AIQuickQuestion) {
  questionDraft.value = { ...item }
}

async function removeQuestion(item: AIQuickQuestion) {
  if (!item.id || !confirm('删除快捷问题「' + item.title + '」？')) return
  try {
    await deleteQuickQuestion(item.id)
    questions.value = await listQuickQuestions()
  } catch (err) {
    error.value = errorMessage(err, '删除快捷问题失败')
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <p v-if="!canManage" class="alert" role="status">当前角色只能查看 AI 配置，修改需要「AI 客服」管理权限。</p>

    <div class="flex flex-wrap gap-1.5">
      <button class="chip" :class="tab === 'basic' ? 'chip-active' : ''" @click="tab = 'basic'">基础配置</button>
      <button class="chip" :class="tab === 'prompt' ? 'chip-active' : ''" @click="tab = 'prompt'">提示词与文案</button>
      <button class="chip" :class="tab === 'workflow' ? 'chip-active' : ''" @click="tab = 'workflow'">工作流与转人工</button>
      <button class="chip" :class="tab === 'quick' ? 'chip-active' : ''" @click="tab = 'quick'">快捷问题</button>
    </div>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 6" :key="i" class="skeleton h-9 w-full" />
    </div>

    <div v-else-if="tab === 'basic'" class="card space-y-4">
      <label class="flex items-center gap-2 text-sm">
        <input v-model="config.is_enabled" type="checkbox" :disabled="!canManage" />
        启用 AI 客服（关闭后工单会停在待人工）
      </label>
      <div class="grid gap-4 md:grid-cols-2">
        <div>
          <label class="label" for="ai-provider">服务商</label>
          <input id="ai-provider" v-model="config.provider" class="input" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="ai-model">模型名称</label>
          <input id="ai-model" v-model="config.model" class="input" :disabled="!canManage" />
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
              :placeholder="hasKey ? '已保存，留空表示保持不变' : 'sk-…'"
              :disabled="!canManage"
            />
            <button v-if="canManage && hasKey" class="btn btn-danger btn-sm" @click="clearKey">清除</button>
          </div>
        </div>
        <div>
          <label class="label" for="ai-timeout">请求超时（毫秒）</label>
          <input id="ai-timeout" v-model.number="config.timeout_ms" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="ai-context">最大上下文条数</label>
          <input id="ai-context" v-model.number="config.max_context" class="input nums" type="number" :disabled="!canManage" />
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
        <div>
          <label class="label" for="ai-name">客服名称</label>
          <input id="ai-name" v-model="config.agent_name" class="input" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="ai-avatar">客服头像</label>
          <input id="ai-avatar" v-model="config.avatar" class="input mono text-xs" :disabled="!canManage" />
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
      <div class="grid gap-4 md:grid-cols-4">
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
        <label class="flex items-end gap-2 pb-1 text-sm">
          <input v-model="config.mail_enabled" type="checkbox" :disabled="!canManage" />
          启用邮件通知
        </label>
      </div>
    </div>

    <div v-else-if="tab === 'prompt'" class="card space-y-4">
      <div>
        <label class="label" for="ai-system">系统提示词</label>
        <textarea id="ai-system" v-model="config.system_prompt" class="input min-h-[220px] mono text-xs" :disabled="!canManage" />
      </div>
      <div class="grid gap-4 md:grid-cols-2">
        <div>
          <label class="label" for="ai-greeting">开场白</label>
          <textarea id="ai-greeting" v-model="config.greeting" class="input min-h-[100px]" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="ai-fallback">无法回答时的默认回复</label>
          <textarea id="ai-fallback" v-model="config.fallback_reply" class="input min-h-[100px]" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="ai-transfer">转人工提示语</label>
          <textarea id="ai-transfer" v-model="config.transfer_tip" class="input min-h-[80px]" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="ai-ticket">工单创建提示语</label>
          <textarea id="ai-ticket" v-model="config.ticket_tip" class="input min-h-[80px]" :disabled="!canManage" />
        </div>
        <div class="md:col-span-2">
          <label class="label" for="ai-sensitive">敏感问题提示语</label>
          <textarea id="ai-sensitive" v-model="config.sensitive_tip" class="input min-h-[80px]" :disabled="!canManage" />
        </div>
      </div>
    </div>

    <div v-else-if="tab === 'workflow'" class="card space-y-4">
      <div class="grid gap-4 md:grid-cols-3">
        <div>
          <label class="label" for="wf-handle">AI 默认处理时长（分钟）</label>
          <input id="wf-handle" v-model.number="workflow.default_handle_minutes" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="wf-fail">AI 连续失败次数上限</label>
          <input id="wf-fail" v-model.number="workflow.max_failures" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="wf-transfer-fail">几次无法回答后建议转人工</label>
          <input id="wf-transfer-fail" v-model.number="workflow.transfer_after_failures" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="wf-downvote">几次点踩后转人工</label>
          <input id="wf-downvote" v-model.number="workflow.transfer_after_downvotes" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="wf-amount">高金额订单阈值</label>
          <input id="wf-amount" v-model.number="workflow.high_amount_threshold" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="wf-estimate">预计响应时间（分钟）</label>
          <input id="wf-estimate" v-model.number="workflow.estimate_reply_minutes" class="input nums" type="number" :disabled="!canManage" />
        </div>
        <div>
          <label class="label" for="wf-hours">人工客服工作时间</label>
          <input id="wf-hours" v-model="workflow.working_hours" class="input" :disabled="!canManage" />
        </div>
        <div class="md:col-span-2">
          <label class="label" for="wf-notice">转人工提示语</label>
          <input id="wf-notice" v-model="workflow.transfer_notice" class="input" :disabled="!canManage" />
        </div>
      </div>

      <div>
        <p class="label">自动转人工策略</p>
        <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_on_explicit" type="checkbox" :disabled="!canManage" />用户要求人工时立即转</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_high_amount" type="checkbox" :disabled="!canManage" />高金额订单直接转</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_refund" type="checkbox" :disabled="!canManage" />退款问题直接转</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_card_dispute" type="checkbox" :disabled="!canManage" />卡密争议直接转</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_payment_issue" type="checkbox" :disabled="!canManage" />支付异常直接转</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.transfer_abuse" type="checkbox" :disabled="!canManage" />账号封禁类问题转</label>
        </div>
      </div>

      <div>
        <p class="label">AI 行为开关</p>
        <div class="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_create_ticket" type="checkbox" :disabled="!canManage" />可以创建工单</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_update_ticket" type="checkbox" :disabled="!canManage" />可以修改工单状态</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_notify" type="checkbox" :disabled="!canManage" />可以发送通知</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_query_order" type="checkbox" :disabled="!canManage" />可以查询订单</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_query_shipping" type="checkbox" :disabled="!canManage" />可以查询发货状态</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_recommend_activity" type="checkbox" :disabled="!canManage" />可以推荐活动</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="workflow.can_grant_coupon" type="checkbox" :disabled="!canManage" />可以发放优惠券（高风险）</label>
        </div>
      </div>
    </div>

    <div v-else class="space-y-4">
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

      <div v-if="!questions.length" class="card py-12 text-center text-sm text-[var(--text-quiet)]">还没有配置快捷问题</div>
      <div v-else class="table-container">
        <table class="table">
          <thead>
            <tr><th>标题</th><th>内容</th><th>位置</th><th>排序</th><th>状态</th><th class="text-right">操作</th></tr>
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
  </div>
</template>
