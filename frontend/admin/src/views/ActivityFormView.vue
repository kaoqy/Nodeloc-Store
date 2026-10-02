<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  activityTypeOptions,
  createActivity,
  getActivity,
  updateActivity,
  type Activity,
  type ActivityRule,
} from '../api/activities'
import { listProducts } from '../api/products'
import { listCategories } from '../api/categories'
import { errorMessage } from '../utils/format'
import type { Category, Product } from '../types'

const route = useRoute()
const router = useRouter()
const editingID = computed(() => Number(route.params.id || 0))

const STEPS = [
  { key: 'basic', label: '基础信息' },
  { key: 'type', label: '活动类型' },
  { key: 'rules', label: '活动规则' },
  { key: 'scope', label: '适用范围' },
  { key: 'limit', label: '时间和库存' },
  { key: 'user', label: '用户限制' },
  { key: 'stack', label: '叠加与展示' },
  { key: 'notify', label: '通知设置' },
]

const step = ref('basic')
const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const products = ref<Product[]>([])
const categories = ref<Category[]>([])

const form = ref<Partial<Activity>>({
  name: '',
  subtitle: '',
  description: '',
  type: 'limited_discount',
  status: 'draft',
  user_scope: 'all',
  sort_order: 0,
  stock_limit: 0,
  quota_limit: 0,
  per_user_limit: 0,
  require_login: true,
  allow_stacking: false,
  auto_apply: true,
  show_on_home: true,
  show_in_list: true,
  notify_users: false,
})

const rule = ref<ActivityRule>({ rule_type: 'percent_off', config: '{"percent":90}', is_enabled: true, sort_order: 0 })
const rules = ref<ActivityRule[]>([])

// 不同活动类型默认给出对应的规则，减少管理员手填 JSON 的次数。
const typeRuleDefaults: Record<string, string> = {
  limited_discount: '{"percent":90}',
  store_discount: '{"percent":95}',
  product_discount: '{"percent":90}',
  category_discount: '{"percent":90}',
  full_reduction: '{"tiers":[{"threshold":100,"amount":20}]}',
  full_quantity: '{"quantity":3,"amount":5}',
  bulk_discount: '{"quantity":5,"price":10}',
  coupon_activity: '{"coupon_id":0}',
  new_user: '{"amount":5}',
  first_purchase: '{"amount":5}',
  member_only: '{"percent":95}',
  limited_activity: '{"percent":90}',
  seckill: '{"price":1}',
  coupon_claim: '{"coupon_id":0}',
}

const typeRuleKind: Record<string, string> = {
  limited_discount: 'percent_off',
  store_discount: 'percent_off',
  product_discount: 'percent_off',
  category_discount: 'percent_off',
  full_reduction: 'full_reduce',
  full_quantity: 'full_quantity',
  bulk_discount: 'bulk_price',
  coupon_activity: 'coupon_lock',
  new_user: 'amount_off',
  first_purchase: 'amount_off',
  member_only: 'percent_off',
  limited_activity: 'percent_off',
  seckill: 'fixed_price',
  coupon_claim: 'gift_coupon',
}

const selectedProducts = computed({
  get: () => (form.value.product_ids ? (JSON.parse(form.value.product_ids) as number[]) : []),
  set: (value: number[]) => {
    form.value.product_ids = JSON.stringify(value)
  },
})

const selectedCategories = computed({
  get: () => (form.value.category_ids ? (JSON.parse(form.value.category_ids) as number[]) : []),
  set: (value: number[]) => {
    form.value.category_ids = JSON.stringify(value)
  },
})

