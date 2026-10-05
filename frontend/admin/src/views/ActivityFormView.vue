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
import { listCoupons } from '../api/coupons'
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

// ── 规则编辑器 ────────────────────────────────────────────────────
// 规则参数在库里是 JSON 字符串，直接让管理员手填既容易写错也看不出
// 每种类型该填什么。这里按规则类型给出专门的输入项，输入后统一序列化成
// 后端需要的 JSON，列表里再用中文摘要回显。
type RuleKind =
  | 'factor_off'
  | 'percent_off'
  | 'amount_off'
  | 'fixed_price'
  | 'full_reduce'
  | 'full_quantity'
  | 'bulk_price'
  | 'coupon_lock'
  | 'gift_coupon'

interface RuleDraft {
  kind: RuleKind
  factor: number
  percent: number
  amount: number
  price: number
  threshold: number
  quantity: number
  perUnit: number
  quantityMode: 'flat' | 'per_unit'
  couponId: number
  useTiers: boolean
  tiersText: string
}

const ruleKindOptions: { value: RuleKind; label: string; hint: string }[] = [
  { value: 'factor_off', label: '折扣系数', hint: '售价 × 系数，例如 0.85 表示 85 折' },
  { value: 'percent_off', label: '百分比折扣', hint: '例如 90 表示 9 折（兼容旧规则）' },
  { value: 'amount_off', label: '立减金额', hint: '每单直接减固定金额' },
  { value: 'fixed_price', label: '固定折后价', hint: '把单价降到指定价格' },
  { value: 'full_reduce', label: '满减', hint: '满 X 元减 Y 元，可配阶梯' },
  { value: 'full_quantity', label: '满件优惠', hint: '满 N 件减固定金额，或满 N 件每件减 X' },
  { value: 'bulk_price', label: '批量购买价', hint: '达到 N 件后按指定单价结算' },
  { value: 'coupon_lock', label: '指定优惠码', hint: '锁定一张已有优惠券参与' },
  { value: 'gift_coupon', label: '赠送优惠券', hint: '下单后赠送一张优惠券' },
]

const ruleKindLabel: Record<string, string> = {}
for (const option of ruleKindOptions) ruleKindLabel[option.value] = option.label

const draft = ref<RuleDraft>({
  kind: 'percent_off',
  factor: 0.9,
  percent: 90,
  amount: 5,
  price: 9.9,
  threshold: 100,
  quantity: 3,
  perUnit: 5,
  quantityMode: 'flat',
  couponId: 0,
  useTiers: false,
  tiersText: '[{"threshold":100,"amount":20}]',
})
const editingIndex = ref(-1)
const coupons = ref<{ id: number; code: string }[]>([])

function parseTiers(text: string): { threshold: number; amount: number }[] {
  let tiers: { threshold?: number; amount?: number }[]
  try {
    tiers = JSON.parse(text || '[]') as { threshold?: number; amount?: number }[]
  } catch {
    throw new Error('阶梯配置不是合法 JSON，请检查格式后再保存')
  }
  if (!Array.isArray(tiers) || tiers.length === 0) throw new Error('阶梯档位至少要有一档')
  return tiers.map((tier) => ({
    threshold: Number(tier.threshold) || 0,
    amount: Number(tier.amount) || 0,
  }))
}

function buildConfig(value: RuleDraft): string {
  switch (value.kind) {
    case 'factor_off':
      return JSON.stringify({ factor: value.factor })
    case 'percent_off':
      return JSON.stringify({ percent: value.percent })
    case 'amount_off':
      return JSON.stringify({ amount: value.amount })
    case 'fixed_price':
      return JSON.stringify({ price: value.price })
    case 'full_reduce':
      if (value.useTiers) return JSON.stringify({ tiers: parseTiers(value.tiersText) })
      return JSON.stringify({ threshold: value.threshold, amount: value.amount })
    case 'full_quantity':
      return value.quantityMode === 'per_unit'
        ? JSON.stringify({ quantity: value.quantity, per_unit: value.perUnit })
        : JSON.stringify({ quantity: value.quantity, amount: value.amount })
    case 'bulk_price':
      return JSON.stringify({ quantity: value.quantity, price: value.price })
    case 'coupon_lock':
    case 'gift_coupon':
      return JSON.stringify({ coupon_id: value.couponId })
    default:
      return '{}'
  }
}

