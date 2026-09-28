import client from './client'

export interface PermissionGroup {
  resource: string
  label: string
  actions: string[]
}

export interface RoleRow {
  role: string
  label: string
  editable: boolean
  permissions: string[]
}

export const listPermissions = () =>
  client.get<{ data: PermissionGroup[] }>('/admin/permissions').then((r) => r.data.data ?? [])

export const listRoles = () =>
  client.get<{ data: RoleRow[] }>('/admin/roles').then((r) => r.data.data ?? [])

// The whole grant list is replaced in one write, so a half-saved matrix cannot
// survive a request that fails partway through.
export const saveRolePermissions = (role: string, permissions: string[]) =>
  client
    .put<{ ok: boolean; role: string; permissions: string[] }>(`/admin/roles/${role}/permissions`, { permissions })
    .then((r) => r.data)