function toggleProduct(id: number) {
  const next = new Set(selectedProducts.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedProducts.value = Array.from(next)
}

function toggleCategory(id: number) {
  const next = new Set(selectedCategories.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedCategories.value = Array.from(next)
}

function ruleConfigLabel(item: ActivityRule) {
  return item.rule_type + '：' + (item.config || '{}')
}

function addRule() {
  rules.value.push({ ...rule.value, sort_order: rules.value.length })
  rule.value.config = typeRuleDefaults[form.value.type || 'limited_discount'] || '{}'
  rule.value.rule_type = typeRuleKind[form.value.type || 'limited_discount'] || 'percent_off'
}

function removeRule(index: number) {
  rules.value.splice(index, 1)
}

function applyTypeDefaults() {
  rule.value.rule_type = typeRuleKind[form.value.type || ''] || 'percent_off'
  rule.value.config = typeRuleDefaults[form.value.type || ''] || '{}'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [productList, categoryList] = await Promise.all([
      listProducts().catch(() => [] as Product[]),
      listCategories().catch(() => [] as Category[]),
    ])
    products.value = productList
    categories.value = categoryList
    if (editingID.value) {
      const activity = await getActivity(editingID.value)
      form.value = { ...activity }
      rules.value = activity.rule_list ?? []
    }
    applyTypeDefaults()
  } catch (err) {
    error.value = errorMessage(err, '加载活动失败')
  } finally {
    loading.value = false
  }
}

function validate(): string {
  if (!form.value.name || !form.value.name.trim()) return '请填写活动名称。'
  if (!form.value.type) return '请选择活动类型。'
  if (form.value.start_at && form.value.end_at && new Date(form.value.end_at) <= new Date(form.value.start_at)) {
    return '结束时间必须晚于开始时间。'
  }
  if (form.value.auto_apply && rules.value.length === 0) return '自动应用的活动至少要配一条规则。'
  for (const item of rules.value) {
    try {
      JSON.parse(item.config || '{}')
    } catch {
      return '规则「' + item.rule_type + '」的参数不是合法 JSON。'
    }
  }
  return ''
}

async function save(nextStatus?: string) {
  const complaint = validate()
  if (complaint) {
    error.value = complaint
    return
  }
  saving.value = true
  error.value = ''
  notice.value = ''
  try {
    const payload: Partial<Activity> = { ...form.value }
    if (nextStatus) payload.status = nextStatus
    if (editingID.value) {
      await updateActivity(editingID.value, payload, rules.value)
    } else {
      const created = await createActivity(payload, rules.value)
      notice.value = '活动已创建。'
      await router.replace('/activities/' + created.id + '/edit')
    }
    notice.value = notice.value || '活动已保存。'
    form.value = { ...(await getActivity(editingID.value || Number(route.params.id))) }
    rules.value = form.value.rule_list ?? []
  } catch (err) {
    error.value = errorMessage(err, '保存活动失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-lg font-bold">{{ editingID ? '编辑活动' : '新建活动' }}</h2>
        <p class="quiet mt-1 text-xs">规则由后端计算，买家端看到的价格一律以服务端为准。</p>
      </div>
      <div class="flex gap-2">
        <RouterLink to="/activities" class="btn btn-secondary btn-sm">返回列表</RouterLink>
        <button class="btn btn-secondary btn-sm" :disabled="saving" @click="save('draft')">保存草稿</button>
        <button class="btn btn-primary btn-sm" :disabled="saving" @click="save('running')">
          {{ saving ? '保存中…' : '保存并上线' }}
        </button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div class="flex flex-wrap gap-1.5">
      <button
        v-for="item in STEPS"
        :key="item.key"
        class="chip"
        :class="step === item.key ? 'chip-active' : ''"
        @click="step = item.key"
      >
        {{ item.label }}
      </button>
    </div>

    <div v-if="loading" class="card space-y-3">
      <div v-for="i in 5" :key="i" class="skeleton h-9 w-full" />
    </div>

    <div v-else class="card space-y-5">
      <template v-if="step === 'basic'">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="label" for="a-name">活动名称</label>
            <input id="a-name" v-model="form.name" class="input" maxlength="80" placeholder="例如 国庆限时 9 折" />
          </div>
          <div>
            <label class="label" for="a-subtitle">活动副标题</label>
            <input id="a-subtitle" v-model="form.subtitle" class="input" maxlength="120" placeholder="列表页显示的一句话" />
          </div>
        </div>
        <div>
          <label class="label" for="a-desc">活动描述</label>
          <textarea id="a-desc" v-model="form.description" class="input min-h-[120px]" placeholder="规则说明、注意事项" />
        </div>
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="label" for="a-cover">活动封面</label>
            <input id="a-cover" v-model="form.cover_image" class="input mono text-xs" placeholder="/uploads/…" />
          </div>
          <div>
            <label class="label" for="a-banner">活动 Banner</label>
            <input id="a-banner" v-model="form.banner_image" class="input mono text-xs" placeholder="/uploads/…" />
          </div>
        </div>
      </template>

      <template v-else-if="step === 'type'">
        <div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          <button
            v-for="option in activityTypeOptions"
            :key="option.value"
            class="card-quiet text-left transition-colors"
            :class="form.type === option.value ? 'border-[var(--accent-line)] accent-text' : ''"
            @click="form.type = option.value; applyTypeDefaults()"
          >
            <p class="font-semibold">{{ option.label }}</p>
            <p class="quiet mt-1 text-xs">{{ option.value }}</p>
          </button>
        </div>
        <p class="quiet text-xs">切换类型会更新默认规则；已添加的规则需要自行核对。</p>
      </template>

      <template v-else-if="step === 'rules'">
        <div class="grid gap-3 md:grid-cols-3">
          <div>
            <label class="label" for="r-type">规则类型</label>
            <select id="r-type" v-model="rule.rule_type" class="input">
              <option value="percent_off">百分比折扣</option>
              <option value="amount_off">立减金额</option>
              <option value="fixed_price">固定折后价</option>
              <option value="full_reduce">满减</option>
              <option value="full_quantity">满件优惠</option>
              <option value="bulk_price">批量购买价</option>
              <option value="coupon_lock">指定优惠码</option>
              <option value="gift_coupon">赠送优惠券</option>
            </select>
          </div>
          <div class="md:col-span-2">
            <label class="label" for="r-config">规则参数（JSON）</label>
            <input id="r-config" v-model="rule.config" class="input mono text-xs" placeholder='{"percent":90}' />
          </div>
        </div>
        <button class="btn btn-secondary btn-sm" @click="addRule">+ 添加规则</button>

        <div v-if="!rules.length" class="card-quiet text-center text-sm text-[var(--text-quiet)]">
          还没有规则。自动应用的活动至少要有一条规则。
        </div>
        <ul v-else class="space-y-2">
          <li v-for="(item, index) in rules" :key="index" class="card-quiet flex items-center gap-3">
            <span class="badge">{{ item.rule_type }}</span>
            <span class="mono min-w-0 flex-1 truncate text-xs">{{ item.config }}</span>
            <button class="btn btn-danger btn-sm" @click="removeRule(index)">删除</button>
          </li>
        </ul>
      </template>

      <template v-else-if="step === 'scope'">
        <div>
          <p class="label">适用商品（不选表示不限）</p>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="product in products"
              :key="product.id"
              class="chip"
              :class="selectedProducts.includes(product.id) ? 'chip-active' : ''"
              @click="toggleProduct(product.id)"
            >
              {{ product.name }}
            </button>
          </div>
        </div>
        <div>
          <p class="label">适用分类（不选表示不限）</p>
          <div class="flex flex-wrap gap-1.5">
            <button
              v-for="category in categories"
              :key="category.id"
              class="chip"
              :class="selectedCategories.includes(category.id) ? 'chip-active' : ''"
              @click="toggleCategory(category.id)"
            >
              {{ category.name }}
            </button>
          </div>
        </div>
      </template>

      <template v-else-if="step === 'limit'">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="label" for="a-start">开始时间</label>
            <input id="a-start" v-model="form.start_at" class="input" type="datetime-local" />
          </div>
          <div>
            <label class="label" for="a-end">结束时间</label>
            <input id="a-end" v-model="form.end_at" class="input" type="datetime-local" />
          </div>
          <div>
            <label class="label" for="a-stock">活动库存（0 表示不限）</label>
            <input id="a-stock" v-model.number="form.stock_limit" class="input nums" type="number" min="0" />
          </div>
          <div>
            <label class="label" for="a-quota">活动名额（0 表示不限）</label>
            <input id="a-quota" v-model.number="form.quota_limit" class="input nums" type="number" min="0" />
          </div>
          <div>
            <label class="label" for="a-peruser">每人参与次数（0 表示不限）</label>
            <input id="a-peruser" v-model.number="form.per_user_limit" class="input nums" type="number" min="0" />
          </div>
          <div>
            <label class="label" for="a-sort">活动排序</label>
            <input id="a-sort" v-model.number="form.sort_order" class="input nums" type="number" />
          </div>
        </div>
      </template>

      <template v-else-if="step === 'user'">
        <div class="grid gap-4 md:grid-cols-2">
          <div>
            <label class="label" for="a-userscope">适用人群</label>
            <select id="a-userscope" v-model="form.user_scope" class="input">
              <option value="all">所有用户</option>
              <option value="new_user">新用户</option>
              <option value="first_purchase">首次购买</option>
              <option value="member">登录会员</option>
              <option value="role">指定角色</option>
            </select>
          </div>
          <div v-if="form.user_scope === 'role'">
            <label class="label" for="a-role">角色标识</label>
            <input id="a-role" v-model="form.user_role_scope" class="input mono" placeholder="例如 support" />
          </div>
        </div>
        <label class="flex items-center gap-2 text-sm">
          <input v-model="form.require_login" type="checkbox" />
          需要登录后才能参与
        </label>
      </template>

      <template v-else-if="step === 'stack'">
        <label class="flex items-center gap-2 text-sm">
          <input v-model="form.auto_apply" type="checkbox" />
          自动应用到符合条件的订单
        </label>
        <label class="flex items-center gap-2 text-sm">
          <input v-model="form.allow_stacking" type="checkbox" />
          允许与其他规则叠加（默认取优惠最大的一条活动）
        </label>
        <label class="flex items-center gap-2 text-sm">
          <input v-model="form.show_on_home" type="checkbox" />
          在首页展示
        </label>
        <label class="flex items-center gap-2 text-sm">
          <input v-model="form.show_in_list" type="checkbox" />
          在活动中心展示
        </label>
      </template>

      <template v-else>
        <label class="flex items-center gap-2 text-sm">
          <input v-model="form.notify_users" type="checkbox" />
          活动开始时通知用户
        </label>
        <div>
          <label class="label" for="a-notify">通知文案</label>
          <textarea id="a-notify" v-model="form.notify_text" class="input min-h-[100px]" />
        </div>
        <div>
          <label class="label" for="a-remark">活动备注（仅后台可见）</label>
          <textarea id="a-remark" v-model="form.remark" class="input min-h-[80px]" />
        </div>
      </template>
    </div>
  </section>
</template>
