<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import PaginationFooter from '../components/PaginationFooter.vue'
import {
  activityStatusOptions,
  activityTypeOptions,
  deleteActivity,
  duplicateActivity,
  getActivityOverview,
  listActivities,
  setActivityStatus,
  type Activity,
  type ActivityOverview,
} from '../api/activities'
import { errorMessage, money, when } from '../utils/format'
import { useAuthStore } from '../stores/auth'

const PageSize = 12

const auth = useAuthStore()
const canManage = computed(() => auth.allows('activities', 'manage'))
const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const rows = ref<Activity[]>([])
const total = ref(0)
const page = ref(1)
const search = ref('')
const status = ref('all')
const type = ref('all')
const sort = ref('sort_order')
const overview = ref<ActivityOverview>({ total: 0, running: 0, participants: 0, discounts: 0 })

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PageSize)))

const statusTone: Record<string, string> = {
  draft: 'badge',
  scheduled: 'badge-info',
  running: 'badge-success',
  paused: 'badge-warning',
  ended: 'badge',
  offline: 'badge',
  deleted: 'badge-danger',
}

const statusLabel = (value: string) =>
  activityStatusOptions.find((option) => option.value === value)?.label ?? value

const typeLabel = (value: string) =>
  activityTypeOptions.find((option) => option.value === value)?.label ?? value

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [list, stats] = await Promise.all([
      listActivities({
        status: status.value,
        type: type.value,
        q: search.value,
        sort: sort.value,
        limit: PageSize,
        offset: (page.value - 1) * PageSize,
      }),
      getActivityOverview().catch(() => ({ total: 0, running: 0, participants: 0, discounts: 0 })),
    ])
    rows.value = list.data
    total.value = list.total
    overview.value = stats
  } catch (err) {
    error.value = errorMessage(err, '加载活动失败')
  } finally {
    loading.value = false
  }
}

function applyFilters() {
  page.value = 1
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

async function toggleStatus(activity: Activity) {
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const next = activity.status === 'running' ? 'paused' : 'running'
    await setActivityStatus(activity.id, next, next === 'paused' ? '管理员暂停活动' : '管理员恢复活动')
    notice.value = next === 'paused' ? '活动已暂停。' : '活动已恢复。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '更新活动状态失败')
  } finally {
    busy.value = false
  }
}

async function takeOffline(activity: Activity) {
  if (!confirm('下架活动「' + activity.name + '」？已产生的订单与优惠记录会保留。')) return
  busy.value = true
  try {
    await setActivityStatus(activity.id, 'offline', '管理员下架')
    notice.value = '活动已下架。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '下架活动失败')
  } finally {
    busy.value = false
  }
}

async function copy(activity: Activity) {
  busy.value = true
  try {
    await duplicateActivity(activity.id)
    notice.value = '已复制为草稿。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '复制活动失败')
  } finally {
    busy.value = false
  }
}

async function remove(activity: Activity) {
  if (!confirm('删除活动「' + activity.name + '」？历史订单快照会保留。')) return
  busy.value = true
  try {
    await deleteActivity(activity.id)
    notice.value = '活动已删除。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除活动失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card !p-5">
        <p class="eyebrow">活动总数</p>
        <p class="nums mt-2 text-2xl font-bold">{{ overview.total }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">进行中</p>
        <p class="nums mt-2 text-2xl font-bold accent-text">{{ overview.running }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">参与人数</p>
        <p class="nums mt-2 text-2xl font-bold">{{ overview.participants }}</p>
      </div>
      <div class="card !p-5">
        <p class="eyebrow">累计优惠</p>
        <p class="nums mt-2 text-2xl font-bold">{{ money(overview.discounts) }}</p>
      </div>
    </div>

    <div class="flex flex-wrap items-center gap-2">
      <input
        v-model="search"
        class="input w-56"
        type="search"
        placeholder="活动名称 / 副标题"
        aria-label="搜索活动"
        @keyup.enter="applyFilters"
      />
      <select v-model="status" class="input !w-auto" aria-label="活动状态" @change="applyFilters">
        <option value="all">全部状态</option>
        <option v-for="option in activityStatusOptions" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
      <select v-model="type" class="input !w-auto" aria-label="活动类型" @change="applyFilters">
        <option value="all">全部类型</option>
        <option v-for="option in activityTypeOptions" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
      <select v-model="sort" class="input !w-auto" aria-label="排序" @change="applyFilters">
        <option value="sort_order">按排序值</option>
        <option value="newest">最新创建</option>
        <option value="name">按名称</option>
      </select>
      <button class="btn btn-secondary btn-sm" :disabled="loading" @click="applyFilters">查询</button>
      <RouterLink v-if="canManage" to="/activities/new" class="btn btn-primary btn-sm ml-auto">+ 新建活动</RouterLink>
      <p v-else class="quiet ml-auto text-xs">当前角色只能查看活动，编辑需要「活动营销」管理权限。</p>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 4" :key="i" class="skeleton h-10 w-full" />
    </div>

    <div v-else-if="!rows.length" class="card py-16 text-center">
      <p class="font-semibold">还没有活动</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">创建限时折扣、满减或优惠券活动后，买家会在活动中心看到它们。</p>
      <RouterLink v-if="canManage" to="/activities/new" class="btn btn-primary btn-sm mt-6">新建活动</RouterLink>
    </div>

    <div v-else class="table-container">
      <table class="table">
        <thead>
          <tr>
            <th>活动</th>
            <th>类型</th>
            <th>状态</th>
            <th class="nums">优惠金额</th>
            <th class="nums">参与</th>
            <th>时间</th>
            <th class="text-right">操作</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="activity in rows" :key="activity.id">
            <td>
              <RouterLink :to="'/activities/' + activity.id + '/edit'" class="font-semibold hover:accent-text">
                {{ activity.name }}
              </RouterLink>
              <p v-if="activity.subtitle" class="quiet mt-0.5 text-xs">{{ activity.subtitle }}</p>
            </td>
            <td><span class="badge">{{ typeLabel(activity.type) }}</span></td>
            <td><span :class="statusTone[activity.status] || 'badge'">{{ statusLabel(activity.status) }}</span></td>
            <td class="nums">{{ money(activity.discount_total || 0) }}</td>
            <td class="nums">{{ activity.user_count || 0 }}</td>
            <td class="quiet text-xs">
              <span v-if="activity.start_at">开始 {{ when(activity.start_at) }}</span>
              <span v-else>不限开始</span>
              <br />
              <span v-if="activity.end_at">结束 {{ when(activity.end_at) }}</span>
              <span v-else>不限结束</span>
            </td>
            <td class="text-right">
              <div class="flex flex-wrap justify-end gap-1.5">
                <RouterLink :to="'/activities/' + activity.id" class="btn btn-quiet btn-sm">数据</RouterLink>
                <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="toggleStatus(activity)">
                  {{ activity.status === 'running' ? '暂停' : '上线' }}
                </button>
                <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="copy(activity)">复制</button>
                <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="takeOffline(activity)">下架</button>
                <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="remove(activity)">删除</button>
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <PaginationFooter :page="page" :pages="pageCount" :loading="loading" :summary="'共 ' + total + ' 个活动'" @change="goPage" />
  </section>
</template>
