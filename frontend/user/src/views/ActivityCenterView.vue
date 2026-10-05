<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listActivities, claimCoupon, type Activity } from '../api/support'
import { errorMessage } from '../api/client'
import { when } from '../utils/format'

// 活动中心是独立的买家页面，只讲促销。它和工单中心没有任何共用页签：
// 买家想找优惠来这里，想找人解决问题去 /support，两边互不干扰。
const loading = ref(true)
const error = ref('')
const notice = ref('')
const activities = ref<Activity[]>([])
const claiming = ref('')
const typeFilter = ref('all')

const typeLabels: Record<string, string> = {
  limited_discount: '限时折扣',
  store_discount: '全场折扣',
  product_discount: '商品折扣',
  category_discount: '分类折扣',
  full_reduction: '满减',
  full_quantity: '满件优惠',
  bulk_discount: '批量优惠',
  coupon_activity: '优惠码活动',
  new_user: '新人专享',
  first_purchase: '首购优惠',
  member_only: '会员专享',
  limited_activity: '限量活动',
  seckill: '秒杀',
  coupon_claim: '领券活动',
}

function typeLabel(value: string) {
  return typeLabels[value] || '活动'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    activities.value = await listActivities()
  } catch (err) {
    error.value = errorMessage(err, '加载活动失败')
  } finally {
    loading.value = false
  }
}

async function claim(activity: Activity) {
  claiming.value = String(activity.id)
  error.value = ''
  notice.value = ''
  try {
    await claimCoupon(activity.id)
    notice.value = '「' + activity.name + '」的优惠券已领取，下单时填写对应优惠码即可抵扣。'
  } catch (err) {
    error.value = errorMessage(err, '领取失败')
  } finally {
    claiming.value = ''
  }
}

onMounted(load)
</script>

<template>
  <div class="mx-auto w-full max-w-6xl px-4 py-10 sm:px-6">
    <header>
      <p class="eyebrow">活动中心</p>
      <h1 class="mt-2 text-2xl font-bold sm:text-3xl">正在进行的优惠</h1>
      <p class="mt-3 max-w-2xl text-sm leading-relaxed text-[var(--text-dim)]">
        活动折扣在结算时由服务端按规则计算，优惠码在商品详情页或结算页填写。
        同一个订单默认取优惠力度最大的一项。
      </p>
    </header>

    <p v-if="error" class="alert alert-danger mt-6" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success mt-6" role="status">{{ notice }}</p>

    <div v-if="loading" class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      <div v-for="i in 6" :key="i" class="card space-y-3">
        <div class="skeleton aspect-[16/9] w-full" />
        <div class="skeleton h-5 w-2/3" />
        <div class="skeleton h-4 w-full" />
      </div>
    </div>

    <div v-else-if="!activities.length" class="card mt-8 py-20 text-center">
      <p class="text-[var(--text-quiet)]" aria-hidden="true">◎</p>
      <p class="mt-3 font-semibold">暂时没有进行中的活动</p>
      <p class="mt-1.5 text-sm text-[var(--text-quiet)]">有新的促销活动时会显示在这里。</p>
      <RouterLink to="/" class="btn btn-secondary btn-sm mt-6">去逛商品</RouterLink>
    </div>

    <div v-else class="mt-8 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
      <article v-for="activity in activities" :key="activity.id" class="card space-y-3">
        <img
          v-if="activity.cover_image"
          :src="activity.cover_image"
          :alt="activity.name"
          class="aspect-[16/9] w-full rounded-[var(--radius-sm)] object-cover"
        />
        <div class="flex flex-wrap items-center gap-2">
          <span class="badge-warning">{{ typeLabel(activity.type) }}</span>
          <span v-if="activity.end_at" class="hint ml-auto nums">至 {{ activity.end_at.slice(0, 10) }}</span>
        </div>
        <h2 class="text-base font-semibold">{{ activity.name }}</h2>
        <p v-if="activity.subtitle" class="text-sm text-[var(--text-dim)]">{{ activity.subtitle }}</p>
        <p v-if="activity.description" class="line-clamp-3 text-sm text-[var(--text-dim)]">{{ activity.description }}</p>
        <p class="hint">
          <span v-if="activity.start_at">开始 {{ when(activity.start_at) }}</span>
          <span v-if="activity.end_at"> · 结束 {{ when(activity.end_at) }}</span>
        </p>
        <button
          v-if="activity.type === 'coupon_claim'"
          class="btn btn-primary btn-sm w-fit"
          :disabled="claiming === String(activity.id)"
          @click="claim(activity)"
        >
          {{ claiming === String(activity.id) ? '领取中…' : '领取优惠券' }}
        </button>
        <RouterLink v-else to="/" class="btn btn-secondary btn-sm w-fit">去看看</RouterLink>
      </article>
    </div>
  </div>
</template>
