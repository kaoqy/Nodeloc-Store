<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listCategories, createCategory, updateCategory, deleteCategory } from '../api/categories'
import type { Category } from '../types'

const loading = ref(true)
const categories = ref<Category[]>([])
const editing = ref<Category | null>(null)

async function load() {
  loading.value = true
  try {
    const result = await listCategories()
    categories.value = result.data
  } finally {
    loading.value = false
  }
}

function startEdit(cat: Category) {
  editing.value = { ...cat }
}

async function saveEdit() {
  if (!editing.value) return
  if (editing.value.id) {
    await updateCategory(editing.value.id, editing.value)
  } else {
    await createCategory(editing.value)
  }
  editing.value = null
  await load()
}

async function deleteCat(id: number) {
  if (!confirm('确定删除此分类？')) return
  await deleteCategory(id)
  await load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">管理商品分类</p>
      <button class="btn-primary" @click="editing = { id: 0, name: '', slug: '', icon: '', sort_order: 0, is_visible: true }">+ 新建分类</button>
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr><th>ID</th><th>名称</th><th>别名</th><th>排序</th><th>可见</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="6"><div class="skeleton h-8" /></td></tr>
          <tr v-else-if="!categories.length"><td colspan="6" class="py-8 text-center text-[#6b6b80]">暂无分类</td></tr>
          <tr v-for="cat in categories" :key="cat.id">
            <td>{{ cat.id }}</td>
            <td>{{ cat.name }}</td>
            <td>{{ cat.slug }}</td>
            <td>{{ cat.sort_order }}</td>
            <td><span :class="['badge', cat.is_visible ? 'badge-success' : 'badge-neutral']">{{ cat.is_visible ? '显示' : '隐藏' }}</span></td>
            <td>
              <button class="btn-ghost text-xs" @click="startEdit(cat)">编辑</button>
              <button class="btn-ghost text-xs text-[#ef4444]" @click="deleteCat(cat.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Edit Modal -->
    <div v-if="editing" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div class="card w-full max-w-md">
        <h3 class="mb-4 text-lg font-semibold">{{ editing.id ? '编辑分类' : '新建分类' }}</h3>
        <div class="space-y-3">
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">名称</label><input v-model="editing.name" class="input" /></div>
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">别名</label><input v-model="editing.slug" class="input" /></div>
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">图标</label><input v-model="editing.icon" class="input" /></div>
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">排序</label><input v-model.number="editing.sort_order" type="number" class="input" /></div>
          <label class="flex items-center gap-2 text-sm"><input v-model="editing.is_visible" type="checkbox" /> 可见</label>
        </div>
        <div class="mt-4 flex gap-2">
          <button class="btn-secondary flex-1" @click="editing = null">取消</button>
          <button class="btn-primary flex-1" @click="saveEdit">保存</button>
        </div>
      </div>
    </div>
  </section>
</template>
