<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import WorkbenchHeader from '../components/WorkbenchHeader.vue'
import {
  createProduct,
  getProduct,
  updateProduct,
} from '../api/products'
import { listCategories } from '../api/categories'
import ImageField from '../components/ImageField.vue'
import { errorMessage } from '../utils/format'
import type { Category, Product, ProductFormField } from '../types'

const route = useRoute()
const router = useRouter()

const productId = computed(() => Number(route.params.id || 0))
const isEdit = computed(() => productId.value > 0)

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const notice = ref('')
const categories = ref<Category[]>([])

const form = reactive({
  name: '',
  slug: '',
  summary: '',
  description: '',
  image_path: '',
  delivery_instructions: '',
  product_type: 'card',
  delivery_channel: 'card',
  min_topup_amount: 0,
  max_topup_amount: 0,
  price: 0,
  original_price: null as number | null,
  stock_count: 0,
  stock_visible: true,
  require_contact: false,
  is_published: true,
  is_featured: false,
  is_archived: false,
  sort_order: 0,
  category_id: null as number | null,
  form_schema: [] as ProductFormField[],
})

const isCard = computed(() => form.product_type === 'card')
const isNewAPI = computed(() => form.delivery_channel === 'new_api')
const isManual = computed(() => !isCard.value && !isNewAPI.value)
const invalid = computed(() => {
  if (!form.name.trim() || !form.slug.trim()) return true
  // New-API products are amount-type goods: they are configured with a top-up
  // range only, so they have no product price to require here. Every other
  // channel still charges through the existing price × quantity rule.
  if (isNewAPI.value) return false
  return !Number.isInteger(Number(form.price)) || Number(form.price) <= 0
})
const priceError = computed(() => {
  if (isNewAPI.value) return ''
  if (!form.name.trim() || !form.slug.trim()) return ''
  if (!Number.isInteger(Number(form.price)) || Number(form.price) <= 0) {
    return '请填写大于 0 的整数售价（NL）；下单金额按“售价 × 数量”由服务端计算。'
  }
  return ''
})
const topupError = computed(() => {
  if (!isNewAPI.value) return ''
  const min = Number(form.min_topup_amount)
  const max = Number(form.max_topup_amount)
  if (!Number.isInteger(min) || min <= 0) return '请填写单次最少充值额度，且为正整数。'
  if (!Number.isInteger(max) || max <= 0) return '请填写单次最多充值额度，且为正整数。'
  if (min > max) return '单次最少充值额度不能大于最多充值额度。'
  return ''
})
const formInvalid = computed(() => invalid.value || Boolean(topupError.value))
const invalidPriceHint = computed(() =>
  form.original_price !== null &&
  form.original_price !== undefined &&
  Number(form.original_price) > 0 &&
  Number(form.original_price) < Number(form.price)
    ? '划线原价低于当前售价，前台会显示成“涨价”；请确认价格顺序。'
    : '',
)

// The update endpoint replaces the whole row, so only these fields are sent back.
function payload(): Partial<Product> {
  return {
    slug: form.slug.trim(),
    name: form.name.trim(),
    summary: form.summary.trim(),
    description: form.description,
    image_path: form.image_path.trim(),
    product_type: form.product_type,
    delivery_channel: form.delivery_channel,
    min_topup_amount: isNewAPI.value ? Number(form.min_topup_amount) || 0 : 0,
    max_topup_amount: isNewAPI.value ? Number(form.max_topup_amount) || 0 : 0,
    delivery_instructions: form.delivery_instructions,
    require_contact: form.require_contact,
    // New-API is amount-type and carries no product price; other channels keep
    // the existing price × quantity rule.
    price: isNewAPI.value ? 0 : Number(form.price) || 0,
    original_price: form.original_price === null || Number.isNaN(Number(form.original_price))
      ? null
      : isNewAPI.value
        ? null
        : Number(form.original_price),
    stock_visible: form.stock_visible,
    stock_count: form.stock_count,
    is_published: form.is_published,
    is_featured: form.is_featured,
    is_archived: form.is_archived,
    sort_order: Number(form.sort_order) || 0,
    category_id: form.category_id,
    // New-API buyers only enter the top-up amount, which the provider
    // contributes; a product-level form would add inputs this channel must not
    // ask for. The in-memory list is kept so switching back to card/manual
    // restores the fields that were already configured.
    form_schema: isNewAPI.value
      ? '[]'
      : JSON.stringify(form.form_schema.filter((field) => field.key.trim() && field.label.trim())),
  }
}

