<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listCards, deleteCard } from '../api/cards'
import type { Card } from '../types'

const loading = ref(true)
const cards = ref<Card[]>([])
const search = ref('')
const page = ref(1)
const perPage = ref(20)

const statusText: Record<string, string> = {
  available: '可用',
  sold: '已售',
  expired: '过期',
}

async function load() {
  loading.value = true
  try {
    const params: any = { page: page.value, per_page: perPage.value }
    if (search.value) params.q = search.value
    const result = await listCards(params)
    cards.value = result.data
  } finally {
    loading.value = false
  }
}

async function deleteC(id: number) {
  if (!confirm('确定删除此卡密？')) return
  await deleteCard(id)
  await load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-4">
      <div class="flex items-center gap-3">
        <input v-model="search" class="input w-64" placeholder="搜索卡密内容..." @keyup.enter="load" />
        <button class="btn-secondary" @click="load">筛选</button>
      </div>
      <div class="flex items-center gap-2">
        <button class="btn-secondary">导入卡密</button>
        <button class="btn-primary">+ 新建卡密</button>
      </div>
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr><th>ID</th><th>商品</th><th>内容</th><th>状态</th><th>订单</th><th>创建时间</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="7"><div class="skeleton h-8" /></td></tr>
          <tr v-else-if="!cards.length"><td colspan="7" class="py-8 text-center text-[#6b6b80]">暂无卡密</td></tr>
          <tr v-for="card in cards" :key="card.id">
            <td>{{ card.id }}</td>
            <td>{{ card.product?.name || '-' }}</td>
            <td><code class="rounded bg-white/10 px-2 py-0.5 font-mono text-xs max-w-[200px] truncate inline-block">{{ card.content }}</code></td>
            <td>
              <span
                :class="[
                  'badge',
                  card.status === 'available' ? 'badge-success' :
                  card.status === 'sold' ? 'badge-info' : 'badge-neutral'
                ]"
              >
                {{ statusText[card.status] || card.status }}
              </span>
            </td>
            <td>{{ card.order_id || '-' }}</td>
            <td class="text-[#a1a1b5]">{{ card.created_at }}</td>
            <td>
              <button class="btn-ghost text-xs text-[#ef4444]" @click="deleteC(card.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">显示 {{ cards.length }} 条</p>
      <div class="flex items-center gap-2">
        <button class="btn-secondary" :disabled="page <= 1" @click="page--; load()">上一页</button>
        <span class="text-sm text-[#a1a1b5]">第 {{ page }} 页</span>
        <button class="btn-secondary" @click="page++; load()">下一页</button>
      </div>
    </div>
  </section>
</template>
