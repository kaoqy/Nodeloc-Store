<script setup lang="ts">
import { onMounted, ref } from 'vue'
import {
  deleteKnowledge,
  deleteKnowledgeCategory,
  importKnowledge,
  listKnowledge,
  listKnowledgeCategories,
  saveKnowledge,
  saveKnowledgeCategory,
  testKnowledge,
  type KnowledgeArticle,
  type KnowledgeCategory,
} from '../../api/support'
import { errorMessage, when } from '../../utils/format'
import SaveBar from '../../components/SaveBar.vue'
import { useAuthStore } from '../../stores/auth'

const auth = useAuthStore()
const canManage = auth.allows('knowledge', 'manage')

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const articles = ref<KnowledgeArticle[]>([])
const categories = ref<KnowledgeCategory[]>([])
const total = ref(0)
const search = ref('')
const status = ref('all')
const categoryId = ref(0)
const importText = ref('')

const draft = ref<Partial<KnowledgeArticle>>({
  title: '',
  content: '',
  summary: '',
  keywords: '',
  tags: '',
  status: 'draft',
  priority: 0,
  category_id: 0,
})

const categoryDraft = ref<Partial<KnowledgeCategory>>({ name: '', sort_order: 0, is_enabled: true })

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [page, cats] = await Promise.all([
      listKnowledge({
        q: search.value,
        status: status.value === 'all' ? undefined : status.value,
        category_id: categoryId.value || undefined,
        limit: 50,
      }),
      listKnowledgeCategories().catch(() => []),
    ])
    articles.value = page.data
    total.value = page.total
    categories.value = cats
  } catch (err) {
    error.value = errorMessage(err, '加载知识库失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!draft.value.title?.trim() || !draft.value.content?.trim()) {
    error.value = '文章标题和正文都要填。'
    return
  }
  busy.value = true
  error.value = ''
  try {
    await saveKnowledge(draft.value)
    notice.value = '文章已保存。'
    draft.value = { title: '', content: '', summary: '', keywords: '', tags: '', status: 'draft', priority: 0, category_id: 0 }
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存文章失败')
  } finally {
    busy.value = false
  }
}

function edit(article: KnowledgeArticle) {
  draft.value = { ...article }
}

async function remove(article: KnowledgeArticle) {
  if (!article.id || !confirm('删除文章「' + article.title + '」？')) return
  busy.value = true
  try {
    await deleteKnowledge(article.id)
    notice.value = '文章已删除。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除文章失败')
  } finally {
    busy.value = false
  }
}

async function test(article: KnowledgeArticle) {
  if (!article.id) return
  busy.value = true
  try {
    const result = await testKnowledge(article.id)
    notice.value = String(result.message || '')
  } catch (err) {
    error.value = errorMessage(err, '测试失败')
  } finally {
    busy.value = false
  }
}

async function saveCategory() {
  if (!categoryDraft.value.name?.trim()) return
  try {
    await saveKnowledgeCategory(categoryDraft.value)
    categoryDraft.value = { name: '', sort_order: 0, is_enabled: true }
    categories.value = await listKnowledgeCategories()
    notice.value = '分类已保存。'
  } catch (err) {
    error.value = errorMessage(err, '保存分类失败')
  }
}

async function removeCategory(category: KnowledgeCategory) {
  if (!category.id || !confirm('删除分类「' + category.name + '」？')) return
  try {
    await deleteKnowledgeCategory(category.id)
    categories.value = await listKnowledgeCategories()
  } catch (err) {
    error.value = errorMessage(err, '删除分类失败')
  }
}

async function doImport() {
  if (!importText.value.trim()) return
  busy.value = true
  try {
    const result = await importKnowledge(importText.value, categoryId.value)
    notice.value = '已导入 ' + result.imported + ' 篇文章（状态为草稿）。'
    importText.value = ''
    await load()
  } catch (err) {
    error.value = errorMessage(err, '导入失败')
  } finally {
    busy.value = false
  }
}