function loadDraft(kind: string, config: string) {
  let parsed: Record<string, unknown> = {}
  try {
    parsed = JSON.parse(config || '{}') as Record<string, unknown>
  } catch {
    // 历史坏配置不能拖垮整个编辑页；按空规则打开，保存时会写成合法 JSON。
    parsed = {}
  }
  const num = (key: string, fallback: number) => {
    const value = Number(parsed[key])
    return Number.isFinite(value) && value !== 0 ? value : fallback
  }
  draft.value.kind = (ruleKindLabel[kind] ? kind : 'percent_off') as RuleKind
  if (parsed.factor !== undefined) draft.value.factor = Number(parsed.factor) || draft.value.factor
  if (parsed.percent !== undefined) draft.value.percent = Number(parsed.percent) || draft.value.percent
  if (parsed.amount !== undefined) draft.value.amount = num('amount', draft.value.amount)
  if (parsed.price !== undefined) draft.value.price = num('price', draft.value.price)
  if (parsed.threshold !== undefined) draft.value.threshold = num('threshold', draft.value.threshold)
  if (parsed.quantity !== undefined) draft.value.quantity = num('quantity', draft.value.quantity)
  draft.value.perUnit = num('per_unit', draft.value.perUnit)
  draft.value.quantityMode = parsed.per_unit !== undefined ? 'per_unit' : 'flat'
  if (parsed.coupon_id !== undefined) draft.value.couponId = Number(parsed.coupon_id) || 0
  if (Array.isArray(parsed.tiers) && parsed.tiers.length) {
    draft.value.useTiers = true
    draft.value.tiersText = JSON.stringify(parsed.tiers)
  } else {
    draft.value.useTiers = false
  }
}

// 规则列表里用中文摘要说明参数，比裸露的 JSON 更容易核对。
function ruleConfigSummary(item: ActivityRule) {
  try {
    const config = JSON.parse(item.config || '{}') as Record<string, number>
    switch (item.rule_type) {
      case 'factor_off':
        return '售价 × ' + config.factor
      case 'percent_off':
        return config.percent + ' 折（' + config.percent + '%）'
      case 'amount_off':
        return '每单减 ' + config.amount + ' NL'
      case 'fixed_price':
        return '折后单价 ' + config.price + ' NL'
      case 'full_reduce': {
        const tiers = (JSON.parse(item.config).tiers ?? []) as { threshold: number; amount: number }[]
        if (tiers.length) return tiers.map((tier) => '满 ' + tier.threshold + ' 减 ' + tier.amount).join('，')
        return '满 ' + config.threshold + ' NL 减 ' + config.amount + ' NL'
      }
      case 'full_quantity':
        return config.per_unit
          ? '满 ' + config.quantity + ' 件每件减 ' + config.per_unit + ' NL'
          : '满 ' + config.quantity + ' 件减 ' + config.amount + ' NL'
      case 'bulk_price':
        return '满 ' + config.quantity + ' 件单价 ' + config.price + ' NL'
      case 'coupon_lock':
        return '指定优惠券 #' + config.coupon_id
      case 'gift_coupon':
        return '赠送优惠券 #' + config.coupon_id
      default:
        return item.config
    }
  } catch {
    return item.config
  }
}

function ruleKindHint(): string {
  return ruleKindOptions.find((option) => option.value === draft.value.kind)?.hint ?? ''
}

function resetDraft() {
  editingIndex.value = -1
  draft.value.useTiers = false
  draft.value.tiersText = '[{"threshold":100,"amount":20}]'
  const kind = (typeRuleKind[form.value.type || ''] || 'percent_off') as RuleKind
  const seed = typeRuleDefaults[form.value.type || ''] || '{}'
  try {
    loadDraft(kind, seed)
  } catch {
    draft.value.kind = kind
  }
  if (draft.value.kind === 'full_quantity') draft.value.quantityMode = 'flat'
}

function addRule() {
  let config = '{}'
  try {
    config = buildConfig(draft.value)
  } catch (err) {
    error.value = err instanceof Error ? err.message : '规则参数不合法。'
    return
  }
  error.value = ''
  const rule: ActivityRule = {
    rule_type: draft.value.kind,
    config,
    is_enabled: true,
    sort_order: editingIndex.value >= 0 ? editingIndex.value : rules.value.length,
  }
  if (editingIndex.value >= 0) rules.value.splice(editingIndex.value, 1, rule)
  else rules.value.push(rule)
  editingIndex.value = -1
}

function editRule(index: number) {
  const item = rules.value[index]
  try {
    loadDraft(item.rule_type, item.config)
    editingIndex.value = index
  } catch {
    error.value = '规则参数不是合法 JSON，无法编辑。'
  }
}

function removeRule(index: number) {
  rules.value.splice(index, 1)
  if (editingIndex.value === index) editingIndex.value = -1
  else if (editingIndex.value > index) editingIndex.value -= 1
}

