<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AdminIcon from '../components/AdminIcon.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import FilterBar from '../components/FilterBar.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
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

/**
 * 活动营销。列表给出每个活动的参与与优惠总额；上下架、暂停、复制都在行内完成。
 */

const PAGE_SIZE = 15

const auth = useAuthStore()
const canManage = computed(() => auth.allows('activities', 'manage'))

const COLUMNS: Column[] = [
  { label: '活动' },
  { label: '类型', hideOnMobile: true },
  { label: '状态' },
  { label: '优惠金额', numeric: true },
  { label: '参与', numeric: true, hideOnMobile: true },
  { label: '有效期', hideOnMobile: true },
  { label: '', actions: true, width: '230px' },
]

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
const overview = ref<ActivityOverview | null>(null)

const pageCount = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))
const filtered = computed(() => Boolean(search.value.trim() || status.value !== 'all' || type.value !== 'all'))

const typeLabel = (value: string) => activityTypeOptions.find((o) => o.value === value)?.label ?? value

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
        limit: PAGE_SIZE,
        offset: (page.value - 1) * PAGE_SIZE,
      }),
      getActivityOverview().catch(() => null),
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

function apply() {
  page.value = 1
  void load()
}

function goPage(next: number) {
  if (next < 1 || next > pageCount.value || next === page.value) return
  page.value = next
  void load()
}

function clearFilters() {
  search.value = ''
  status.value = 'all'
  type.value = 'all'
  apply()
}

async function changeStatus(activity: Activity, next: string, reason = '') {
  if (busy.value) return
  if (next === 'offline' && !window.confirm('下架活动「' + activity.name + '」？已产生的订单与优惠记录会保留。')) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    await setActivityStatus(activity.id, next, reason)
    const label = activityStatusOptions.find((o) => o.value === next)?.label ?? next
    notice.value = '活动「' + activity.name + '」已' + label + '。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '更新活动状态失败')
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
  if (!window.confirm('删除活动「' + activity.name + '」？历史订单快照会保留。')) return
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
    <PageHeader
      title="活动营销"
      description="限时折扣、满减、领券等活动在这里创建。下单金额一律由服务端按规则计算。"
      bordered
    >
      <template #actions>
        <RouterLink v-if="canManage" to="/activities/new" class="btn btn-primary btn-sm">
          <AdminIcon name="plus" :size="14" />
          新建活动
        </RouterLink>
      </template>
    </PageHeader>

    <div class="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
      <div class="card-quiet">
        <p class="quiet text-xs">活动总数</p>
        <p class="nums mt-1 text-xl font-bold">{{ overview?.total ?? 0 }}</p>
      </div>
      <div class="card-quiet">
        <p class="quiet text-xs">进行中</p>
        <p class="nums accent-text mt-1 text-xl font-bold">{{ overview?.running ?? 0 }}</p>
      </div>
      <div class="card-quiet">
        <p class="quiet text-xs">参与人数</p>
        <p class="nums mt-1 text-xl font-bold">{{ overview?.participants ?? 0 }}</p>
      </div>
      <div class="card-quiet">
        <p class="quiet text-xs">累计优惠</p>
        <p class="nums mt-1 text-xl font-bold">{{ money(overview?.discounts ?? 0) }}</p>
      </div>
    </div>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <FilterBar :count="total ? '共 ' + total + ' 个活动' : ''">
      <input v-model="search" class="input w-52" type="search" placeholder="活动名称 / 副标题" aria-label="搜索活动" @keyup.enter="apply" />
      <select v-model="status" class="input !w-auto" aria-label="活动状态" @change="apply">
        <option value="all">全部状态</option>
        <option v-for="option in activityStatusOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <select v-model="type" class="input !w-auto" aria-label="活动类型" @change="apply">
        <option value="all">全部类型</option>
        <option v-for="option in activityTypeOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
      </select>
      <select v-model="sort" class="input !w-auto" aria-label="排序" @change="apply">
        <option value="sort_order">按排序值</option>
        <option value="newest">最新创建</option>
        <option value="name">按名称</option>
      </select>
      <button class="btn btn-secondary btn-sm" @click="apply">查询</button>
      <template #actions>
        <button v-if="filtered" class="btn btn-quiet btn-sm" @click="clearFilters">清除筛选</button>
      </template>
    </FilterBar>

    <DataTable
      :columns="COLUMNS"
      :loading="loading"
      :error="error"
      :filtered="filtered"
      :page="page"
      :pages="pageCount"
      :total="total"
      :summary="'共 ' + total + ' 个活动'"
      empty-title="还没有活动"
      empty-hint="创建折扣或满减活动后，买家会在活动中心看到它们。"
      @retry="load"
      @clear-filters="clearFilters"
      @change="goPage"
    >
      <tr v-for="activity in rows" :key="activity.id">
        <td>
          <RouterLink :to="'/activities/' + activity.id" class="font-semibold hover:accent-text">{{ activity.name }}</RouterLink>
          <p v-if="activity.subtitle" class="quiet mt-0.5 truncate text-xs">{{ activity.subtitle }}</p>
        </td>
        <td class="hide-on-mobile">{{ typeLabel(activity.type) }}</td>
        <td><StatusBadge :value="activity.status" :label="activityStatusOptions.find((o) => o.value === activity.status)?.label" /></td>
        <td class="nums">{{ money(activity.discount_total ?? 0) }}</td>
        <td class="nums hide-on-mobile">{{ activity.user_count ?? 0 }}</td>
        <td class="quiet hide-on-mobile text-xs">
          <span>{{ activity.start_at ? when(activity.start_at) : '不限开始' }}</span><br />
          <span>{{ activity.end_at ? when(activity.end_at) : '不限结束' }}</span>
        </td>
        <td class="text-right">
          <div class="flex flex-wrap justify-end gap-1.5">
            <RouterLink :to="'/activities/' + activity.id" class="btn btn-quiet btn-sm">数据</RouterLink>
            <RouterLink v-if="canManage" :to="'/activities/' + activity.id + '/edit'" class="btn btn-quiet btn-sm">编辑</RouterLink>
            <button
              v-if="canManage"
              class="btn btn-quiet btn-sm"
              :disabled="busy"
              @click="changeStatus(activity, activity.status === 'running' ? 'paused' : 'running', activity.status === 'running' ? '管理员暂停活动' : '管理员恢复活动')"
            >
              {{ activity.status === 'running' ? '暂停' : '上线' }}
            </button>
            <button v-if="canManage" class="btn btn-quiet btn-sm" :disabled="busy" @click="copy(activity)">复制</button>
            <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="remove(activity)">删除</button>
          </div>
        </td>
      </tr>
    </DataTable>
  </section>
</template>
