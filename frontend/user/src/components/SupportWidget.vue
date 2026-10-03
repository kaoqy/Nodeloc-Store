<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { createTicket, type Ticket } from '../api/support'
import { errorMessage } from '../api/client'
import { useAuthStore } from '../stores/auth'

/**
 * 在线客服窗口。
 *
 * 买家侧 AI 对话已下线，这里收敛成一个纯人工入口：写清楚问题、提交工单，
 * 剩下的由客服按顺序跟进。窗口只保留一个表单和一张「已提交」的提示，
 * 不再有对话流、工具确认与评价。
 */

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const open = ref(false)
const submitting = ref(false)
const error = ref('')
const subject = ref('')
const content = ref('')
const created = ref<Ticket | null>(null)
const panel = ref<HTMLElement | null>(null)

const agentName = '在线客服'

// 已登录才能提交：工单需要落到具体账号上，未登录时先引导去登录页。
const loginRequired = () => !auth.isAuthenticated

async function submit() {
  if (submitting.value || !canSubmit()) return
  if (loginRequired()) {
    void router.push({ path: '/login', query: { redirect: route.fullPath } })
    return
  }
  error.value = ''
  submitting.value = true
  try {
    const ticket = await createTicket({
      type: 'other',
      subject: subject.value.trim().slice(0, 40),
      content: content.value.trim(),
    })
    created.value = ticket
    subject.value = ''
    content.value = ''
  } catch (err) {
    error.value = errorMessage(err, '提交失败，请稍后重试。')
  } finally {
    submitting.value = false
  }
}

function canSubmit() {
  return subject.value.trim().length > 0 || content.value.trim().length > 0
}

function reset() {
  created.value = null
  error.value = ''
  subject.value = ''
  content.value = ''
}

async function openWidget() {
  open.value = true
  await nextTick()
  panel.value?.querySelector('textarea')?.focus?.()
}

onMounted(() => {
  window.addEventListener('nodeloc:open-support', openWidget)
})

onBeforeUnmount(() => {
  window.removeEventListener('nodeloc:open-support', openWidget)
})
</script>

