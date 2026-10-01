<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createCategory, deleteCategory, listCategories, updateCategory } from '../api/categories'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'
import { closeOnEscape } from '../utils/dialog'
import type { Category } from '../types'

const auth = useAuthStore()
const canManage = computed(() => auth.allows('categories', 'manage'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const categories = ref<Category[]>([])
const editing = ref<Partial<Category> | null>(null)
closeOnEscape(editing, null)

const sorted = computed(() => [...categories.value].sort((a, b) => (a.sort_order ?? 0) - (b.sort_order ?? 0) || a.id - b.id))
const canSave = computed(() => Boolean(editing.value?.name?.trim() && editing.value?.slug?.trim()))

function slugify(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^\w一-龥]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    categories.value = await listCategories()
  } catch (err) {
    error.value = errorMessage(err, '加载分类失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!editing.value || !canSave.value) return
  busy.value = true
  error.value = ''
  try {
    const payload = {
      name: editing.value.name?.trim(),
      slug: editing.value.slug?.trim(),
      description: editing.value.description || null,
      icon: editing.value.icon || null,
      sort_order: Number(editing.value.sort_order) || 0,
      is_visible: editing.value.is_visible ?? true,
    }
    if (editing.value.id) {
      await updateCategory(editing.value.id, payload)
    } else {
      await createCategory(payload)
    }
    editing.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存分类失败')
  } finally {
    busy.value = false
  }
}

async function remove(category: Category) {
  // 上一条删除还在路上时，第二个确认框不该再被答应一次。
  if (busy.value) return
  if (!confirm(`删除分类「${category.name}」？该分类下的商品会变成未分类。`)) return
  error.value = ''
  busy.value = true
  try {
    await deleteCategory(category.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除分类失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="quiet text-sm">前台分类导航的顺序与显隐在这里维护。</p>
      <button
        v-if="canManage"
        class="btn btn-primary btn-sm"
        @click="editing = { name: '', slug: '', description: '', icon: '', sort_order: 0, is_visible: true }"
      >
        + 新建分类
      </button>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>分类</th>
            <th>别名</th>
            <th>图标</th>
            <th>排序</th>
            <th>可见</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 5" :key="`skeleton-${i}`">
              <td colspan="6"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!sorted.length">
            <td colspan="6">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">还没有分类</p>
              </div>
            </td>
          </tr>
          <tr v-for="category in sorted" :key="category.id">
            <td>
              <p class="text-sm font-medium">{{ category.name }}</p>
              <p v-if="category.description" class="quiet truncate text-xs">{{ category.description }}</p>
            </td>
            <td class="mono text-sm quiet">{{ category.slug }}</td>
            <td class="text-sm">{{ category.icon || '—' }}</td>
            <td class="nums text-sm">{{ category.sort_order ?? 0 }}</td>
            <td>
              <span class="badge" :class="category.is_visible ? 'badge-success' : 'badge-neutral'">
                {{ category.is_visible ? '显示' : '隐藏' }}
              </span>
            </td>
            <td class="whitespace-nowrap text-right">
              <button v-if="canManage" class="btn btn-ghost btn-sm" @click="editing = { ...category }">编辑</button>
              <button
                v-if="canManage"
                class="btn btn-ghost btn-sm text-[var(--danger)]"
                :disabled="busy"
                @click="remove(category)"
              >
                删除
              </button>
              <span v-if="!canManage" class="quiet text-xs">只读</span>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div
      v-if="editing"
      class="overlay" role="dialog" aria-modal="true" aria-label="分类表单"
      @click.self="editing = null"
    >
      <div class="card w-full max-w-md !p-5">
        <h3 class="text-base font-semibold">{{ editing.id ? '编辑分类' : '新建分类' }}</h3>
        <div class="mt-4 space-y-3">
          <div>
            <label class="label" for="c-name">名称 *</label>
            <input
              id="c-name"
              v-model="editing.name"
              class="input"
              @blur="!editing.slug && (editing.slug = slugify(editing.name || ''))"
            />
          </div>
          <div>
            <label class="label" for="c-slug">别名 (slug) *</label>
            <input id="c-slug" v-model="editing.slug" class="input mono text-xs" />
          </div>
          <div>
            <label class="label" for="c-desc">描述</label>
            <input id="c-desc" v-model="editing.description" class="input" />
          </div>
          <div class="grid gap-3 sm:grid-cols-2">
            <div>
              <label class="label" for="c-icon">图标</label>
              <input id="c-icon" v-model="editing.icon" class="input" placeholder="可选" />
            </div>
            <div>
              <label class="label" for="c-sort">排序</label>
              <input id="c-sort" v-model.number="editing.sort_order" type="number" class="input nums" />
            </div>
          </div>
          <label class="flex items-center gap-2.5 text-sm">
            <input v-model="editing.is_visible" type="checkbox" class="accent-[var(--accent)]" />
            在前台显示
          </label>
        </div>
        <div class="mt-5 flex justify-end gap-2">
          <button class="btn btn-secondary btn-sm" @click="editing = null">取消</button>
          <button class="btn btn-primary btn-sm" :disabled="busy || !canSave" @click="save">
            {{ busy ? '保存中…' : '保存' }}
          </button>
        </div>
      </div>
    </div>
  </section>
</template>
