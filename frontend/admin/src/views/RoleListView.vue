<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { listPermissions, listRoles, saveRolePermissions } from '../api/permissions'
import type { PermissionGroup, RoleRow } from '../api/permissions'
import { errorMessage } from '../utils/format'
import { useAuthStore } from '../stores/auth'

const auth = useAuthStore()

const loading = ref(true)
const busy = ref('')
const error = ref('')
const notice = ref('')
const groups = ref<PermissionGroup[]>([])
const roles = ref<RoleRow[]>([])

// drafts holds what the checkboxes say, not what the database holds, so a role
// can be edited and abandoned without touching live policies.
const drafts = ref<Record<string, string[]>>({})
const open = ref<string>('')

const canManage = computed(() => auth.allows('roles', 'manage'))

function grantsOf(role: RoleRow | undefined): string[] {
  return role ? role.permissions ?? [] : []
}

function hydrate() {
  const next: Record<string, string[]> = {}
  for (const role of roles.value) next[role.role] = [...grantsOf(role)]
  drafts.value = next
}

function holds(role: string, permission: string): boolean {
  return drafts.value[role]?.includes(permission) ?? false
}

function countFor(role: string): number {
  return drafts.value[role]?.length ?? 0
}

function changed(role: RoleRow): boolean {
  const saved = new Set(grantsOf(role))
  const draft = drafts.value[role.role] ?? []
  return draft.length !== saved.size || draft.some((permission) => !saved.has(permission))
}

function toggle(role: string, permission: string) {
  if (!canManage.value) return
  const current = drafts.value[role] ?? []
  drafts.value = {
    ...drafts.value,
    [role]: current.includes(permission)
      ? current.filter((item) => item !== permission)
      : [...current, permission],
  }
}

function toggleGroup(role: string, group: PermissionGroup) {
  if (!canManage.value) return
  const all = group.actions.map((action) => `${group.resource}:${action}`)
  const done = all.every((permission) => holds(role, permission))
  const current = drafts.value[role] ?? []
  drafts.value = {
    ...drafts.value,
    [role]: done
      ? current.filter((permission) => !all.includes(permission))
      : [...new Set([...current, ...all])],
  }
}

function groupState(role: string, group: PermissionGroup): 'none' | 'some' | 'all' {
  const all = group.actions.map((action) => `${group.resource}:${action}`)
  const on = all.filter((permission) => holds(role, permission)).length
  if (!on) return 'none'
  return on === all.length ? 'all' : 'some'
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    const [permissionGroups, roleRows] = await Promise.all([listPermissions(), listRoles()])
    groups.value = permissionGroups
    roles.value = roleRows
    hydrate()
    if (!open.value) open.value = roleRows.find((row) => row.editable)?.role ?? roleRows[0]?.role ?? ''
  } catch (err) {
    error.value = errorMessage(err, '加载角色权限失败')
  } finally {
    loading.value = false
  }
}

async function save(role: RoleRow) {
  if (!canManage.value || busy.value) return
  busy.value = role.role
  error.value = ''
  notice.value = ''
  try {
    const result = await saveRolePermissions(role.role, drafts.value[role.role] ?? [])
    const index = roles.value.findIndex((item) => item.role === result.role)
    if (index >= 0) roles.value[index] = { ...roles.value[index], permissions: result.permissions ?? [] }
    hydrate()
    notice.value = `已保存「${role.label}」的权限，成员下一次请求即生效。`
  } catch (err) {
    error.value = errorMessage(err, '保存角色权限失败')
  } finally {
    busy.value = ''
  }
}

function reset(role: RoleRow) {
  drafts.value = { ...drafts.value, [role.role]: [...grantsOf(role)] }
}

onMounted(load)
</script>

<template>
  <section class="space-y-4">
    <p class="quiet text-sm leading-relaxed">
      细分权限决定每个角色在后台能看到哪些页面、能做哪些改动。改动会立刻对全部该角色成员生效；超级管理员固定拥有全部权限，
      不可编辑，以免把自己锁在店铺之外。
    </p>

    <p v-if="!canManage" class="alert alert-warning" role="status">
      当前角色可以查看权限矩阵，但没有「角色权限」的修改权，需要管理员调整。
    </p>
    <p v-if="error" class="alert alert-danger" role="alert">{{ error }}</p>
    <p v-if="notice" class="alert alert-success" role="status">{{ notice }}</p>

    <div v-if="loading" class="space-y-3">
      <div v-for="i in 3" :key="i" class="skeleton h-24" />
    </div>

    <div v-else class="space-y-3">
      <div v-for="role in roles" :key="role.role" class="card !p-0">
        <button
          class="flex w-full items-center gap-3 px-5 py-4 text-left"
          :aria-expanded="open === role.role"
          @click="open = open === role.role ? '' : role.role"
        >
          <span class="min-w-0">
            <span class="block text-sm font-semibold">{{ role.label }}</span>
            <span class="mono quiet block truncate text-xs">{{ role.role }}</span>
          </span>
          <span class="ml-auto flex items-center gap-2">
            <span v-if="!role.editable" class="badge badge-info">全部权限</span>
            <span v-else-if="changed(role)" class="badge badge-warning">未保存</span>
            <span v-else class="badge badge-neutral">{{ countFor(role.role) }} 项</span>
            <span class="quiet text-xs">{{ open === role.role ? '收起' : '展开' }}</span>
          </span>
        </button>

        <div v-if="open === role.role" class="border-t border-[var(--stroke)] px-5 py-4">
          <p v-if="!role.editable" class="quiet text-sm">
            超级管理员的策略是通配符 <span class="mono">*:*</span>，不参与勾选，也无法被保存覆盖。
          </p>

          <div v-else class="grid gap-2 sm:grid-cols-2">
            <div
              v-for="group in groups"
              :key="group.resource"
              class="card-quiet flex items-center gap-3 !p-3"
            >
              <button
                v-if="group.actions.length > 1"
                class="btn btn-ghost btn-sm shrink-0"
                :disabled="!canManage"
                :title="groupState(role.role, group) === 'all' ? '取消整组' : '勾选整组'"
                @click="toggleGroup(role.role, group)"
              >
                {{ groupState(role.role, group) === 'all' ? '−' : '+' }}
              </button>
              <span class="min-w-0 flex-1">
                <span class="block text-[13px] font-medium">{{ group.label }}</span>
                <span class="mono quiet block truncate text-[11px]">{{ group.resource }}</span>
              </span>
              <label
                v-for="action in group.actions"
                :key="action"
                class="flex shrink-0 items-center gap-1.5 text-xs"
              >
                <input
                  type="checkbox"
                  class="accent-[var(--accent)]"
                  :checked="holds(role.role, `${group.resource}:${action}`)"
                  :disabled="!canManage"
                  @change="toggle(role.role, `${group.resource}:${action}`)"
                />
                <span class="quiet">{{ action === 'view' ? '查看' : '管理' }}</span>
              </label>
            </div>
          </div>

          <div v-if="role.editable" class="mt-4 flex items-center gap-2">
            <button
              class="btn btn-primary btn-sm"
              :disabled="!canManage || busy === role.role || !changed(role)"
              @click="save(role)"
            >
              {{ busy === role.role ? '保存中…' : '保存该角色' }}
            </button>
            <button
              class="btn btn-quiet btn-sm"
              :disabled="!changed(role) || busy === role.role"
              @click="reset(role)"
            >
              撤销改动
            </button>
            <span class="quiet ml-auto text-xs">{{ countFor(role.role) }} 项已勾选</span>
          </div>
        </div>
      </div>
    </div>
  </section>
</template>
