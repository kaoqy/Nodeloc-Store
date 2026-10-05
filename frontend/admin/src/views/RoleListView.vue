<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AdminIcon from '../components/AdminIcon.vue'
import PageHeader from '../components/PageHeader.vue'
import { listPermissions, listRoles, saveRolePermissions } from '../api/permissions'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'

/**
 * 权限与管理员：每个角色能看到哪些页面、能做哪些操作。
 *
 * 权限矩阵直接按后端返回的权限目录渲染，勾选即保存；super_admin 固定拥有
 * 全部权限，不提供编辑（避免把唯一的管理员锁在门外）。
 */

const auth = useAuthStore()
const canManage = computed(() => auth.allows('roles', 'manage'))

const loading = ref(true)
const busy = ref(false)
const error = ref('')
const notice = ref('')
const groups = ref<{ resource: string; label: string; actions: string[] }[]>([])
const roles = ref<{ role: string; label: string; permissions: string[]; headcount: number; editable: boolean }[]>([])
const active = ref('')

// 只有两个角色。历史角色如果还留在数据里，统一显示为「管理员（旧角色）」，
// 让店主知道可以把它改成 admin。
const ROLE_LABELS: Record<string, string> = {
  super_admin: '超级管理员',
  admin: '管理员',
}
const LEGACY_LABELS: Record<string, string> = {
  operator: '运营（旧）', support: '客服（旧）', ops_manager: '运营管理员（旧）',
  product_manager: '商品管理员（旧）', order_manager: '订单管理员（旧）',
  finance: '财务（旧）', support_lead: '客服主管（旧）', support_agent: '普通客服（旧）',
  ai_admin: '管理员（旧）', data_viewer: '数据查看员（旧）',
}

const draft = ref<Record<string, Set<string>>>({})

const activeRole = computed(() => roles.value.find((r) => r.role === active.value) ?? null)
const isSuper = computed(() => active.value === 'super_admin')
const dirty = computed(() => {
  const role = activeRole.value
  if (!role) return false
  const current = draft.value[role.role]
  if (!current) return false
  const saved = new Set(role.permissions)
  if (saved.size !== current.size) return true
  for (const item of current) if (!saved.has(item)) return true
  return false
})

function key(resource: string, action: string) {
  return resource + ':' + action
}

function has(permission: string): boolean {
  return draft.value[active.value]?.has(permission) ?? false
}

function toggle(permission: string) {
  if (!canManage.value || isSuper.value) return
  const set = draft.value[active.value]
  if (!set) return
  if (set.has(permission)) set.delete(permission)
  else set.add(permission)
  // 触发响应式：Set 的增删不会自动通知 computed
  draft.value = { ...draft.value, [active.value]: new Set(set) }
}

/** 整组勾选/取消：常见需求是「给运营加一组订单相关的权限」。 */
function toggleGroup(resource: string) {
  if (!canManage.value || isSuper.value) return
  const set = draft.value[active.value]
  if (!set) return
  const group = groups.value.find((g) => g.resource === resource)
  if (!group) return
  const allOn = group.actions.every((a) => set.has(key(resource, a)))
  for (const action of group.actions) {
    if (allOn) set.delete(key(resource, action))
    else set.add(key(resource, action))
  }
  draft.value = { ...draft.value, [active.value]: new Set(set) }
}

function groupState(resource: string): 'all' | 'none' | 'some' {
  const group = groups.value.find((g) => g.resource === resource)
  if (!group) return 'none'
  const set = draft.value[active.value]
  if (!set) return 'none'
  const on = group.actions.filter((a) => set.has(key(resource, a))).length
  if (on === 0) return 'none'
  if (on === group.actions.length) return 'all'
  return 'some'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [groupList, roleList] = await Promise.all([listPermissions(), listRoles()])
    groups.value = groupList
    roles.value = roleList.map((r) => ({
      role: r.role,
      label: ROLE_LABELS[r.role] ?? LEGACY_LABELS[r.role] ?? r.role,
      permissions: r.permissions ?? [],
      headcount: r.user_count ?? 0,
      editable: r.editable !== false,
    }))
    draft.value = Object.fromEntries(roleList.map((r) => [r.role, new Set(r.permissions)]))
    if (!active.value && roles.value.length) active.value = roles.value[0].role
  } catch (err) {
    error.value = errorMessage(err, '加载权限失败')
  } finally {
    loading.value = false
  }
}

