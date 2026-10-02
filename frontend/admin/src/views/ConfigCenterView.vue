<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
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
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const canManageTemplates = auth.allows('notification_templates', 'manage')
const canManageSystem = auth.allows('config_center', 'manage')

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const tab = ref<'templates' | 'system' | 'logs'>('templates')
const templates = ref<NotificationTemplate[]>([])
const configs = ref<SystemConfig[]>([])
const logs = ref<NotificationLogRow[]>([])
const logTotal = ref(0)
const logStatus = ref('')
const groupFilter = ref('')

const templateDraft = ref<Partial<NotificationTemplate>>({
  key: '',
  name: '',
  event: '',
  category: 'ticket',
  is_enabled: true,
  in_app: true,
  mail: false,
  title_template: '',
  content_template: '',
  retry_limit: 3,
  sort_order: 0,
})

const grouped = computed(() => {
  const map = new Map<string, SystemConfig[]>()
  for (const config of configs.value) {
    const list = map.get(config.group) ?? []
    list.push(config)
    map.set(config.group, list)
  }
  return Array.from(map.entries())
})

const groupLabels: Record<string, string> = {
  ticket: '工单配置',
  risk: '风控配置',
  retention: '数据保留策略',
  upload: '文件上传配置',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [templateList, configList, logPage] = await Promise.all([
      listTemplates(),
      listSystemConfigs(),
      listNotificationLogs({ status: logStatus.value || undefined, limit: 50 }).catch(() => ({ data: [], total: 0 })),
    ])
    templates.value = templateList
    configs.value = configList
    logs.value = logPage.data
    logTotal.value = logPage.total
  } catch (err) {
    error.value = errorMessage(err, '加载配置中心失败')
  } finally {
    loading.value = false
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
  if (!template.id || !confirm('删除模板「' + template.name + '」？')) return
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

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-bold">配置中心</h2>
        <p class="quiet mt-1 text-xs">通知模板与系统配置集中在这里，修改会记录到审计日志。</p>
      </div>
      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'templates' ? 'chip-active' : ''" @click="tab = 'templates'">通知模板</button>
        <button class="chip" :class="tab === 'system' ? 'chip-active' : ''" @click="tab = 'system'">系统配置</button>
        <button class="chip" :class="tab === 'logs' ? 'chip-active' : ''" @click="tab = 'logs'">发送日志</button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
    </div>

    <template v-else-if="tab === 'templates'">
      <div class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th>模板</th><th>事件</th><th>渠道</th><th>标题模板</th><th>状态</th><th class="text-right">操作</th>
            </tr>
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
              <td class="quiet max-w-[260px] truncate text-xs">{{ item.title_template }}</td>
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
        <input v-model="templateDraft.title_template" class="input" placeholder="标题模板，例如 工单 {{ticket_no}} 已创建" />
        <textarea v-model="templateDraft.content_template" class="input min-h-[100px]" placeholder="内容模板" />
        <div class="flex flex-wrap items-center gap-4">
          <label class="flex items-center gap-2 text-sm"><input v-model="templateDraft.is_enabled" type="checkbox" />启用</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="templateDraft.in_app" type="checkbox" />站内通知</label>
          <label class="flex items-center gap-2 text-sm"><input v-model="templateDraft.mail" type="checkbox" />邮件通知</label>
          <input v-model.number="templateDraft.retry_limit" class="input nums !w-32" type="number" placeholder="重试次数" />
          <button class="btn btn-primary btn-sm ml-auto" :disabled="busy" @click="saveTemplateDraft">保存模板</button>
        </div>
      </div>
    </template>

    <template v-else-if="tab === 'logs'">
      <div class="flex flex-wrap items-center gap-2">
        <select v-model="logStatus" class="input !w-auto" @change="load">
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
              <td class="quiet max-w-[260px] truncate text-xs">{{ row.title }}</td>
              <td>
                <span :class="row.status === 'sent' ? 'badge-success' : 'badge-danger'">{{ row.status === 'sent' ? '已发送' : '失败' }}</span>
                <p v-if="row.error" class="quiet mt-0.5 max-w-[220px] truncate text-[11px]">{{ row.error }}</p>
              </td>
              <td class="quiet text-xs">{{ row.created_at }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>

    <template v-else>
      <div class="flex flex-wrap items-center gap-2">
        <select v-model="groupFilter" class="input !w-auto">
          <option value="">全部分组</option>
          <option v-for="[group] in grouped" :key="group" :value="group">{{ groupLabels[group] || group }}</option>
        </select>
      </div>
      <p v-if="!canManageSystem" class="alert" role="status">当前角色只能查看系统配置，修改需要「配置中心」管理权限。</p>

      <div v-for="[group, items] in grouped" v-show="!groupFilter || groupFilter === group" :key="group" class="card space-y-3">
        <p class="eyebrow">{{ groupLabels[group] || group }}</p>
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
    </template>
  </section>
</template>