<template>
  <div class="support-root print:hidden">
    <!-- 收起状态：一个圆形悬浮按钮 -->
    <button
      v-if="!open"
      class="support-fab"
      type="button"
      :aria-label="'打开' + agentName"
      @click="openWidget"
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" class="size-6" aria-hidden="true">
        <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v7a2.5 2.5 0 0 1-2.5 2.5H12l-4.5 3.4V16H6.5A2.5 2.5 0 0 1 4 13.5z" />
        <path d="M8.5 9.5h7M8.5 12.5h4.5" />
      </svg>
      <span class="support-fab-label">{{ agentName }}</span>
    </button>

    <!-- 展开状态：人工工单入口 -->
    <section
      v-else
      ref="panel"
      class="support-panel"
      role="dialog"
      aria-modal="false"
      :aria-label="agentName"
    >
      <header class="flex items-center gap-3 border-b border-[var(--stroke)] px-4 py-3">
        <span class="relative shrink-0">
          <span class="brand-mark">{{ agentName.slice(0, 1) }}</span>
          <span class="support-online" aria-hidden="true" />
        </span>
        <span class="min-w-0 flex-1">
          <span class="block truncate text-[13.5px] font-semibold">{{ agentName }}</span>
          <span class="quiet block truncate text-[11.5px]">提交问题，客服会尽快跟进</span>
        </span>
        <button class="support-close" type="button" aria-label="收起" @click="open = false">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" class="size-4" aria-hidden="true">
            <path d="M6 12h12" />
          </svg>
        </button>
      </header>

      <div class="flex-1 overflow-y-auto px-4 py-4">
        <!-- 提交成功：给出工单号与跟进入口 -->
        <div v-if="created" class="support-done">
          <p class="support-done-title">工单已提交</p>
          <p class="quiet mt-1 text-xs">
            工单号 <span class="mono">{{ created.ticket_no }}</span>，客服会按顺序处理。
          </p>
          <div class="mt-3 flex flex-wrap gap-2">
            <RouterLink to="/tickets" class="btn btn-primary btn-sm">查看我的工单</RouterLink>
            <button class="btn btn-secondary btn-sm" type="button" @click="reset">再提一个</button>
          </div>
        </div>

        <template v-else>
          <p class="support-lead">描述遇到的问题，提交后会生成一张工单，由客服跟进。</p>
          <label class="support-field" for="support-subject">
            <span class="support-label">问题标题</span>
            <input
              id="support-subject"
              v-model="subject"
              class="input"
              maxlength="60"
              placeholder="例如：卡密无法使用"
              @keydown.enter.exact.prevent="submit"
            />
          </label>
          <label class="support-field" for="support-content">
            <span class="support-label">详细描述</span>
            <textarea
              id="support-content"
              v-model="content"
              class="input min-h-[120px]"
              maxlength="2000"
              placeholder="把订单号、发生时间和提示信息写清楚，能更快定位问题。"
            />
          </label>
          <p class="quiet mt-1 text-[11.5px]">订单相关的工单可以带上订单号，客服会先核对交付记录。</p>
        </template>
      </div>

      <p v-if="error" class="alert alert-danger mx-4 mb-2 text-xs" role="alert">{{ error }}</p>

      <div v-if="!created" class="border-t border-[var(--stroke)] px-4 py-3">
        <button
          class="btn btn-primary w-full"
          :disabled="!canSubmit() || submitting"
          @click="submit"
        >
          {{ submitting ? '提交中…' : '提交工单' }}
        </button>
        <div class="mt-2 flex items-center gap-2 text-[11.5px]">
          <RouterLink v-if="auth.isAuthenticated" to="/tickets" class="support-action support-action-quiet ml-auto">
            我的工单
          </RouterLink>
          <button v-else class="support-action support-action-quiet ml-auto" type="button" @click="submit">
            登录后提交
          </button>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
/* ── 悬浮按钮 ─────────────────────────────────────────────────── */
.support-root {
  position: fixed;
  right: 20px;
  bottom: 20px;
  z-index: 40;
}
.support-fab {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 18px 12px 14px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--accent-line);
  background: linear-gradient(140deg, var(--accent-hi), var(--accent));
  color: var(--on-accent);
  font-size: 13.5px;
  font-weight: 600;
  box-shadow: var(--shadow-accent), var(--shadow-md);
  transition: transform 220ms var(--spring), box-shadow var(--normal);
}
.support-fab:hover { transform: translateY(-2px); box-shadow: var(--shadow-accent), var(--shadow-lg); }
.support-fab-label { white-space: nowrap; }

/* ── 对话面板 ─────────────────────────────────────────────────── */
.support-panel {
  display: flex;
  flex-direction: column;
  width: min(94vw, 400px);
  height: min(78vh, 620px);
  border: 1px solid var(--stroke);
  border-radius: var(--radius-lg);
  background: var(--surface);
  box-shadow: var(--shadow-lg);
  overflow: hidden;
  animation: support-in 240ms var(--spring);
}
@keyframes support-in {
  from { opacity: 0; transform: translateY(12px) scale(0.98); }
  to { opacity: 1; transform: none; }
}

.support-close {
  display: grid;
  place-items: center;
  width: 30px;
  height: 30px;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  border: 1px solid transparent;
  color: var(--text-quiet);
  transition: background var(--fast), color var(--fast), border-color var(--fast);
}
.support-close:hover { background: var(--surface-hi); border-color: var(--stroke); color: var(--text); }

.support-online {
  position: absolute;
  right: -1px;
  bottom: -1px;
  width: 10px;
  height: 10px;
  border-radius: 50%;
  border: 2px solid var(--surface);
  background: var(--success);
}

