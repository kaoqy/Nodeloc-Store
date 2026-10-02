import client from './client'
import type { Plugin, PluginBinding, PluginCatalogEntry, PluginConfigField } from '../types'

/** Every provider this build carries, joined with what the shop enrolled. */
export const listPluginCatalog = () =>
  client.get<{ data: PluginCatalogEntry[] }>('/plugins').then((r) => r.data.data ?? [])

/** Only the plugins the shop has enrolled. */
export const listInstalledPlugins = () =>
  client.get<{ data: Plugin[] }>('/plugins/installed').then((r) => r.data.data ?? [])

/** One provider's configuration form, so 设置 stays a generic screen. */
export const pluginSchema = (key: string) =>
  client.get<{ data: PluginConfigField[] }>(`/plugins/${key}/schema`).then((r) => r.data.data ?? [])

export const enrollPlugin = (key: string) =>
  client.post<{ data: Plugin }>('/plugins', { key }).then((r) => r.data.data)

export const enablePlugin = (id: number) =>
  client.post<{ data: Plugin }>(`/admin/plugins/${id}/enable`).then((r) => r.data.data)

export const disablePlugin = (id: number) =>
  client.post<{ data: Plugin }>(`/admin/plugins/${id}/disable`).then((r) => r.data.data)

/**
 * Writes a plugin's configuration. Secrets are write-only: an empty value keeps
 * whatever is stored, and the sentinel '__clear__' removes one deliberately.
 */
export const updatePluginConfig = (
  id: number,
  settings: Record<string, string>,
  secrets: Record<string, string>,
) => client.put<{ data: Plugin }>(`/admin/plugins/${id}/config`, { settings, secrets }).then((r) => r.data.data)

export const uninstallPlugin = (id: number) => client.delete(`/admin/plugins/${id}`)

export const listBindings = (params: { plugin_id?: number; product_id?: number } = {}) =>
  client
    .get<{ data: PluginBinding[] }>('/plugins/bindings', { params })
    .then((r) => r.data.data ?? [])

export const saveBinding = (payload: Partial<PluginBinding>) =>
  payload.id
    ? client.put<{ data: PluginBinding }>(`/plugins/bindings/${payload.id}`, payload).then((r) => r.data.data)
    : client.post<{ data: PluginBinding }>('/plugins/bindings', payload).then((r) => r.data.data)

export const deleteBinding = (id: number) => client.delete(`/plugins/bindings/${id}`)
