<script setup lang="ts">
import { ref } from 'vue'

const open = ref('buy')
const sections = [
  { id: 'buy', title: '如何购买商品？', body: '在首页搜索或选择分类，打开商品详情页确认价格、库存和交付方式，填写必要信息后点击立即购买。未登录时会先进入登录流程。' },
  { id: 'payment', title: '支付完成后在哪里查看？', body: '支付完成后回到订单详情页。商店会以服务端确认结果为准自动核实支付状态；请不要因为页面暂未更新而重复付款。' },
  { id: 'delivery', title: '卡密什么时候发放？', body: '自动发货商品在支付确认且有可用库存后交付。若库存暂时不足，订单会显示等待补货；人工交付商品则由商家在订单中完成发货。' },
  { id: 'oauth', title: 'NodeLoc 登录失败怎么办？', body: '请从登录页重新发起一次授权。若持续失败，请将后台 OAuth 登录记录中的时间、阶段和错误编号提供给店家，不要提供 Client Secret、授权码或 Token。' },
  { id: 'after-sale', title: '遇到订单问题怎么办？', body: '先打开对应订单查看支付和交付状态。未支付订单可以继续支付，已支付但未交付的订单请等待自动重试或联系页面中配置的店家联系方式。' },
]
</script>

<template>
  <div class="mx-auto w-full max-w-4xl px-4 py-10 sm:px-6">
    <header class="mb-8">
      <p class="eyebrow">帮助中心</p>
      <h1 class="mt-2 text-3xl font-bold tracking-tight">购买、支付与交付</h1>
      <p class="mt-3 max-w-2xl text-sm leading-relaxed text-[var(--text-dim)]">
        这里整理当前商店真实支持的购买流程、NodeLoc 登录、支付核实和卡密交付说明。
      </p>
    </header>

    <div class="grid gap-3">
      <article v-for="item in sections" :key="item.id" class="card overflow-hidden !p-0">
        <button
          class="flex min-h-14 w-full items-center justify-between gap-4 px-5 py-4 text-left font-semibold transition-colors hover:bg-[var(--surface-hi)]"
          type="button"
          :aria-expanded="open === item.id"
          @click="open = open === item.id ? '' : item.id"
        >
          <span>{{ item.title }}</span>
          <span class="mono text-lg text-[var(--text-quiet)]" aria-hidden="true">{{ open === item.id ? '−' : '+' }}</span>
        </button>
        <p v-if="open === item.id" class="border-t border-[var(--stroke-quiet)] px-5 py-4 text-sm leading-7 text-[var(--text-dim)]">
          {{ item.body }}
        </p>
      </article>
    </div>
  </div>
</template>
