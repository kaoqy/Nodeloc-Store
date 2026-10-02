<script setup lang="ts">
import { computed } from 'vue'
import { useAuthStore } from '../stores/auth'

// 配置中心每一块的统一外壳：标题、说明、权限提示都长在同一处，
// 新增一块配置时只需再写一个 panel，不必重新发明页面头部。
const props = withDefaults(
  defineProps<{
    title: string
    description?: string
    resource: string
    actions?: boolean
  }>(),
  { description: '', actions: true },
)

const auth = useAuthStore()
const canManage = computed(() => auth.allows(props.resource, 'manage'))
</script>

<template>
  <div class="space-y-4">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div>
        <p class="text-base font-bold">{{ title }}</p>
        <p v-if="description" class="quiet mt-1 text-xs leading-relaxed">{{ description }}</p>
      </div>
      <div v-if="$slots.actions" class="flex flex-wrap gap-1.5">
        <slot name="actions" />
      </div>
    </div>
    <p v-if="actions && !canManage" class="alert" role="status">
      当前角色只能查看这一块配置，修改需要「{{ resource }}」管理权限。
    </p>
    <slot />
  </div>
</template>
