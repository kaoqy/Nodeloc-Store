<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { listActivities, type Activity } from '../api/support'
import { errorMessage } from '../api/client'

// 首页活动横幅：只展示进行中、允许在首页露出的活动。
// 它只讲「店里在促销什么」，不承担客服入口；活动款式与计价均由服务端计算。
const loading = ref(true)
const error = ref('')
const activities = ref<Activity[]>([])

const typeLabels: Record<string, string> = {
  limited_discount: '限时折扣',
  store_discount: '全场折扣',
  product_discount: '商品折扣',
  category_discount: '分类折扣',
  full_reduction: '满减',
  full_quantity: '满件优惠',
  bulk_discount: '批量优惠',
  coupon_activity: '优惠码',
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
    const list = await listActivities()
    activities.value = list.slice(0, 3)
  } catch (err) {
    error.value = errorMessage(err, '活动加载失败')
  } finally {
    loading.value = false
  }
}

onMounted(load)
</script>

<template>
  <section
    v-if="loading || activities.length || error"
    class="rise-in mt-10"
    aria-labelledby="home-promo-heading"
  >
    <div class="mb-3 flex flex-wrap items-end justify-between gap-2">
      <div>
        <p class="eyebrow">活动中心</p>
        <h2 id="home-promo-heading" class="mt-1.5 text-xl font-bold">正在进行的优惠</h2>
      </div>
      <RouterLink to="/activities" class="hint transition-colors hover:text-[var(--accent)]">
        查看全部活动 →
      </RouterLink>
    </div>

    <div v-if="loading" class="grid gap-4 lg:grid-cols-3">
      <div v-for="i in 3" :key="i" class="card space-y-3">
        <div class="skeleton h-5 w-1/3" />
        <div class="skeleton h-4 w-full" />
        <div class="skeleton h-4 w-2/3" />
      </div>
    </div>

    <p v-else-if="error" class="card quiet text-sm">{{ error }}</p>

    <div v-else class="grid gap-4 lg:grid-cols-3">
      <article
        v-for="activity in activities"
        :key="activity.id"
        class="card promo-card space-y-3"
      >
        <div class="flex items-center gap-2">
          <span class="badge-warning">{{ typeLabel(activity.type) }}</span>
          <span v-if="activity.end_at" class="hint ml-auto nums">至 {{ activity.end_at.slice(0, 10) }}</span>
        </div>
        <h3 class="text-base font-semibold">{{ activity.name }}</h3>
        <p v-if="activity.subtitle" class="text-sm text-[var(--text-dim)]">{{ activity.subtitle }}</p>
        <p v-else-if="activity.description" class="line-clamp-2 text-sm text-[var(--text-dim)]">
          {{ activity.description }}
        </p>
        <RouterLink to="/activities" class="btn btn-secondary btn-sm mt-1 w-fit">去看看</RouterLink>
      </article>
    </div>
  </section>
</template>
