import client from './client'

export interface PluginFormField {
  key: string
  label: string
  type: 'text' | 'number'
  required: boolean
  placeholder?: string
  help?: string
  max_length?: number
}

export interface ProductPluginDescriptor {
  plugin_key: string
  plugin_name: string
  requires_form: boolean
  mapping_value?: string
  options?: string[]
  form_schema?: PluginFormField[]
  quota_per_nl?: string
  delivery_note?: string
}

/**
 * Public, read-only product plugin descriptor. It is safe before login and
 * contains no provider credentials.
 */
export async function getProductPlugin(productId: number): Promise<ProductPluginDescriptor | null> {
  const { data } = await client.get<{ data: ProductPluginDescriptor | null }>(
    `/products/${productId}/plugin`,
  )
  return data.data ?? null
}
