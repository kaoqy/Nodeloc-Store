<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { createCoupon, deleteCoupon, listCoupons, updateCoupon } from '../api/coupons'
import { errorMessage, money, when } from '../utils/format'
import type { Coupon } from '../types'

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const coupons = ref<Coupon[]>([])
const editing = ref<Partial<Coupon> | null>(null)

const canSave = computed(() => {
  const item = editing.value
  if (!item) return false
  const value = Number(item.discount_value)
  if (!item.code?.trim() || !value || value <= 0) return false
  return item.discount_type !== 'percent' || value <= 100
})

async function load() {
  loading.value = true
  error.value = ''
  try {
    coupons.value = await listCoupons()
  } catch (err) {
    error.value = errorMessage(err, '加载优惠券失败')
  } finally {
    loading.value = false
  }
}

function startCreate() {
  editing.value = {
    code: '',
    discount_type: 'fixed',
    discount_value: 0,
    min_order_amount: 0,
    max_uses: 0,
    used_count: 0,
    is_active: true,
    valid_from: null,
    valid_until: null,
  }
}

// Dates are calendar days chosen in the admin's timezone: convert with local
// parts, never with toISOString(), whose UTC rendering shifts the day by the
// timezone offset.
function toDateInput(value?: string | null): string {
  if (!value) return ''
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (part: number) => String(part).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

function fromDateInput(value: string): string | null {
  return value ? new Date(`${value}T00:00:00`).toISOString() : null
}

async function save() {
  if (!editing.value || !canSave.value) return
  const item = editing.value
  busy.value = true
  error.value = ''
  try {
    const payload = {
      code: item.code?.trim().toUpperCase(),
      discount_type: item.discount_type,
      discount_value: Number(item.discount_value),
      min_order_amount: Number(item.min_order_amount) || 0,
      max_uses: Number(item.max_uses) || 0,
      used_count: item.used_count ?? 0,
      is_active: item.is_active ?? true,
      valid_from: item.valid_from || null,
      valid_until: item.valid_until || null,
    }
    if (item.id) {
      await updateCoupon(item.id, payload)
    } else {
      await createCoupon(payload)
    }
    editing.value = null
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存优惠券失败')
  } finally {
    busy.value = false
  }
}

async function remove(coupon: Coupon) {
  if (!confirm(`删除优惠码「${coupon.code}」？`)) return
  error.value = ''
  try {
    await deleteCoupon(coupon.id)
    await load()
  } catch (err) {
    error.value = errorMessage(err, '删除优惠券失败')
  }
}

function discountLabel(coupon: Coupon): string {
  return coupon.discount_type === 'percent' ? `立减 ${coupon.discount_value}%` : `立减 ${money(coupon.discount_value)}`
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <div class="flex flex-wrap items-center justify-between gap-3">
      <p class="quiet text-sm">优惠码目前独立维护，尚未参与下单金额计算。</p>
      <button class="btn btn-primary btn-sm" @click="startCreate">+ 新建优惠券</button>
    </div>

    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>

    <div class="table-container">
      <table>
        <thead>
          <tr>
            <th>优惠码</th>
            <th>折扣</th>
            <th>最低消费</th>
            <th>使用情况</th>
            <th>有效期</th>
            <th>状态</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          <template v-if="loading">
            <tr v-for="i in 5" :key="`skeleton-${i}`">
              <td colspan="7"><div class="skeleton h-6" /></td>
            </tr>
          </template>
          <tr v-else-if="!coupons.length">
            <td colspan="7">
              <div class="empty-state">
                <p class="empty-glyph" aria-hidden="true">◌</p>
                <p class="empty-title">还没有优惠券</p>
              </div>
            </td>
          </tr>
          <tr v-for="coupon in coupons" :key="coupon.id">
            <td><code class="mono text-sm">{{ coupon.code }}</code></td>
            <td class="nums text-sm">{{ discountLabel(coupon) }}</td>
            <td class="nums text-sm muted">{{ coupon.min_order_amount ? money(coupon.min_order_amount) : '不限' }}</td>
            <td class="nums text-sm">
              {{ coupon.used_count }} / {{ coupon.max_uses || '∞' }}
            </td>
            <td class="text-xs quiet">
              {{ coupon.valid_from || coupon.valid_until ? `${toDateInput(coupon.valid_from) || '立即'} → ${toDateInput(coupon.valid_until) || '长期'}` : '长期有效' }}
            </td>
            <td>
              <span class="badge" :class="coupon.is_active ? 'badge-success' : 'badge-neutral'">
                {{ coupon.is_active ? '启用' : '停用' }}
              </span>
            </td>
            <td class="whitespace-nowrap text-right">
              <button class="btn btn-ghost btn-sm" @click="editing = { ...coupon }">编辑</button>
              <button class="btn btn-ghost btn-sm text-[var(--danger)]" @click="remove(coupon)">删除</button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>

    <div
      v-if="editing"
      class="overlay" role="dialog" aria-modal="true" aria-label="优惠券表单"
      @click.self="editing = null"
    >
      <div class="card w-full max-w-md !p-5">
        <h3 class="text-base font-semibold">{{ editing.id ? '编辑优惠券' : '新建优惠券' }}</h3>
        <div class="mt-4 space-y-3">
          <div>
            <label class="label" for="k-code">优惠码 *</label>
            <input id="k-code" v-model="editing.code" class="input mono uppercase" placeholder="SUMMER10" />
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-type">类型</label>
              <select id="k-type" v-model="editing.discount_type" class="input">
                <option value="fixed">固定金额</option>
                <option value="percent">百分比</option>
              </select>
            </div>
            <div>
              <label class="label" for="k-value">{{ editing.discount_type === 'percent' ? '折扣（%）' : '立减（元）' }} *</label>
              <input id="k-value" v-model.number="editing.discount_value" type="number" min="1" class="input nums" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-min">最低消费</label>
              <input id="k-min" v-model.number="editing.min_order_amount" type="number" min="0" class="input nums" />
            </div>
            <div>
              <label class="label" for="k-max">最大次数（0=不限）</label>
              <input id="k-max" v-model.number="editing.max_uses" type="number" min="0" class="input nums" />
            </div>
          </div>
          <div class="grid grid-cols-2 gap-3">
            <div>
              <label class="label" for="k-from">生效日期</label>
              <input
                id="k-from"
                :value="toDateInput(editing.valid_from)"
                type="date"
                class="input"
                @input="editing.valid_from = fromDateInput(($event.target as HTMLInputElement).value)"
              />
            </div>
            <div>
              <label class="label" for="k-until">失效日期</label>
              <input
                id="k-until"
                :value="toDateInput(editing.valid_until)"
                type="date"
                class="input"
                @input="editing.valid_until = fromDateInput(($event.target as HTMLInputElement).value)"
              />
            </div>
          </div>
          <div v-if="editing.id" class="hint">已使用 {{ editing.used_count }} 次 · 创建于 {{ when(editing.created_at) }}</div>
          <label class="flex items-center gap-2.5 text-sm">
            <input v-model="editing.is_active" type="checkbox" class="accent-[var(--accent)]" />
            启用
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
