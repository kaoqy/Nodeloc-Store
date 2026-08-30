<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { getProduct, createProduct, updateProduct } from '../api/products'
import { listCategories } from '../api/categories'
import type { Category } from '../types'

const route = useRoute()
const router = useRouter()
const isEdit = ref(false)
const loading = ref(true)
const saving = ref(false)
const categories = ref<Category[]>([])

const form = reactive({
  id: 0,
  name: '',
  slug: '',
  category_id: undefined as number | undefined,
  price: 0,
  original_price: undefined as number | undefined,
  stock_count: 0,
  stock_visible: true,
  auto_deliver: true,
  is_published: true,
  product_type: 'card',
  summary: '',
  description: '',
  delivery_instructions: '',
  image_path: '',
  require_contact: false,
})

async function load() {
  try {
    categories.value = (await listCategories()).data
    if (route.currentRoute.value.params.id) {
      isEdit.value = true
      const product = await getProduct(Number(route.currentRoute.value.params.id))
      Object.assign(form, product)
    }
  } finally {
    loading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    if (isEdit.value) {
      await updateProduct(form.id, form)
    } else {
      await createProduct(form)
    }
    router.push('/products')
  } finally {
    saving.value = false
  }
}

onMounted(load)
</script>

<template>
  <section v-if="loading" class="space-y-4">
    <div class="skeleton h-8 w-48" />
    <div class="grid gap-4 lg:grid-cols-2">
      <div v-for="i in 6" :key="i" class="skeleton h-12" />
    </div>
  </section>

  <section v-else class="space-y-6">
    <div class="flex items-center justify-between">
      <div>
        <RouterLink to="/products" class="text-sm text-[#6b6b80] hover:text-white">← 返回商品列表</RouterLink>
        <h2 class="mt-2 text-xl font-bold">{{ isEdit ? '编辑商品' : '新建商品' }}</h2>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" @click="router.push('/products')">取消</button>
        <button class="btn-primary" :disabled="saving" @click="save">
          {{ saving ? '保存中...' : '保存商品' }}
        </button>
      </div>
    </div>

    <div class="grid gap-6 lg:grid-cols-3">
      <div class="space-y-4 lg:col-span-2">
        <div class="card">
          <h3 class="mb-4 font-semibold">基本信息</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <div class="sm:col-span-2">
              <label class="mb-1 block text-sm text-[#a1a1b5]">商品名称</label>
              <input v-model="form.name" class="input" placeholder="输入商品名称" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">商品别名 (slug)</label>
              <input v-model="form.slug" class="input" placeholder="product-slug" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">商品类型</label>
              <select v-model="form.product_type" class="input">
                <option value="card">卡密</option>
                <option value="manual">人工交付</option>
              </select>
            </div>
            <div class="sm:col-span-2">
              <label class="mb-1 block text-sm text-[#a1a1b5]">商品简介</label>
              <input v-model="form.summary" class="input" placeholder="简短描述" />
            </div>
            <div class="sm:col-span-2">
              <label class="mb-1 block text-sm text-[#a1a1b5]">详细描述</label>
              <textarea v-model="form.description" class="input min-h-32 resize-none" placeholder="商品详细说明..." />
            </div>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">价格与库存</h3>
          <div class="grid gap-4 sm:grid-cols-2">
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">售价</label>
              <input v-model.number="form.price" type="number" class="input" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">原价（可选）</label>
              <input v-model.number="form.original_price" type="number" class="input" />
            </div>
            <div>
              <label class="mb-1 block text-sm text-[#a1a1b5]">库存数量</label>
              <input v-model.number="form.stock_count" type="number" class="input" />
            </div>
            <div class="flex items-center gap-4 pt-6">
              <label class="flex items-center gap-2 text-sm">
                <input v-model="form.stock_visible" type="checkbox" class="rounded" />
                显示库存
              </label>
              <label class="flex items-center gap-2 text-sm">
                <input v-model="form.auto_deliver" type="checkbox" class="rounded" />
                自动发货
              </label>
            </div>
          </div>
        </div>
      </div>

      <div class="space-y-4">
        <div class="card">
          <h3 class="mb-4 font-semibold">发布设置</h3>
          <div class="space-y-3">
            <label class="flex items-center justify-between">
              <span class="text-sm">上架状态</span>
              <button
                :class="['btn', form.is_published ? 'btn-primary' : 'btn-secondary']"
                @click="form.is_published = !form.is_published"
              >
                {{ form.is_published ? '已上架' : '已下架' }}
              </button>
            </label>
            <label class="flex items-center justify-between">
              <span class="text-sm">需要联系方式</span>
              <input v-model="form.require_contact" type="checkbox" class="rounded" />
            </label>
          </div>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">分类</h3>
          <select v-model="form.category_id" class="input">
            <option :value="undefined">选择分类</option>
            <option v-for="cat in categories" :key="cat.id" :value="cat.id">{{ cat.name }}</option>
          </select>
        </div>

        <div class="card">
          <h3 class="mb-4 font-semibold">商品图片</h3>
          <input v-model="form.image_path" class="input" placeholder="图片 URL" />
          <div v-if="form.image_path" class="mt-3">
            <img :src="form.image_path" class="h-32 w-full rounded-lg object-cover" />
          </div>
        </div>

        <div v-if="form.product_type === 'manual'" class="card">
          <h3 class="mb-4 font-semibold">交付说明</h3>
          <textarea v-model="form.delivery_instructions" class="input min-h-24 resize-none" placeholder="人工交付给买家的说明..." />
        </div>
      </div>
    </div>
  </section>
</template>