async function save() {
  if (!activeRole.value) return
  busy.value = true
  error.value = ''
  notice.value = ''
  try {
    const permissions = [...(draft.value[active.value] ?? new Set<string>())]
    await saveRolePermissions(active.value, permissions)
    notice.value = '「' + activeRole.value.label + '」的权限已保存。'
    await load()
  } catch (err) {
    error.value = errorMessage(err, '保存权限失败')
  } finally {
    busy.value = false
  }
}

function reset() {
  const role = activeRole.value
  if (!role) return
  draft.value = { ...draft.value, [role.role]: new Set(role.permissions) }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <PageHeader title="权限与管理员" description="每个角色能看到与能操作的范围。改动立即对持有该角色的账号生效。" bordered>
      <template #actions>
        <button v-if="canManage && !isSuper" class="btn btn-quiet btn-sm" :disabled="!dirty || busy" @click="reset">
          取消修改
        </button>
        <button
          v-if="canManage && !isSuper"
          class="btn btn-primary btn-sm"
          :disabled="!dirty || busy"
          @click="save"
        >
          <span v-if="busy" class="spinner !size-3.5" />
          {{ busy ? '保存中…' : '保存权限' }}
        </button>
      </template>
    </PageHeader>

    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="!canManage" class="alert" role="status">当前角色只能查看权限矩阵，修改需要「角色权限」管理权限。</p>

    <div class="grid gap-4 lg:grid-cols-[240px_minmax(0,1fr)]">
      <nav class="card !p-2" aria-label="角色列表">
        <ul class="space-y-0.5">
          <li v-for="role in roles" :key="role.role">
            <button
              class="role-item w-full text-left"
              :class="active === role.role ? 'role-item-active' : ''"
              @click="active = role.role"
            >
              <span class="min-w-0 flex-1">
                <span class="block truncate text-[13px]">{{ role.label }}</span>
                <span class="quiet block truncate text-[11px]">
                  {{ role.permissions.length }} 项权限
                  <span v-if="role.headcount"> · {{ role.headcount }} 人</span>
                </span>
              </span>
            </button>
          </li>
        </ul>
      </nav>

      <div class="min-w-0 space-y-3">
        <div v-if="loading" class="card space-y-2">
          <div v-for="i in 6" :key="i" class="skeleton h-9 w-full" />
        </div>

        <div v-else-if="error && !activeRole" class="card py-12 text-center">
          <p class="font-semibold">权限目录暂时不可用</p>
          <p class="quiet mt-1 text-sm">{{ error }}</p>
          <button class="btn btn-secondary btn-sm mt-5" :disabled="loading" @click="load">重新加载</button>
        </div>

        <template v-else-if="activeRole">
          <div class="card space-y-1">
            <div class="flex flex-wrap items-center gap-2">
              <h3 class="text-[15px] font-bold">{{ activeRole.label }}</h3>
              <span v-if="isSuper" class="badge-accent">固定拥有全部权限</span>
              <span v-if="dirty" class="badge-warning">有未保存的改动</span>
            </div>
            <p class="quiet text-xs">
              勾选表示该角色可以执行这个操作。隐藏菜单不等于禁止调用，接口会用同一套权限再校验一次。
            </p>
          </div>

          <div class="grid gap-3 sm:grid-cols-2">
            <div v-for="group in groups" :key="group.resource" class="card space-y-2.5">
              <div class="flex items-center justify-between gap-2">
                <p class="text-[13.5px] font-semibold">{{ group.label }}</p>
                <button
                  v-if="canManage && !isSuper"
                  class="hint"
                  type="button"
                  @click="toggleGroup(group.resource)"
                >
                  {{ groupState(group.resource) === 'all' ? '取消全选' : '全选' }}
                </button>
                <span v-else-if="isSuper" class="hint">全部</span>
              </div>
              <div class="flex flex-wrap gap-1.5">
                <button
                  v-for="action in group.actions"
                  :key="action"
                  class="chip"
                  :class="has(key(group.resource, action)) ? 'chip-active' : ''"
                  :disabled="!canManage || isSuper"
                  @click="toggle(key(group.resource, action))"
                >
                  <AdminIcon v-if="has(key(group.resource, action))" name="check" :size="12" />
                  {{ action === 'view' ? '查看' : action === 'manage' ? '管理' : action === 'export' ? '导出' : action === 'assign' ? '分配' : action }}
                </button>
              </div>
            </div>
          </div>
        </template>
      </div>
    </div>
  </section>
</template>

<style scoped>
.role-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: var(--radius-sm);
  color: var(--text-dim);
  transition: background var(--fast), color var(--fast);
}
.role-item:hover { background: var(--surface-hi); color: var(--text); }
.role-item-active { background: var(--accent-soft); color: var(--accent); font-weight: 600; }
</style>