onMounted(load)
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <p class="text-sm font-semibold">知识库文章</p>
        <p class="quiet mt-1 text-xs">共 {{ total }} 篇。AI 回答时优先引用这里已发布的文章。</p>
      </div>
      <div class="flex flex-wrap gap-2">
        <input v-model="search" class="input w-52" placeholder="搜索标题 / 关键词 / 正文" @keyup.enter="load" />
        <select v-model="status" class="input !w-auto" @change="load">
          <option value="all">全部状态</option>
          <option value="published">已发布</option>
          <option value="draft">草稿</option>
          <option value="disabled">已停用</option>
        </select>
        <select v-model.number="categoryId" class="input !w-auto" @change="load">
          <option :value="0">全部分类</option>
          <option v-for="category in categories" :key="category.id" :value="category.id">{{ category.name }}</option>
        </select>
        <button class="btn btn-secondary btn-sm" @click="load">查询</button>
      </div>
    </div>

    <SaveBar
      resource="knowledge"
      :saving="busy"
      :dirty="Boolean(draft.title || draft.content)"
      save-label="保存文章"
      hint="保存右侧正在编辑的文章"
      @save="save"
    />

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div class="grid gap-4 xl:grid-cols-[minmax(0,1fr)_360px]">
      <div class="space-y-4">
        <div v-if="loading" class="card space-y-3">
          <div v-for="i in 5" :key="i" class="skeleton h-10 w-full" />
        </div>
        <div v-else-if="!articles.length" class="card py-16 text-center text-sm text-[var(--text-quiet)]">
          还没有知识库文章。可以先导入或新建「购买流程」「退款规则」等基础说明。
        </div>
        <div v-else class="table-container">
          <table class="table">
            <thead>
              <tr><th>标题</th><th>分类</th><th>状态</th><th class="nums">优先级</th><th class="nums">版本</th><th>更新</th><th class="text-right">操作</th></tr>
            </thead>
            <tbody>
              <tr v-for="article in articles" :key="article.id">
                <td>
                  <p class="font-semibold">{{ article.title }}</p>
                  <p class="quiet mono text-[11px]">{{ article.slug }}</p>
                </td>
                <td class="text-xs">
                  {{ categories.find((item) => item.id === article.category_id)?.name || '未分类' }}
                </td>
                <td>
                  <span :class="article.status === 'published' ? 'badge-success' : article.status === 'disabled' ? 'badge-danger' : 'badge'">
                    {{ article.status }}
                  </span>
                  <span v-if="article.builtin" class="badge ml-1">内置</span>
                </td>
                <td class="nums">{{ article.priority }}</td>
                <td class="nums">{{ article.version || 1 }}</td>
                <td class="quiet text-xs">{{ when(article.created_at) }}</td>
                <td class="text-right">
                  <div class="flex justify-end gap-1.5">
                    <button class="btn btn-quiet btn-sm" @click="test(article)">测试引用</button>
                    <button v-if="canManage" class="btn btn-quiet btn-sm" @click="edit(article)">编辑</button>
                    <button v-if="canManage" class="btn btn-danger btn-sm" @click="remove(article)">删除</button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <aside class="space-y-4">
        <div v-if="canManage" class="card space-y-3">
          <p class="eyebrow">{{ draft.id ? '编辑文章' : '新建文章' }}</p>
          <input v-model="draft.title" class="input" placeholder="文章标题" />
          <input v-model="draft.summary" class="input" placeholder="摘要（可选）" />
          <textarea v-model="draft.content" class="input min-h-[160px]" placeholder="正文内容，AI 会引用这里的文字" />
          <input v-model="draft.keywords" class="input" placeholder="关键词，用逗号分隔" />
          <div class="grid grid-cols-2 gap-2">
            <select v-model.number="draft.category_id" class="input">
              <option :value="0">未分类</option>
              <option v-for="category in categories" :key="category.id" :value="category.id">{{ category.name }}</option>
            </select>
            <select v-model="draft.status" class="input">
              <option value="draft">草稿</option>
              <option value="published">已发布</option>
              <option value="disabled">已停用</option>
            </select>
          </div>
          <input v-model.number="draft.priority" class="input nums" type="number" placeholder="优先级（越大越优先）" />
          <div class="flex gap-2">
            <button class="btn btn-primary btn-sm flex-1" :disabled="busy" @click="save">保存文章</button>
            <button class="btn btn-secondary btn-sm" @click="draft = { title: '', content: '', summary: '', keywords: '', tags: '', status: 'draft', priority: 0, category_id: 0 }">清空</button>
          </div>
        </div>

        <div v-if="canManage" class="card space-y-3">
          <p class="eyebrow">批量导入</p>
          <p class="quiet text-xs">每行一篇，格式为「标题|正文」；只写标题也可以。</p>
          <textarea v-model="importText" class="input min-h-[120px] mono text-xs" placeholder="购买流程|1. 登录
2. 选择商品" />
          <button class="btn btn-secondary btn-sm" :disabled="busy" @click="doImport">导入</button>
        </div>

        <div v-if="canManage" class="card space-y-3">
          <p class="eyebrow">分类管理</p>
          <div v-for="category in categories" :key="category.id" class="flex items-center gap-2 text-sm">
            <span class="min-w-0 flex-1 truncate">{{ category.name }}</span>
            <button class="btn btn-danger btn-sm" @click="removeCategory(category)">删除</button>
          </div>
          <div class="flex gap-2">
            <input v-model="categoryDraft.name" class="input" placeholder="新分类名称" />
            <button class="btn btn-secondary btn-sm" @click="saveCategory">添加</button>
          </div>
        </div>
      </aside>
    </div>
  </div>
</template>
