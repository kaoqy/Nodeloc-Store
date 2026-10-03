<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '../stores/auth'

// 统一配置操作栏：所有设置类页面共用同一条操作栏，保证「保存」永远在同一处、
// 永远可见。以前每个页面各写一份，才会出现有的页面把保存放到被裁掉的头部、
// 有的页面干脆没有保存入口。
const props = withDefaults(
  defineProps<{
    /** 权限资源名，例如 ai / settings / config_center */
    resource: string
    saving?: boolean
    /** 是否有未保存的改动；false 时保存按钮禁用但可见 */
    dirty?: boolean
    /** 保存按钮文案，默认「保存」 */
    saveLabel?: string
    /** 已保存的说明，例如「已保存 AI 配置」 */
    hint?: string
    /** 是否显示取消修改 */
    cancellable?: boolean
    /** 是否显示恢复默认（由父组件处理 reset） */
    resettable?: boolean
    /** 是否显示测试按钮（由父组件处理 test） */
    testable?: boolean
    testLabel?: string
    testing?: boolean
  }>(),
  {
    saving: false,
    dirty: true,
    saveLabel: '保存',
    hint: '',
    cancellable: false,
    resettable: false,
    testable: false,
    testLabel: '测试配置',
    testing: false,
  },
)

const emit = defineEmits<{
  save: []
  cancel: []
  reset: []
  test: []
}>()

const auth = useAuthStore()
const canManage = computed(() => auth.allows(props.resource, 'manage'))
const disabled = computed(() => props.saving || !props.dirty)
</script>

<template>
  <div
    class="sticky top-[72px] z-10 flex flex-wrap items-center gap-2 rounded-[var(--radius-sm)] border border-[var(--stroke)] bg-[var(--glass)] px-3 py-2 backdrop-blur"
    role="toolbar"
    aria-label="配置操作"
  >
    <span class="quiet text-xs">
      {{ canManage ? (dirty ? '有未保存的改动' : '已是最新') : '只读' }}
      <template v-if="hint"> · {{ hint }}</template>
    </span>

    <div class="ml-auto flex flex-wrap gap-2">
      <button
        v-if="testable && canManage"
        type="button"
        class="btn btn-quiet btn-sm"
        :disabled="testing"
        @click="emit('test')"
      >
        {{ testing ? '测试中…' : testLabel }}
      </button>
      <button
        v-if="resettable && canManage"
        type="button"
        class="btn btn-quiet btn-sm"
        :disabled="saving"
        @click="emit('reset')"
      >
        恢复默认
      </button>
      <button
        v-if="cancellable && canManage"
        type="button"
        class="btn btn-secondary btn-sm"
        :disabled="saving || !dirty"
        @click="emit('cancel')"
      >
        取消修改
      </button>
      <button
        type="button"
        class="btn btn-primary btn-sm"
        :disabled="disabled || !canManage"
        @click="emit('save')"
      >
        {{ saving ? '保存中…' : saveLabel }}
      </button>
    </div>
  </div>
</template>
