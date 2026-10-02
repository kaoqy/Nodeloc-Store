<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  deleteAgent,
  deleteQuickReply,
  listAgents,
  listQuickReplies,
  saveAgent,
  saveQuickReply,
  renderQuickReply,
  type CustomerServiceAgent,
  type QuickReply,
} from '../api/support'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()
const canManage = auth.allows('agents', 'manage')

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const agents = ref<{ agent: CustomerServiceAgent; current_load: number }[]>([])
const replies = ref<QuickReply[]>([])
const tab = ref<'agents' | 'replies'>('agents')

const agentDraft = ref<Partial<CustomerServiceAgent>>({
  user_id: 0,
  nickname: '',
  status: 'offline',
  accept_manual: true,
  max_concurrent: 5,
  work_start: '09:00',
  work_end: '21:00',
  role: 'support_agent',
  is_active: true,
})

const replyDraft = ref<Partial<QuickReply>>({
  title: '',
  content: '',
  ticket_types: '',
  roles: '',
  sort_order: 0,
  is_enabled: true,
})

const previewVariables = ref({ ticket_no: 'TK202601010001', user_name: '示例用户', order_no: 'NL20260101001' })
const preview = ref('')

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [agentList, replyList] = await Promise.all([listAgents(), listQuickReplies()])
    agents.value = agentList
    replies.value = replyList
  } catch (err) {
    error.value = errorMessage(err, '加载客服配置失败')
  } finally {
    loading.value = false
  }
}

async function saveAgentDraft() {
  if (!agentDraft.value.user_id) {
    error.value = '请填写客服账号的用户 ID。'
    return
  }
  busy.value = true
  try {
    await saveAgent(agentDraft.value)
    notice.value = '客服配置已保存。'
    agentDraft.value = {
      user_id: 0, nickname: '', status: 'offline', accept_manual: true,
      max_concurrent: 5, work_start: '09:00', work_end: '21:00', role: 'support_agent', is_active: true,
    }
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存客服配置失败')
  } finally {
    busy.value = false
  }
}

function editAgent(row: { agent: CustomerServiceAgent }) {
  agentDraft.value = { ...row.agent }
}

async function removeAgent(row: { agent: CustomerServiceAgent }) {
  if (!row.agent.id || !confirm('移除这位客服？已有工单不会丢失。')) return
  try {
    await deleteAgent(row.agent.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '移除客服失败')
  }
}

async function saveReplyDraft() {
  if (!replyDraft.value.title?.trim() || !replyDraft.value.content?.trim()) {
    error.value = '快捷回复标题和内容都要填。'
    return
  }
  busy.value = true
  try {
    await saveQuickReply(replyDraft.value)
    notice.value = '快捷回复已保存。'
    replyDraft.value = { title: '', content: '', ticket_types: '', roles: '', sort_order: 0, is_enabled: true }
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存快捷回复失败')
  } finally {
    busy.value = false
  }
}

function editReply(item: QuickReply) {
  replyDraft.value = { ...item }
}

