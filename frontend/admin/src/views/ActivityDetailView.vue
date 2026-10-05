<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  getActivity,
  getActivityStats,
  listActivityLogs,
  listActivityRecords,
  setActivityStatus,
  type Activity,
  type ActivityLogRow,
  type ActivityRecordRow,
  type ActivityStats,
} from '../api/activities'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const id = computed(() => Number(route.params.id || 0))
const canManage = computed(() => auth.allows('activities', 'manage'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const activity = ref<Activity | null>(null)
const stats = ref<ActivityStats | null>(null)
const records = ref<ActivityRecordRow[]>([])
const logs = ref<ActivityLogRow[]>([])
const tab = ref<'stats' | 'records' | 'logs'>('stats')

const recordTotal = ref(0)
const logTotal = ref(0)

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [detail, metric, recordPage, logPage] = await Promise.all([
      getActivity(id.value),
      getActivityStats(id.value),
      listActivityRecords(id.value, 20, 0),
      listActivityLogs(id.value, 20, 0),
    ])
    activity.value = detail
    stats.value = metric
    records.value = recordPage.data
    recordTotal.value = recordPage.total
    logs.value = logPage.data
    logTotal.value = logPage.total
  } catch (err) {
    error.value = errorMessage(err, '加载活动数据失败')
  } finally {
    loading.value = false
  }
}

async function toggle() {
  if (!activity.value) return
  busy.value = true
  error.value = ''
  try {
    const next = activity.value.status === 'running' ? 'paused' : 'running'
    await setActivityStatus(activity.value.id, next, next === 'paused' ? '管理员暂停活动' : '管理员恢复活动')
    notice.value = next === 'paused' ? '活动已暂停。' : '活动已恢复。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '更新状态失败')
  } finally {
    busy.value = false
  }
}

const recordStatusLabels: Record<string, string> = {
  reserved: '已占用',
  used: '已支付',
  refunded: '已退款',
  cancelled: '已取消',
}

const percent = (value: number) => (value * 100).toFixed(1) + '%'

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="page-title">{{ activity?.name || '活动数据' }}</h2>
        <p class="quiet mt-1 text-xs">
          {{ activity?.subtitle || '参与情况、优惠金额与操作日志' }}
        </p>
      </div>
      <div class="flex gap-2">
        <RouterLink to="/activities" class="btn btn-secondary btn-sm">返回列表</RouterLink>
        <RouterLink v-if="canManage" :to="'/activities/' + id + '/edit'" class="btn btn-secondary btn-sm">编辑活动</RouterLink>
        <button v-if="canManage" class="btn btn-primary btn-sm" :disabled="busy" @click="toggle">
          {{ activity?.status === 'running' ? '暂停活动' : '恢复上线' }}
        </button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <div v-if="error && !stats" class="card py-12 text-center">
      <p class="quiet text-sm">活动数据暂时不可用。</p>
      <div class="mt-4 flex justify-center gap-2">
        <button class="btn btn-secondary btn-sm" :disabled="loading" @click="load">重新加载</button>
        <RouterLink to="/activities" class="btn btn-quiet btn-sm">返回活动列表</RouterLink>
      </div>
    </div>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 4" :key="i" class="skeleton h-9 w-full" />
    </div>

    <template v-else-if="stats">
      <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <div class="card !p-5">
          <p class="eyebrow">参与人数</p>
          <p class="nums mt-2 text-2xl font-bold">{{ stats.participants }}</p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">订单数</p>
          <p class="nums mt-2 text-2xl font-bold">{{ stats.orders }}</p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">优惠金额</p>
          <p class="nums mt-2 text-2xl font-bold">{{ money(stats.discount_total) }}</p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">活动收入</p>
          <p class="nums mt-2 text-2xl font-bold">{{ money(stats.revenue_total) }}</p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">名额使用</p>
          <p class="nums mt-2 text-lg font-bold">
            {{ stats.quota_used }} / {{ stats.quota_limit || '不限' }}
          </p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">库存使用</p>
          <p class="nums mt-2 text-lg font-bold">
            {{ stats.stock_used }} / {{ stats.stock_limit || '不限' }}
          </p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">转化率</p>
          <p class="nums mt-2 text-lg font-bold">{{ percent(stats.conversion_rate) }}</p>
        </div>
        <div class="card !p-5">
          <p class="eyebrow">退款笔数</p>
          <p class="nums mt-2 text-lg font-bold">{{ stats.refund_count }}</p>
        </div>
      </div>

      <div class="flex gap-1.5">
        <button class="chip" :class="tab === 'stats' ? 'chip-active' : ''" @click="tab = 'stats'">参与记录</button>
        <button class="chip" :class="tab === 'logs' ? 'chip-active' : ''" @click="tab = 'logs'">操作日志</button>
      </div>

      <div v-if="tab === 'stats'" class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th>用户</th>
              <th class="nums">数量</th>
              <th class="nums">原价</th>
              <th class="nums">优惠</th>
              <th class="nums">实付</th>
              <th>状态</th>
              <th>时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!records.length">
              <td colspan="7" class="py-10 text-center text-[var(--text-quiet)]">还没有参与记录</td>
            </tr>
            <tr v-for="record in records" :key="record.id">
              <td class="mono text-xs">#{{ record.user_id }}</td>
              <td class="nums">{{ record.quantity }}</td>
              <td class="nums">{{ money(record.original_amount) }}</td>
              <td class="nums accent-text">{{ money(record.discount_amount) }}</td>
              <td class="nums">{{ money(record.payable_amount) }}</td>
              <td><span class="badge">{{ recordStatusLabels[record.status] || record.status }}</span></td>
              <td class="quiet text-xs">{{ when(record.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-else class="table-container">
        <table class="table">
          <thead>
            <tr>
              <th>操作</th>
              <th>说明</th>
              <th>操作者</th>
              <th>结果</th>
              <th>时间</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!logs.length">
              <td colspan="5" class="py-10 text-center text-[var(--text-quiet)]">还没有操作日志</td>
            </tr>
            <tr v-for="log in logs" :key="log.id">
              <td class="mono text-xs">{{ log.action }}</td>
              <td class="quiet text-xs">{{ log.detail || '—' }}</td>
              <td class="mono text-xs">{{ log.actor_id ? '#' + log.actor_id : '系统' }}</td>
              <td><span :class="log.result === 'ok' ? 'badge-success' : 'badge-danger'">{{ log.result }}</span></td>
              <td class="quiet text-xs">{{ when(log.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </template>
  </section>
</template>