function applyTypeDefaults() {
  resetDraft()
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


async function load() {
  loading.value = true
  error.value = ''
  try {
    const [productList, categoryList, couponList] = await Promise.all([
      listProducts().catch(() => [] as Product[]),
      listCategories().catch(() => [] as Category[]),
      listCoupons().catch(() => []),
    ])
    products.value = productList
    categories.value = categoryList
    coupons.value = couponList.map((item) => ({ id: item.id, code: item.code }))
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
    let savedID = editingID.value
    if (editingID.value) {
      await updateActivity(editingID.value, payload, rules.value)
    } else {
      const created = await createActivity(payload, rules.value)
      savedID = created.id
      notice.value = '活动已创建。'
      await router.replace('/activities/' + savedID + '/edit')
    }
    notice.value = notice.value || '活动已保存。'
    form.value = { ...(await getActivity(savedID)) }
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
        <h2 class="page-title">{{ editingID ? '编辑活动' : '新建活动' }}</h2>
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
        <div class="rule-editor">
          <div class="rule-editor-head">
            <div>
              <p class="font-semibold">{{ editingIndex >= 0 ? '编辑规则' : '添加规则' }}</p>
              <p class="quiet text-xs">{{ ruleKindHint() }}</p>
            </div>
            <button v-if="editingIndex >= 0" class="btn btn-secondary btn-sm" @click="resetDraft">取消编辑</button>
          </div>

          <div class="grid gap-3 md:grid-cols-3">
            <div :class="draft.kind === 'percent_off' || draft.kind === 'amount_off' ? 'md:col-span-3' : ''">
              <label class="label" for="r-kind">规则类型</label>
              <select id="r-kind" v-model="draft.kind" class="input">
                <option v-for="option in ruleKindOptions" :key="option.value" :value="option.value">
                  {{ option.label }}
                </option>
              </select>
            </div>

            <div v-if="draft.kind === 'factor_off' || draft.kind === 'percent_off'">
              <label class="label" for="r-factor">
                {{ draft.kind === 'factor_off' ? '折扣系数' : '折扣百分比' }}
              </label>
              <input
                v-if="draft.kind === 'factor_off'"
                id="r-factor"
                v-model.number="draft.factor"
                class="input nums"
                type="number"
                min="0.01"
                max="0.99"
                step="0.01"
              />
              <input
                v-else
                id="r-factor"
                v-model.number="draft.percent"
                class="input nums"
                type="number"
                min="1"
                max="99"
                step="1"
              />
              <p class="hint">
                {{ draft.kind === 'factor_off'
                  ? '例如 0.85 表示按售价的 85% 结算，不会随数量放大。'
                  : '例如 90 表示 9 折，仅用于兼容旧规则。' }}
              </p>
            </div>

            <div v-else-if="draft.kind === 'amount_off'">
              <label class="label" for="r-amount">立减金额（元）</label>
              <input id="r-amount" v-model.number="draft.amount" class="input nums" type="number" min="0.01" step="0.01" />
              <p class="hint">每单满足条件即减这个金额。</p>
            </div>

            <div v-else-if="draft.kind === 'fixed_price'">
              <label class="label" for="r-price">折后单价（元）</label>
              <input id="r-price" v-model.number="draft.price" class="input nums" type="number" min="0.01" step="0.01" />
              <p class="hint">必须低于原价，否则规则不会生效。</p>
            </div>

            <div v-else-if="draft.kind === 'full_reduce'" class="md:col-span-3 space-y-3">
              <label class="flex items-center gap-2 text-sm">
                <input v-model="draft.useTiers" type="checkbox" />
                使用多档阶梯
              </label>
              <div v-if="draft.useTiers">
                <label class="label" for="r-tiers">阶梯档位（JSON）</label>
                <textarea id="r-tiers" v-model="draft.tiersText" class="input mono min-h-[80px] text-xs" />
                <p class="hint">每档形如 {"threshold":100,"amount":20}，表示满 100 元减 20 元；命中最高一档生效。</p>
              </div>
              <div v-else class="grid gap-3 md:grid-cols-2">
                <div>
                  <label class="label" for="r-threshold">门槛金额（元）</label>
                  <input id="r-threshold" v-model.number="draft.threshold" class="input nums" type="number" min="0.01" step="0.01" />
                </div>
                <div>
                  <label class="label" for="r-amount2">减免金额（元）</label>
                  <input id="r-amount2" v-model.number="draft.amount" class="input nums" type="number" min="0.01" step="0.01" />
                </div>
              </div>
            </div>

            <div v-else-if="draft.kind === 'full_quantity'" class="md:col-span-3 space-y-3">
              <div class="grid gap-3 md:grid-cols-2">
                <div>
                  <label class="label" for="r-quantity">满件数量</label>
                  <input id="r-quantity" v-model.number="draft.quantity" class="input nums" type="number" min="1" step="1" />
                </div>
                <div>
                  <label class="label">优惠方式</label>
                  <div class="segmented">
                    <button
                      class="segmented-item"
                      :class="draft.quantityMode === 'flat' ? 'segmented-item-on' : ''"
                      type="button"
                      @click="draft.quantityMode = 'flat'"
                    >
                      减固定金额
                    </button>
                    <button
                      class="segmented-item"
                      :class="draft.quantityMode === 'per_unit' ? 'segmented-item-on' : ''"
                      type="button"
                      @click="draft.quantityMode = 'per_unit'"
                    >
                      每件减
                    </button>
                  </div>
                </div>
              </div>
              <div>
                <label class="label" for="r-amount3">
                  {{ draft.quantityMode === 'per_unit' ? '每件减免（元）' : '减免金额（元）' }}
                </label>
                <input
                  v-if="draft.quantityMode === 'per_unit'"
                  id="r-amount3"
                  v-model.number="draft.perUnit"
                  class="input nums"
                  type="number"
                  min="0.01"
                  step="0.01"
                />
                <input
                  v-else
                  id="r-amount3"
                  v-model.number="draft.amount"
                  class="input nums"
                  type="number"
                  min="0.01"
                  step="0.01"
                />
                <p class="hint">
                  {{ draft.quantityMode === 'per_unit'
                    ? '满件后按实际件数每件减，例如满 3 件每件减 5 元。'
                    : '满件后一次性减固定金额，不随件数变化。' }}
                </p>
              </div>
            </div>

            <div v-else-if="draft.kind === 'bulk_price'" class="md:col-span-3 grid gap-3 md:grid-cols-2">
              <div>
                <label class="label" for="r-bulk-qty">达到件数</label>
                <input id="r-bulk-qty" v-model.number="draft.quantity" class="input nums" type="number" min="1" step="1" />
              </div>
              <div>
                <label class="label" for="r-bulk-price">折后单价（元）</label>
                <input id="r-bulk-price" v-model.number="draft.price" class="input nums" type="number" min="0.01" step="0.01" />
              </div>
              <p class="hint md:col-span-2">达到件数后整单按这个单价结算。</p>
            </div>

            <div v-else class="md:col-span-3">
              <label class="label" for="r-coupon">选择优惠券</label>
              <select id="r-coupon" v-model.number="draft.couponId" class="input">
                <option :value="0">请选择优惠券</option>
                <option v-for="item in coupons" :key="item.id" :value="item.id">{{ item.code }}（#{{ item.id }}）</option>
              </select>
              <p class="hint">列表为空时请先到「优惠券」页创建。</p>
            </div>
          </div>

          <button class="btn btn-sm" @click="addRule">{{ editingIndex >= 0 ? '保存修改' : '+ 添加规则' }}</button>
        </div>

        <div v-if="!rules.length" class="card-quiet text-center text-sm text-[var(--text-quiet)]">
          还没有规则。自动应用的活动至少要有一条规则。
        </div>
        <ul v-else class="space-y-2">
          <li v-for="(item, index) in rules" :key="index" class="rule-row">
            <span class="badge">{{ ruleKindLabel[item.rule_type] ?? item.rule_type }}</span>
            <span class="min-w-0 flex-1 text-sm">{{ ruleConfigSummary(item) }}</span>
            <button class="btn btn-secondary btn-sm" @click="editRule(index)">编辑</button>
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

<style scoped>
.rule-editor {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1rem;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-sm);
  background: var(--surface-sunken);
}

.rule-editor-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 0.75rem;
}

.rule-row {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.6rem 0.85rem;
  border: 1px solid var(--stroke-quiet);
  border-radius: var(--radius-sm);
  background: var(--surface-hi);
}

.segmented {
  display: inline-flex;
  padding: 2px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-pill);
  background: var(--surface-sunken);
}

.segmented-item {
  padding: 0.35rem 0.85rem;
  border: 0;
  border-radius: var(--radius-pill);
  background: transparent;
  color: var(--text-quiet);
  font-size: 0.8125rem;
  cursor: pointer;
  transition: background var(--fast), color var(--fast);
}

.segmented-item-on {
  background: var(--surface-hi);
  color: var(--text);
  box-shadow: 0 1px 2px rgb(0 0 0 / 0.08);
}
</style>