function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^\w一-龥]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

function defaultChannel(productType: string): string {
  return productType === 'manual' ? 'manual' : 'card'
}

/**
 * Picking a channel keeps the two representations consistent: the channel is
 * saved, and product_type mirrors it for the existing stock/card logic. Values
 * that do not apply to the new channel are kept in the form so switching back
 * and forth does not erase what the operator already typed.
 */
function chooseChannel(channel: string) {
  form.delivery_channel = channel
  if (channel === 'new_api') {
    form.product_type = 'manual'
  } else {
    form.product_type = channel
  }
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    categories.value = await listCategories()
    if (isEdit.value) {
      const product = await getProduct(productId.value)
      let formFields: ProductFormField[] = []
      if (product.form_schema) {
        try {
          const parsed = JSON.parse(product.form_schema) as unknown
          if (Array.isArray(parsed)) formFields = parsed as ProductFormField[]
          else error.value = '原购买表单不是有效的字段列表，已按空表单打开；保存前请重新核对。'
        } catch {
          error.value = '原购买表单数据无法解析，已按空表单打开；保存会覆盖这组字段，请重新填写。'
        }
      }
      Object.assign(form, {
        name: product.name,
        slug: product.slug,
        summary: product.summary || '',
        description: product.description || '',
        image_path: product.image_path || '',
        delivery_instructions: product.delivery_instructions || '',
        product_type: product.product_type,
        delivery_channel: product.delivery_channel || defaultChannel(product.product_type),
        min_topup_amount: product.min_topup_amount ?? 0,
        max_topup_amount: product.max_topup_amount ?? 0,
        price: product.price,
        original_price: product.original_price ?? null,
        stock_count: product.stock_count,
        stock_visible: product.stock_visible ?? true,
        require_contact: product.require_contact ?? false,
        is_published: product.is_published,
        is_featured: product.is_featured ?? false,
        is_archived: product.is_archived ?? false,
        sort_order: product.sort_order ?? 0,
        category_id: product.category_id ?? null,
        form_schema: formFields,
      })
    }
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (formInvalid.value) {
    if (topupError.value) error.value = topupError.value
    else if (priceError.value) error.value = priceError.value
    return
  }
  if (invalidPriceHint.value) {
    error.value = invalidPriceHint.value
    return
  }
  saving.value = true
  error.value = ''
  try {
    if (isEdit.value) {
      await updateProduct(productId.value, payload())
    } else {
      await createProduct(payload())
    }
    router.push('/products')
  } catch (err) {
    error.value = errorMessage(err, isEdit.value ? '保存失败' : '创建失败')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-48" />
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="skeleton h-96 lg:col-span-2" />
      <div class="skeleton h-96" />
    </div>
  </section>

  <section v-else class="workbench-page">
    <WorkbenchHeader
      :title="isEdit ? '编辑商品' : '新建商品'"
      description="按基本信息、发货渠道、价格与购买设置维护商品。"
      eyebrow="商品管理"
      back-to="/products"
      back-label="返回商品列表"
    >
      <template #actions>
        <button class="btn btn-secondary btn-sm" @click="router.push('/products')">取消</button>
        <button class="btn btn-primary btn-sm" :disabled="saving || formInvalid" @click="save">
          {{ saving ? '保存中…' : '保存商品' }}
        </button>
      </template>
    </WorkbenchHeader>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-else-if="invalidPriceHint" class="alert alert-warning" role="status">{{ invalidPriceHint }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div class="grid gap-5 lg:grid-cols-3">
      <div class="space-y-5 lg:col-span-2">
        <div class="card">
          <h3 class="mb-4 text-sm font-semibold">基本信息</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="p-name">商品名称 *</label>
              <input
                id="p-name"
                v-model="form.name"
                class="input"
                placeholder="输入商品名称"
                @blur="!form.slug && (form.slug = slugify(form.name))"
              />
            </div>
            <div>
              <label class="label" for="p-slug">URL 别名 (slug) *</label>
              <input id="p-slug" v-model="form.slug" class="input mono text-xs" placeholder="product-slug" />
              <p class="hint mt-1.5">买家端地址：/products/{{ form.slug || 'slug' }}</p>
            </div>
            <div>
              <label class="label" for="p-category">分类</label>
              <select id="p-category" v-model="form.category_id" class="input">
                <option :value="null">未分类</option>
                <option v-for="item in categories" :key="item.id" :value="item.id">{{ item.name }}</option>
              </select>
            </div>
            <div class="sm:col-span-2">
              <label class="label" for="p-summary">简介</label>
              <input id="p-summary" v-model="form.summary" class="input" placeholder="列表里显示的一句话描述" />
            </div>
            <div class="sm:col-span-2">
              <label class="label" for="p-desc">详细描述</label>
              <textarea id="p-desc" v-model="form.description" class="input h-32 resize-none" />
            </div>
          </div>
        </div>

        <div class="card">
          <div class="mb-4">
            <h3 class="text-sm font-semibold">发货渠道</h3>
            <p class="hint mt-1">决定买家付款后商品如何交付。选择 New-API 时为整件商品创建兑换码，不占用卡密库存。</p>
          </div>
          <div class="grid gap-3 sm:grid-cols-3">
            <button
              v-for="option in [
                { value: 'card', title: '卡密自动发货', desc: '导入卡密库存，付款后自动发放一条。' },
                { value: 'manual', title: '商家人工发货', desc: '付款后进入人工待发货队列。' },
                { value: 'new_api', title: 'New-API 兑换码', desc: '付款后按充值额度创建兑换码并交付。' },
              ]"
              :key="option.value"
              type="button"
              class="channel-card text-left"
              :class="{ 'channel-card-active': form.delivery_channel === option.value }"
              :aria-pressed="form.delivery_channel === option.value"
              @click="chooseChannel(option.value)"
            >
              <span class="flex items-center justify-between gap-2">
                <span class="text-sm font-semibold">{{ option.title }}</span>
                <span
                  class="size-3.5 shrink-0 rounded-full border"
                  :class="form.delivery_channel === option.value ? 'border-[var(--accent)] bg-[var(--accent)]' : 'border-[var(--stroke-hi)]'"
                  aria-hidden="true"
                />
              </span>
              <span class="hint mt-1.5 block leading-relaxed">{{ option.desc }}</span>
            </button>
          </div>
        </div>

        <!-- New-API 是额度型商品：只有最少/最多两个额度字段，没有售价、划线价、
             库存或兑换比例。其它渠道继续用原来的价格与库存表单。 -->
        <div v-if="isNewAPI" class="card">
          <h3 class="mb-1 text-sm font-semibold">充值额度范围</h3>
          <p class="hint mb-4">用户下单时只需填写本次充值额度，系统按该额度创建兑换码。</p>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="p-min-topup">单次最少充值额度 *</label>
              <input
                id="p-min-topup"
                v-model.number="form.min_topup_amount"
                type="number"
                min="1"
                step="1"
                class="input nums"
                :class="{ 'input-error': Boolean(topupError) }"
                :aria-invalid="Boolean(topupError)"
              />
              <p class="hint mt-1.5">买家每次至少可充值的额度（正整数）。</p>
            </div>
            <div>
              <label class="label" for="p-max-topup">单次最多充值额度 *</label>
              <input
                id="p-max-topup"
                v-model.number="form.max_topup_amount"
                type="number"
                min="1"
                step="1"
                class="input nums"
                :class="{ 'input-error': Boolean(topupError) }"
                :aria-invalid="Boolean(topupError)"
              />
              <p class="hint mt-1.5">买家每次最多可充值的额度（正整数）。</p>
            </div>
            <p v-if="topupError" class="alert alert-danger sm:col-span-2" role="alert">{{ topupError }}</p>
          </div>
          <p class="hint mt-4">
            本渠道不设置售价、单价或兑换比例。若店铺尚未配置该额度的计价规则，买家下单会被明确告知，而不是显示虚假价格。
          </p>
        </div>

        <div v-else class="card">
          <h3 class="mb-4 text-sm font-semibold">价格与库存</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="p-price">售价（NL）*</label>
              <input
                id="p-price"
                v-model.number="form.price"
                type="number"
                min="1"
                step="1"
                class="input nums"
                :class="{ 'input-error': Boolean(priceError) }"
                :aria-invalid="Boolean(priceError)"
              />
              <p class="hint mt-1.5">下单金额按“售价 × 数量”由服务端计算。</p>
              <p v-if="priceError" class="field-error" role="alert">{{ priceError }}</p>
            </div>
            <div>
              <label class="label" for="p-original">划线原价（可选，NL）</label>
              <input id="p-original" v-model.number="form.original_price" type="number" min="0" step="1" class="input nums" />
            </div>
            <div v-if="isEdit">
              <span class="label">当前库存</span>
              <p class="nums mt-1.5 text-lg font-semibold">
                {{ isCard ? form.stock_count : '—' }}
              </p>
              <p class="hint mt-1.5">
                {{ isCard ? '卡密商品库存等于可用卡密数量，请在卡密页导入或清理。' : '人工交付商品不限制库存。' }}
              </p>
            </div>
            <div v-else>
              <span class="label">初始库存</span>
              <p class="hint mt-2.5">新建后为 0，导入卡密后自动计算。</p>
            </div>
            <label class="flex items-center gap-2.5 self-end pb-1 text-sm">
              <input v-model="form.stock_visible" type="checkbox" class="accent-[var(--accent)]" />
              在商品页显示库存
            </label>
          </div>
        </div>

        <div v-if="!isNewAPI" class="card">
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <h3 class="text-sm font-semibold">购买表单字段</h3>
              <p class="hint mt-1">买家下单时填写，内容会保存在订单中。仅支持文本与单选，不执行外部代码。</p>
            </div>
            <button class="btn btn-secondary btn-sm" type="button" :disabled="form.form_schema.length >= 20" @click="form.form_schema.push({ key: `field_${form.form_schema.length + 1}`, label: '', type: 'text', required: false, max_length: 255 })">添加字段</button>
          </div>
          <div v-if="form.form_schema.length" class="mt-4 space-y-3">
            <div v-for="(field, index) in form.form_schema" :key="index" class="grid gap-3 rounded-lg border border-[var(--stroke)] p-3 sm:grid-cols-2">
              <div><label class="label">字段标识</label><input v-model="field.key" class="input mono" maxlength="48" placeholder="account_id" /></div>
              <div><label class="label">买家看到的名称</label><input v-model="field.label" class="input" maxlength="80" placeholder="游戏账号" /></div>
              <div><label class="label">字段类型</label><select v-model="field.type" class="input"><option value="text">文本</option><option value="select">单选</option></select></div>
              <div class="flex items-end justify-between gap-3"><label class="flex items-center gap-2 pb-2 text-sm"><input v-model="field.required" type="checkbox" />必填</label><button class="btn btn-danger btn-sm" type="button" @click="form.form_schema.splice(index, 1)">移除</button></div>
              <div v-if="field.type === 'text'"><label class="label">提示文字</label><input v-model="field.placeholder" class="input" maxlength="120" placeholder="请输入账号" /></div>
              <div v-if="field.type === 'text'"><label class="label">最多字符数</label><input v-model.number="field.max_length" class="input" type="number" min="1" max="1000" /></div>
              <div v-if="field.type === 'select'" class="sm:col-span-2"><label class="label">选项（每行一项）</label><textarea :value="field.options?.join('\n')" class="input min-h-24" @input="field.options = ($event.target as HTMLTextAreaElement).value.split('\n').map((item) => item.trim()).filter(Boolean)" /></div>
            </div>
          </div>
          <p v-else class="hint mt-4">尚未添加字段；商品购买无需额外表单。</p>
        </div>

        <div v-if="isManual" class="card">
          <h3 class="mb-1 text-sm font-semibold">交付说明</h3>
          <p class="hint mb-4">人工交付商品会提示买家等待商家处理，可在此写明流程或时效。</p>
          <textarea v-model="form.delivery_instructions" class="input h-28 resize-none" />
        </div>

        <div v-else-if="isCard && isEdit" class="card">
          <div class="flex items-center justify-between">
            <div>
              <h3 class="text-sm font-semibold">卡密库存</h3>
              <p class="hint mt-1">当前可用 {{ form.stock_count }} 条</p>
            </div>
            <RouterLink :to="`/cards/${productId}`" class="btn btn-secondary btn-sm">管理卡密</RouterLink>
          </div>
        </div>
      </div>

      <div class="space-y-5">
        <div class="card">
          <h3 class="mb-4 text-sm font-semibold">发布</h3>
          <div class="space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-sm">立即上架</span>
              <input v-model="form.is_published" type="checkbox" class="accent-[var(--accent)]" />
            </div>
            <div class="flex items-center justify-between">
              <span class="text-sm">需要买家联系方式</span>
              <input v-model="form.require_contact" type="checkbox" class="accent-[var(--accent)]" />
            </div>
            <div>
              <div class="flex items-center justify-between">
                <span class="text-sm">前台推荐（首页「店长推荐」筛选）</span>
                <input v-model="form.is_featured" type="checkbox" class="accent-[var(--accent)]" />
              </div>
              <p class="hint mt-1.5">推荐位只在上架商品里生效。</p>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-sm">归档（不在前台显示）</span>
              <input v-model="form.is_archived" type="checkbox" class="accent-[var(--accent)]" />
            </div>
            <div>
              <label class="label" for="p-sort">排序权重</label>
              <input id="p-sort" v-model.number="form.sort_order" type="number" class="input nums" />
              <p class="hint mt-1.5">数字越小越靠前。</p>
            </div>
          </div>
        </div>

        <div v-if="isNewAPI" class="card">
          <h3 class="text-sm font-semibold">New-API 发货预览</h3>
          <ol class="mt-3 space-y-2 text-sm leading-relaxed text-[var(--text-dim)]">
            <li>1. 买家进入商品页，填写本次充值额度。</li>
            <li>2. 额度需在 {{ form.min_topup_amount || '?' }} – {{ form.max_topup_amount || '?' }} 之间，系统按服务端配置换算 quota。</li>
            <li>3. 支付成功后，系统向 New-API 创建兑换码并交付给买家。</li>
            <li>4. 买家需到 New-API 平台自行兑换，系统不会自动充值到账户。</li>
          </ol>
          <p class="hint mt-3">
            需要先在「系统设置 → New-API 发货」配置 API 地址、管理员凭据和 quota 比例，否则保存渠道时会被拒绝。
          </p>
        </div>

        <div class="card">
          <h3 class="mb-4 text-sm font-semibold">封面图</h3>
          <ImageField v-model="form.image_path" />
          <img
            v-if="form.image_path"
            :src="form.image_path"
            alt="封面预览"
            class="mt-3 h-36 w-full rounded-lg border border-[var(--stroke)] object-cover"
          />
          <p v-else class="mt-3 flex h-36 items-center justify-center rounded-lg border border-dashed border-[var(--stroke)] text-sm quiet">
            暂无封面
          </p>
        </div>
      </div>
    </div>
  </section>
</template>
