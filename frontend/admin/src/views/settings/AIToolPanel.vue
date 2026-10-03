<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  listAITools,
  listAIToolCalls,
  listAIRoles,
  listAIToolCatalogue,
  type AIToolSummary,
  saveAITool,
  setAIToolPermission,
  type AIToolCall,
  type AIToolRow,
} from '../../api/support'
import { errorMessage, when } from '../../utils/format'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const canManage = auth.allows('ai_tools', 'manage')

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const tab = ref<'tools' | 'calls'>('tools')
const rows = ref<AIToolRow[]>([])
const roles = ref<string[]>([])
// AI 真正拿到的工具清单：与写进系统提示词的内容一致，店家在这里核对即可。
const catalogue = ref<AIToolSummary[]>([])
const catalogueRole = ref('user')
const calls = ref<AIToolCall[]>([])
const callTotal = ref(0)
const callFilter = ref({ tool: '', status: '' })

const riskTone: Record<string, string> = {
  low: 'badge-success',
  medium: 'badge-info',
  high: 'badge-warning',
  critical: 'badge-danger',
}

const riskLabel: Record<string, string> = {
  low: '低',
  medium: '中',
  high: '高',
  critical: '极高',
}

const roleLabel: Record<string, string> = {
  guest: '游客',
  user: '普通用户',
  support: '客服',
  support_agent: '普通客服',
  support_lead: '客服主管',
  operator: '运营',
  ops_manager: '运营管理员',
  product_manager: '商品管理员',
  order_manager: '订单管理员',
  finance: '财务',
  ai_admin: 'AI 管理员',
  data_viewer: '数据查看员',
  admin: '管理员',
  super_admin: '超级管理员',
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [toolList, roleList] = await Promise.all([listAITools(), listAIRoles()])
    rows.value = toolList
    roles.value = roleList
    await Promise.all([loadCalls(), loadCatalogue()])
  } catch (err) {
    error.value = errorMessage(err, '加载 AI 工具失败')
  } finally {
    loading.value = false
  }
}

// loadCatalogue 取「这个角色下 AI 能用哪些工具」，与提示词同源。
async function loadCatalogue() {
  catalogue.value = await listAIToolCatalogue(catalogueRole.value).catch(() => [])
}

async function loadCalls() {
  const page = await listAIToolCalls({
    tool: callFilter.value.tool || undefined,
    status: callFilter.value.status || undefined,
    limit: 50,
  })
  calls.value = page.data
  callTotal.value = page.total
}

async function toggleTool(row: AIToolRow) {
  if (!canManage) return
  busy.value = true
  error.value = ''
  try {
    const next = await saveAITool(row.tool.id, { ...row.tool, is_enabled: !row.tool.is_enabled })
    row.tool = next
    notice.value = '工具「' + next.name + '」已' + (next.is_enabled ? '启用' : '停用') + '。'
  } catch (err) {
    error.value = errorMessage(err, '更新工具失败')
  } finally {
    busy.value = false
  }
}

async function toggleAuto(row: AIToolRow) {
  if (!canManage) return
  busy.value = true
  try {
    row.tool = await saveAITool(row.tool.id, { ...row.tool, allow_auto: !row.tool.allow_auto })
    notice.value = '已更新自动调用策略。'
  } catch (err) {
    error.value = errorMessage(err, '更新工具失败')
  } finally {
    busy.value = false
  }
}

async function toggleConfirm(row: AIToolRow) {
  if (!canManage) return
  busy.value = true
  try {
    row.tool = await saveAITool(row.tool.id, { ...row.tool, require_confirm: !row.tool.require_confirm })
    notice.value = '已更新二次确认策略。'
  } catch (err) {
    error.value = errorMessage(err, '更新工具失败')
  } finally {
    busy.value = false
  }
}

async function permission(row: AIToolRow, role: string) {
  if (!canManage) return
  const current = (row.permissions ?? []).find((item) => item.role === role)
  const allowed = current ? !current.allowed : false
  busy.value = true
  try {
    await setAIToolPermission(row.tool.id, role, allowed)
    if (current) current.allowed = allowed
    else (row.permissions ??= []).push({ tool_id: row.tool.id, role, allowed })
    notice.value = '已更新 ' + (roleLabel[role] || role) + ' 的工具权限。'
  } catch (err) {
    error.value = errorMessage(err, '更新权限失败')
  } finally {
    busy.value = false
  }
}

