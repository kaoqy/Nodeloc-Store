<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { createProduct, getProduct, updateProduct } from '../api/products'
import { listCategories } from '../api/categories'
import { errorMessage } from '../utils/format'
import type { Category, Product } from '../types'

const route = useRoute()
const router = useRouter()

const productId = computed(() => Number(route.params.id || 0))
const isEdit = computed(() => productId.value > 0)

const loading = ref(true)
const saving = ref(false)
const error = ref('')
const categories = ref<Category[]>([])

const form = reactive({
  name: '',
  slug: '',
  summary: '',
  description: '',
  image_path: '',
  delivery_instructions: '',
  product_type: 'card',
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
})

const isCard = computed(() => form.product_type === 'card')
const invalid = computed(() => !form.name.trim() || !form.slug.trim() || Number(form.price) < 0)

// The update endpoint replaces the whole row, so only these fields are sent back.
function payload(): Partial<Product> {
  return {
    slug: form.slug.trim(),
    name: form.name.trim(),
    summary: form.summary.trim(),
    description: form.description,
    image_path: form.image_path.trim(),
    product_type: form.product_type,
    delivery_instructions: form.delivery_instructions,
    require_contact: form.require_contact,
    price: Number(form.price) || 0,
    original_price: form.original_price === null || Number.isNaN(Number(form.original_price))
      ? null
      : Number(form.original_price),
    stock_visible: form.stock_visible,
    stock_count: form.stock_count,
    is_published: form.is_published,
    is_featured: form.is_featured,
    is_archived: form.is_archived,
    sort_order: Number(form.sort_order) || 0,
    category_id: form.category_id,
  }
}

function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^\w一-龥]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

async function load() {
  loading.value = true
  try {
    categories.value = await listCategories()
    if (isEdit.value) {
      const product = await getProduct(productId.value)
      Object.assign(form, {
        name: product.name,
        slug: product.slug,
        summary: product.summary || '',
        description: product.description || '',
        image_path: product.image_path || '',
        delivery_instructions: product.delivery_instructions || '',
        product_type: product.product_type,
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
      })
    }
  } catch (err) {
    error.value = errorMessage(err, '加载商品失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (invalid.value) return
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

  <section v-else class="space-y-5">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <RouterLink to="/products" class="quiet text-xs hover:text-[var(--text)]">← 返回商品列表</RouterLink>
        <h2 class="mt-1.5 text-xl font-bold">{{ isEdit ? '编辑商品' : '新建商品' }}</h2>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn btn-secondary btn-sm" @click="router.push('/products')">取消</button>
        <button class="btn btn-primary btn-sm" :disabled="saving || invalid" @click="save">
          {{ saving ? '保存中…' : '保存商品' }}
        </button>
      </div>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

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
              <label class="label" for="p-type">交付方式</label>
              <select id="p-type" v-model="form.product_type" class="input">
                <option value="card">卡密自动发货</option>
                <option value="manual">商家人工发货</option>
              </select>
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
          <h3 class="mb-4 text-sm font-semibold">价格与库存</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="label" for="p-price">售价（元）*</label>
              <input id="p-price" v-model.number="form.price" type="number" min="0" step="0.01" class="input nums" />
            </div>
            <div>
              <label class="label" for="p-original">划线原价（可选）</label>
              <input id="p-original" v-model.number="form.original_price" type="number" min="0" step="0.01" class="input nums" />
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

        <div v-if="!isCard" class="card">
          <h3 class="mb-1 text-sm font-semibold">交付说明</h3>
          <p class="hint mb-4">人工交付商品会提示买家等待商家处理，可在此写明流程或时效。</p>
          <textarea v-model="form.delivery_instructions" class="input h-28 resize-none" />
        </div>

        <div v-else-if="isEdit" class="card">
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

        <div class="card">
          <h3 class="mb-4 text-sm font-semibold">封面图</h3>
          <input v-model="form.image_path" class="input mono text-xs" placeholder="https://… 或 /uploads/…" />
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
