<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AdminIcon from '../components/AdminIcon.vue'
import AppDrawer from '../components/AppDrawer.vue'
import DataTable, { type Column } from '../components/DataTable.vue'
import PageHeader from '../components/PageHeader.vue'
import StatusBadge from '../components/StatusBadge.vue'
import { createCategory, deleteCategory, listCategories, updateCategory } from '../api/categories'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import type { Category } from '../types'

/** 分类管理：名称、图标、排序与前台可见性。 */

const auth = useAuthStore()
const canManage = computed(() => auth.allows('categories', 'manage'))

const COLUMNS: Column[] = [
  { label: '分类' },
  { label: 'Slug', hideOnMobile: true },
  { label: '排序', numeric: true, hideOnMobile: true },
  { label: '状态' },
  { label: '', actions: true, width: '150px' },
]

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const rows = ref<Category[]>([])
const editing = ref<Partial<Category> | null>(null)

async function load() {
  loading.value = true
  error.value = ''
  try {
    rows.value = await listCategories()
  } catch (err) {
    error.value = errorMessage(err, '加载分类失败')
  } finally {
    loading.value = false
  }
}

function startCreate() {
  editing.value = { name: '', slug: '', description: '', icon: '', sort_order: rows.value.length * 10, is_visible: true }
}

async function save() {
  const draft = editing.value
  if (!draft) return
  if (!draft.name?.trim()) {
    error.value = '请填写分类名称。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    if (draft.id) await updateCategory(draft.id, draft)
    else await createCategory(draft)
    notice.value = draft.id ? '分类已更新。' : '分类已创建。'
    editing.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存失败')
  } finally {
    busy.value = false
  }
}

async function remove(row: Category) {
  if (!window.confirm('删除分类「' + row.name + '」？该分类下的商品不会被删除，但会变成未分类。')) return
  busy.value = true
  try {
    await deleteCategory(row.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <PageHeader title="分类管理" description="分类决定前台筛选入口与商品归属。" bordered>
      <template #actions>
        <button v-if="canManage" class="btn btn-primary btn-sm" @click="startCreate">
          <AdminIcon name="plus" :size="14" />
          新建分类
        </button>
      </template>
    </PageHeader>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <DataTable
      :columns="COLUMNS"
      :loading="loading"
      :error="error"
      :total="rows.length"
      :summary="'共 ' + rows.length + ' 个分类'"
      empty-title="还没有分类"
      empty-hint="创建分类后，商品可以归入其中并在前台按分类筛选。"
      @retry="load"
    >
      <tr v-for="row in rows" :key="row.id">
        <td>
          <span class="font-semibold">
            <span v-if="row.icon" class="mr-1.5">{{ row.icon }}</span>{{ row.name }}
          </span>
          <p v-if="row.description" class="quiet mt-0.5 truncate text-xs">{{ row.description }}</p>
        </td>
        <td class="mono hide-on-mobile text-xs">{{ row.slug }}</td>
        <td class="nums hide-on-mobile">{{ row.sort_order }}</td>
        <td><StatusBadge :value="row.is_visible ? 'ok' : 'disabled'" :label="row.is_visible ? '可见' : '隐藏'" /></td>
        <td class="text-right">
          <div class="flex justify-end gap-1.5">
            <button v-if="canManage" class="btn btn-quiet btn-sm" @click="editing = { ...row }">编辑</button>
            <button v-if="canManage" class="btn btn-danger btn-sm" :disabled="busy" @click="remove(row)">删除</button>
          </div>
        </td>
      </tr>
    </DataTable>

    <AppDrawer :open="Boolean(editing)" :title="editing?.id ? '编辑分类' : '新建分类'" width="sm" @close="editing = null">
      <div v-if="editing" class="space-y-3">
        <div>
          <label class="label" for="cat-name">名称</label>
          <input id="cat-name" v-model="editing.name" class="input" maxlength="120" />
        </div>
        <div>
          <label class="label" for="cat-slug">Slug（留空自动生成）</label>
          <input id="cat-slug" v-model="editing.slug" class="input mono text-xs" maxlength="120" />
        </div>
        <div>
          <label class="label" for="cat-desc">说明</label>
          <textarea id="cat-desc" v-model="editing.description" class="input min-h-[80px]" />
        </div>
        <div class="grid grid-cols-2 gap-3">
          <div>
            <label class="label" for="cat-icon">图标</label>
            <input id="cat-icon" v-model="editing.icon" class="input" placeholder="可选 emoji" maxlength="8" />
          </div>
          <div>
            <label class="label" for="cat-sort">排序</label>
            <input id="cat-sort" v-model.number="editing.sort_order" class="input nums" type="number" />
          </div>
        </div>
        <label class="flex items-center gap-2 text-sm"><input v-model="editing.is_visible" type="checkbox" />在前台可见</label>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="editing = null">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy" @click="save">{{ busy ? '保存中…' : '保存' }}</button>
        </div>
      </template>
    </AppDrawer>
  </section>
</template>
