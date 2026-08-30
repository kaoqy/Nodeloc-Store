<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { listCoupons, createCoupon, updateCoupon, deleteCoupon } from '../api/coupons'
import type { Coupon } from '../types'

const loading = ref(true)
const coupons = ref<Coupon[]>([])
const editing = ref<Coupon | null>(null)

async function load() {
  loading.value = true
  try {
    const result = await listCoupons()
    coupons.value = result.data
  } finally {
    loading.value = false
  }
}

function startEdit(coupon: Coupon) {
  editing.value = { ...coupon }
}

async function saveEdit() {
  if (!editing.value) return
  if (editing.value.id) {
    await updateCoupon(editing.value.id, editing.value)
  } else {
    await createCoupon(editing.value)
  }
  editing.value = null
  await load()
}

async function deleteC(id: number) {
  if (!confirm('确定删除？')) return
  await deleteCoupon(id)
  await load()
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex items-center justify-between">
      <p class="text-sm text-[#6b6b80]">管理优惠券</p>
      <button class="btn-primary" @click="editing = { id: 0, code: '', discount_type: 'fixed', discount_value: 0, min_order_amount: 0, max_uses: 0, used_count: 0, is_active: true }">+ 新建优惠券</button>
    </div>

    <div class="table-container">
      <table>
        <thead>
          <tr><th>ID</th><th>优惠码</th><th>折扣</th><th>最低消费</th><th>使用次数</th><th>状态</th><th>操作</th></tr>
        </thead>
        <tbody>
          <tr v-if="loading"><td colspan="7"><div class="skeleton h-8" /></td></tr>
          <tr v-else-if="!coupons.length"><td colspan="7" class="py-8 text-center text-[#6b6b80]">暂无优惠券</td></tr>
          <tr v-for="c in coupons" :key="c.id">
            <td>{{ c.id }}</td>
            <td><code class="rounded bg-white/10 px-2 py-0.5 font-mono text-sm">{{ c.code }}</code></td>
            <td>{{ c.discount_type === 'fixed' ? `¥${c.discount_value}` : `${c.discount_value}%` }}</td>
            <td>¥{{ c.min_order_amount }}</td>
            <td>{{ c.used_count }}/{{ c.max_uses || '∞' }}</td>
            <td><span :class="['badge', c.is_active ? 'badge-success' : 'badge-neutral']">{{ c.is_active ? '启用' : '禁用' }}</span></td>
            <td>
              <button class="btn-ghost text-xs" @click="startEdit(c)">编辑</button>
              <button class="btn-ghost text-xs text-[#ef4444]" @click="deleteC(c.id)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <!-- Edit Modal -->
    <div v-if="editing" class="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
      <div class="card w-full max-w-md">
        <h3 class="mb-4 text-lg font-semibold">{{ editing.id ? '编辑优惠券' : '新建优惠券' }}</h3>
        <div class="space-y-3">
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">优惠码</label><input v-model="editing.code" class="input" /></div>
          <div class="grid grid-cols-2 gap-2">
            <div><label class="mb-1 block text-sm text-[#a1a1b5]">折扣类型</label>
              <select v-model="editing.discount_type" class="input"><option value="fixed">固定金额</option><option value="percentage">百分比</option></select>
            </div>
            <div><label class="mb-1 block text-sm text-[#a1a1b5]">折扣值</label><input v-model.number="editing.discount_value" type="number" class="input" /></div>
          </div>
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">最低消费</label><input v-model.number="editing.min_order_amount" type="number" class="input" /></div>
          <div><label class="mb-1 block text-sm text-[#a1a1b5]">最大使用次数 (0=无限)</label><input v-model.number="editing.max_uses" type="number" class="input" /></div>
          <label class="flex items-center gap-2 text-sm"><input v-model="editing.is_active" type="checkbox" /> 启用</label>
        </div>
        <div class="mt-4 flex gap-2">
          <button class="btn-secondary flex-1" @click="editing = null">取消</button>
          <button class="btn-primary flex-1" @click="saveEdit">保存</button>
        </div>
      </div>
    </div>
  </section>
</template>