/* ── 气泡 ─────────────────────────────────────────────────────── */
.support-bubble {
  max-width: 84%;
  border-radius: var(--radius-md);
  padding: 9px 12px;
}
.support-bubble-ai {
  background: var(--surface-hi);
  border: 1px solid var(--stroke-quiet);
  border-bottom-left-radius: 4px;
}
.support-bubble-user {
  background: linear-gradient(140deg, var(--accent-hi), var(--accent));
  color: var(--on-accent);
  border-bottom-right-radius: 4px;
}
.support-bubble-system {
  max-width: 100%;
  background: var(--accent-soft);
  border: 1px solid var(--accent-line);
  color: var(--text-dim);
  font-size: 12.5px;
}

.support-meta {
  margin-top: 8px;
  padding-top: 8px;
  border-top: 1px dashed var(--stroke);
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  font-size: 11px;
}
.support-meta-label { color: var(--text-quiet); }
.support-chip {
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke);
  padding: 1px 8px;
  color: var(--text-dim);
}
.support-chip-warn { border-color: var(--warning); color: var(--warning); }

.support-confirm {
  margin-top: 8px;
  border: 1px solid var(--warning);
  border-radius: var(--radius-sm);
  background: var(--warning-soft, var(--accent-soft));
  padding: 8px 10px;
}
.support-confirm-title { font-size: 11.5px; font-weight: 700; color: var(--warning); }
.support-confirm-text { margin-top: 3px; font-size: 12px; line-height: 1.6; color: var(--text-dim); }

.support-rate {
  border-radius: var(--radius-pill);
  padding: 2px 8px;
  color: var(--text-quiet);
  transition: background var(--fast), color var(--fast);
}
.support-rate:hover { background: var(--surface-hi); color: var(--text-dim); }
.support-rate-on { background: var(--accent-soft); color: var(--accent); }

/* 输入中的三点动画 */
.support-typing { display: inline-flex; gap: 3px; }
.support-typing i {
  width: 4px; height: 4px; border-radius: 50%;
  background: var(--text-quiet);
  animation: support-blink 1.2s infinite ease-in-out;
}
.support-typing i:nth-child(2) { animation-delay: 0.15s; }
.support-typing i:nth-child(3) { animation-delay: 0.3s; }
@keyframes support-blink { 0%, 80%, 100% { opacity: 0.25; } 40% { opacity: 1; } }

/* ── 输入区 ───────────────────────────────────────────────────── */
.support-input {
  display: flex;
  align-items: flex-end;
  gap: 8px;
  border: 1px solid var(--stroke);
  border-radius: var(--radius-md);
  background: var(--surface-sunken);
  padding: 6px 6px 6px 12px;
  transition: border-color var(--fast);
}
.support-input:focus-within { border-color: var(--accent-line); }
.support-textarea {
  flex: 1;
  min-height: 34px;
  max-height: 120px;
  border: 0;
  background: transparent;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
  line-height: 1.6;
  resize: none;
  outline: none;
}
.support-send {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  flex-shrink: 0;
  border-radius: var(--radius-sm);
  background: var(--accent);
  color: var(--on-accent);
  transition: opacity var(--fast);
}
.support-send:disabled { opacity: 0.45; }

.support-action {
  border-radius: var(--radius-pill);
  border: 1px solid var(--stroke);
  padding: 3px 10px;
  color: var(--text-dim);
  transition: background var(--fast), border-color var(--fast), color var(--fast);
}
.support-action:hover { border-color: var(--accent-line); color: var(--accent); background: var(--accent-soft); }
.support-action-quiet { border-color: transparent; color: var(--text-quiet); }
.support-action:disabled { opacity: 0.5; }

/* 小屏：面板铺满可用宽度，按钮文字隐藏 */
@media (max-width: 480px) {
  .support-root { right: 12px; bottom: 12px; left: 12px; }
  .support-fab { width: 100%; justify-content: center; }
  .support-panel { width: 100%; height: min(80vh, 560px); }
  .support-fab-label { display: none; }
}

@media (prefers-reduced-motion: reduce) {
  .support-panel { animation: none; }
  .support-fab { transition: none; }
}
</style>