async function removeReply(item: QuickReply) {
  if (!item.id || !confirm('删除快捷回复「' + item.title + '」？')) return
  try {
    await deleteQuickReply(item.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除快捷回复失败')
  }
}

async function doPreview(item: QuickReply) {
  if (!item.id) return
  try {
    preview.value = await renderQuickReply(item.id, previewVariables.value)
  } catch (err) {
    error.value = errorMessage(err, '预览失败')
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-bold">客服人员与快捷回复</h2>
        <p class="quiet mt-1 text-xs">人工工单按在线状态与并发量自动分配；没有空闲客服时留在待人工队列。</p>
      </div>
      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'agents' ? 'chip-active' : ''" @click="tab = 'agents'">客服人员</button>
        <button class="chip" :class="tab === 'replies' ? 'chip-active' : ''" @click="tab = 'replies'">快捷回复</button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
    </div>

    <template v-else-if="tab === 'agents'">
      <div class="table-container">
        <table class="table">
          <thead>
            <tr><th>客服</th><th>账号</th><th>状态</th><th>接待人工</th><th class="nums">并发上限</th><th class="nums">当前负载</th><th>工作时间</th><th class="text-right">操作</th></tr>
          </thead>
          <tbody>
            <tr v-if="!agents.length">
              <td colspan="8" class="py-10 text-center text-[var(--text-quiet)]">还没有配置客服坐席</td>
            </tr>
            <tr v-for="row in agents" :key="row.agent.id">
              <td class="font-semibold">{{ row.agent.nickname || '客服 #' + row.agent.id }}</td>
              <td class="mono text-xs">#{{ row.agent.user_id }}</td>
              <td>
                <span :class="row.agent.status === 'online' ? 'badge-success' : row.agent.status === 'busy' ? 'badge-warning' : 'badge'">
                  {{ row.agent.status === 'online' ? '在线' : row.agent.status === 'busy' ? '忙碌' : '离线' }}
                </span>
              </td>
              <td class="text-xs">{{ row.agent.accept_manual ? '是' : '否' }}</td>
              <td class="nums">{{ row.agent.max_concurrent }}</td>
              <td class="nums">{{ row.current_load }}</td>
              <td class="quiet text-xs">{{ row.agent.work_start }}–{{ row.agent.work_end }}</td>
              <td class="text-right">
                <button v-if="canManage" class="btn btn-quiet btn-sm" @click="editAgent(row)">编辑</button>
                <button v-if="canManage" class="btn btn-danger btn-sm" @click="removeAgent(row)">移除</button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="canManage" class="card space-y-3">
        <p class="eyebrow">{{ agentDraft.id ? '编辑客服' : '新增客服' }}</p>
        <div class="grid gap-3 md:grid-cols-4">
          <input v-model.number="agentDraft.user_id" class="input nums" type="number" placeholder="用户 ID" />
          <input v-model="agentDraft.nickname" class="input" placeholder="客服昵称" />
          <select v-model="agentDraft.status" class="input">
            <option value="online">在线</option>
            <option value="busy">忙碌</option>
            <option value="offline">离线</option>
          </select>
          <input v-model.number="agentDraft.max_concurrent" class="input nums" type="number" placeholder="并发上限" />
          <input v-model="agentDraft.work_start" class="input" placeholder="上班时间 09:00" />
          <input v-model="agentDraft.work_end" class="input" placeholder="下班时间 21:00" />
          <select v-model="agentDraft.role" class="input">
            <option value="support_agent">普通客服</option>
            <option value="support_lead">客服主管</option>
            <option value="support">客服</option>
            <option value="admin">管理员</option>
          </select>
          <label class="flex items-center gap-2 text-sm"><input v-model="agentDraft.accept_manual" type="checkbox" />接待人工工单</label>
        </div>
        <div class="flex gap-2">
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="saveAgentDraft">保存客服</button>
        </div>
      </div>
    </template>

    <template v-else>
      <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
        <div class="table-container">
          <table class="table">
            <thead>
              <tr><th>标题</th><th>内容</th><th>适用类型</th><th class="nums">使用次数</th><th>状态</th><th class="text-right">操作</th></tr>
            </thead>
            <tbody>
              <tr v-if="!replies.length">
                <td colspan="6" class="py-10 text-center text-[var(--text-quiet)]">还没有快捷回复</td>
              </tr>
              <tr v-for="item in replies" :key="item.id">
                <td class="font-semibold">{{ item.title }}</td>
                <td class="quiet max-w-[260px] truncate text-xs">{{ item.content }}</td>
                <td class="text-xs">{{ item.ticket_types || '全部' }}</td>
                <td class="nums">{{ item.use_count || 0 }}</td>
                <td><span :class="item.is_enabled ? 'badge-success' : 'badge'">{{ item.is_enabled ? '启用' : '停用' }}</span></td>
                <td class="text-right">
                  <div class="flex justify-end gap-1.5">
                    <button class="btn btn-quiet btn-sm" @click="doPreview(item)">预览</button>
                    <button v-if="canManage" class="btn btn-quiet btn-sm" @click="editReply(item)">编辑</button>
                    <button v-if="canManage" class="btn btn-danger btn-sm" @click="removeReply(item)">删除</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <aside class="space-y-4">
          <div v-if="canManage" class="card space-y-3">
            <p class="eyebrow">{{ replyDraft.id ? '编辑快捷回复' : '新增快捷回复' }}</p>
            <input v-model="replyDraft.title" class="input" placeholder="标题" />
            <textarea v-model="replyDraft.content" class="input min-h-[120px]" placeholder="内容，可用 {ticket_no} {user_name} {order_no} 变量" />
            <input v-model="replyDraft.ticket_types" class="input" placeholder="适用工单类型，逗号分隔" />
            <input v-model="replyDraft.roles" class="input" placeholder="可见角色，逗号分隔" />
            <div class="flex items-center gap-3">
              <label class="flex items-center gap-2 text-sm"><input v-model="replyDraft.is_enabled" type="checkbox" />启用</label>
              <input v-model.number="replyDraft.sort_order" class="input nums" type="number" placeholder="排序" />
            </div>
            <button class="btn btn-primary btn-sm" :disabled="busy" @click="saveReplyDraft">保存</button>
          </div>

          <div v-if="preview" class="card">
            <p class="eyebrow">预览</p>
            <p class="mt-2 whitespace-pre-line text-sm">{{ preview }}</p>
          </div>
        </aside>
      </div>
    </template>
  </section>
</template>
