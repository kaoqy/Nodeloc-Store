import client, { errorMessage } from './client'
import type { Category, CouponQuote, Product, ProductFormField, StorefrontCoupon, StoreStats } from '../types'

function normalizeProduct(product: Product & { form_schema?: string | ProductFormField[] | null }): Product {
  if (typeof product.form_schema === 'string') {
    try {
      product.form_schema = JSON.parse(product.form_schema) as ProductFormField[]
    } catch {
      product.form_schema = []
    }
  }
  return product as Product
}

/**
 * The storefront lists from the server, not from a copy of the whole catalogue:
 * a shop with real stock ships one page per visit, and 销量 sorting happens
 * where the numbers live.
 */
export interface ProductListParams {
  q?: string
  sort?: string
  category?: number | ''
  featured?: boolean
  inStock?: boolean
  limit?: number
  offset?: number
}

export interface ProductListResult {
  data: Product[]
  total: number
}

export async function listProducts(params: ProductListParams = {}): Promise<ProductListResult> {
  const { data } = await client.get<ProductListResult>('/store/products', {
    params: {
      q: params.q?.trim() || undefined,
      sort: params.sort && params.sort !== 'default' ? params.sort : undefined,
      category: params.category || undefined,
      featured: params.featured ? 'true' : undefined,
      in_stock: params.inStock ? 'true' : undefined,
      limit: params.limit,
      offset: params.offset || undefined,
    },
  })
  return { data: data.data ?? [], total: data.total ?? 0 }
}

export async function getProduct(slug: string): Promise<Product> {
  const { data } = await client.get<{ data: Product }>(`/store/products/${slug}`)
  return normalizeProduct(data.data)
}

export async function listCategories(): Promise<Category[]> {
  const { data } = await client.get<{ data: Category[] }>('/store/categories')
  return data.data ?? []
}

export interface StoreStatsResult {
  stats: StoreStats
  coupons_enabled: boolean
  /** 配置中心的前台展示开关：为 false 时商品卡片不显示销量。 */
  show_sold_count?: boolean
}

export async function storeStats(): Promise<StoreStatsResult> {
  const { data } = await client.get<StoreStatsResult>('/store/stats')
  return data
}

/**
 * The shop's own promo shelf: codes the owner chose to advertise and that a
 * buyer could still use right now. Public and unpaginated on purpose — a code
 * kept secret stays secret, and a running promotion is not a long list.
 */
export async function listStoreCoupons(): Promise<StorefrontCoupon[]> {
  const { data } = await client.get<{ data?: StorefrontCoupon[] }>('/store/coupons')
  return data.data ?? []
}

/**
 * 优惠码 is quoted before the order exists, so the buyer sees the discount and
 * the shop's own reason for refusing the code instead of a failed checkout.
 */
export async function quoteCoupon(payload: {
  code: string
  slug: string
  quantity: number
}): Promise<CouponQuote> {
  const { data } = await client.post<CouponQuote>('/store/coupons/quote', {
    code: payload.code,
    product_slug: payload.slug,
    quantity: payload.quantity,
  })
  return data
}

export function couponQuoteMessage(error: unknown): string {
  return errorMessage(error, '优惠码无法使用。')
}