function permissionState(row: AIToolRow, role: string) {
  // 后端老版本可能返回 null；这里兜底，避免整页白屏。
  const found = (row.permissions ?? []).find((item) => item.role === role)
  if (found) return found.allowed
  return false
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <p v-if="!canManage" class="alert" role="status">当前角色只能查看工具配置，修改需要「AI 工具」管理权限。</p>

    <div class="card space-y-3">
      <div class="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p class="eyebrow">AI 可调用的工具</p>
          <p class="quiet mt-1 text-xs">
            这里列出的工具会被写进 AI 的系统提示词。AI 只能调用这份清单，执行时还会再校验一次权限。
          </p>
        </div>
        <select v-model="catalogueRole" class="input !w-auto" aria-label="按角色查看" @change="loadCatalogue">
          <option v-for="role in roles" :key="role" :value="role">{{ roleLabel[role] || role }}</option>
        </select>
      </div>
      <div v-if="!catalogue.length" class="card-quiet py-6 text-center text-sm text-[var(--text-quiet)]">
        这个角色当前没有可用工具。检查上面的开关与授权。
      </div>
      <ul v-else class="grid gap-2 sm:grid-cols-2">
        <li v-for="tool in catalogue" :key="tool.key" class="card-quiet">
          <p class="text-[13px] font-semibold">{{ tool.name }}</p>
          <p class="mono quiet text-[11px]">{{ tool.key }}</p>
          <p class="quiet mt-1 text-xs">{{ tool.description }}</p>
          <p v-if="tool.params?.length" class="mono mt-1 text-[11px] text-[var(--text-quiet)]">
            参数：{{ tool.params.join('、') }}
          </p>
        </li>
      </ul>
    </div>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 6" :key="i" class="skeleton h-10 w-full" />
    </div>

    <template v-else-if="tab === 'tools'">
      <div v-if="!rows.length" class="card py-16 text-center text-sm text-[var(--text-quiet)]">
        还没有注册工具。保存一次 AI 配置后系统会自动写入内置工具。
      </div>
      <div v-for="row in rows" :key="row.tool.id" class="card space-y-3">
        <div class="flex flex-wrap items-center gap-2">
          <p class="font-semibold">{{ row.tool.name }}</p>
          <span class="mono quiet text-xs">{{ row.tool.key }}</span>
          <span :class="riskTone[row.tool.risk_level] || 'badge'">风险：{{ riskLabel[row.tool.risk_level] || row.tool.risk_level }}</span>
          <span v-if="row.tool.builtin" class="badge">内置</span>
          <span :class="row.tool.is_enabled ? 'badge-success' : 'badge'">{{ row.tool.is_enabled ? '已启用' : '已停用' }}</span>
          <div class="ml-auto flex gap-1.5">
            <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggleTool(row)">
              {{ row.tool.is_enabled ? '停用' : '启用' }}
            </button>
          </div>
        </div>
        <p class="quiet text-xs">{{ row.tool.description || '—' }}</p>
        <div class="flex flex-wrap items-center gap-4 text-xs">
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              :checked="row.tool.allow_auto"
              :disabled="!canManage"
              @change="toggleAuto(row)"
            />
            允许 AI 自动调用
          </label>
          <label class="flex items-center gap-2">
            <input
              type="checkbox"
              :checked="row.tool.require_confirm"
              :disabled="!canManage"
              @change="toggleConfirm(row)"
            />
            需要用户二次确认
          </label>
          <span class="quiet">限频 {{ row.tool.rate_limit }}/分钟</span>
          <span class="quiet">超时 {{ row.tool.timeout_ms }} ms</span>
          <span class="quiet">{{ row.tool.own_data_only ? '仅限本人数据' : '公开数据' }}</span>
          <span class="quiet">{{ row.tool.require_login ? '需要登录' : '无需登录' }}</span>
        </div>
        <details>
          <summary class="cursor-pointer text-xs accent-text">角色授权</summary>
          <div class="mt-2 flex flex-wrap gap-2">
            <button
              v-for="role in roles"
              :key="role"
              class="chip"
              :class="permissionState(row, role) ? 'chip-active' : ''"
              :disabled="!canManage || busy"
              @click="permission(row, role)"
            >
              {{ roleLabel[role] || role }}
            </button>
          </div>
        </details>
      </div>
    </template>

    <template v-else>
      <div class="flex flex-wrap items-center gap-2">
        <input v-model="callFilter.tool" class="input w-56" placeholder="工具标识" @keyup.enter="loadCalls" />
        <select v-model="callFilter.status" class="input !w-auto" @change="loadCalls">
          <option value="">全部结果</option>
          <option value="ok">成功</option>
          <option value="failed">失败</option>
        </select>
        <button class="btn btn-secondary btn-sm" @click="loadCalls">查询</button>
      </div>
      <div class="table-container">
        <table class="table">
          <thead>
            <tr><th>工具</th><th>用户</th><th>参数</th><th>结果</th><th>状态</th><th class="nums">耗时</th><th>时间</th></tr>
          </thead>
          <tbody>
            <tr v-if="!calls.length">
              <td colspan="7" class="py-10 text-center text-[var(--text-quiet)]">还没有工具调用记录</td>
            </tr>
            <tr v-for="call in calls" :key="call.id">
              <td>
                <p class="text-xs font-semibold">{{ call.tool_name || call.tool_key }}</p>
                <p class="mono quiet text-[11px]">{{ call.tool_key }}</p>
              </td>
              <td class="mono text-xs">{{ call.user_id ? '#' + call.user_id : '游客' }}</td>
              <td class="mono max-w-[200px] truncate text-[11px]">{{ call.params || '—' }}</td>
              <td class="mono max-w-[240px] truncate text-[11px]">{{ call.result || call.error || '—' }}</td>
              <td><span :class="call.status === 'ok' ? 'badge-success' : 'badge-danger'">{{ call.status }}</span></td>
              <td class="nums text-xs">{{ call.duration_ms }} ms</td>
              <td class="quiet text-xs">{{ when(call.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </div>
</template>
