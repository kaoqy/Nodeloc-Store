export interface User {
  id: number
  username: string
  name?: string
  email?: string | null
  avatar?: string | null
  role: string
  points: number
  is_admin: boolean
  is_active: boolean
  consecutive_days?: number
  total_checkins?: number
  last_checkin_date?: string | null
  last_login_at?: string | null
  oauth_provider?: string | null
  oauth_uid?: string | null
  oauth_username?: string | null
  oauth_name?: string | null
  oauth_avatar?: string | null
  oauth_bound?: boolean
  created_at?: string
  updated_at?: string
}

export interface AuthTokens {
  access_token: string
}

export interface LoginResponse {
  user: User
  tokens: AuthTokens
}

export interface Product {
  id: number
  name: string
  slug: string
  description?: string | null
  summary?: string | null
  price: number
  original_price?: number | null
  stock_count: number
  stock_visible?: boolean
  auto_deliver?: boolean
  is_published: boolean
  is_archived?: boolean
  product_type: string
  delivery_instructions?: string | null
  image_path?: string | null
  require_contact?: boolean
  category_id?: number | null
  category?: Category | null
  sort_order?: number
  created_at?: string
  updated_at?: string
}

export interface Card {
  id: number
  product_id: number
  content?: string
  status?: string
  order_id?: number | null
  sold_at?: string | null
  created_at?: string
  updated_at?: string
}

export interface Order {
  id: number
  order_no: string
  user_id?: number
  user?: User
  product?: Product
  product_id?: number
  product_name?: string
  quantity: number
  unit_price?: number
  total_amount: number
  status: string
  fulfillment_status?: string
  transaction_id?: string | null
  delivery_content?: string | null
  delivery_note?: string | null
  customer_contact?: string | null
  customer_note?: string | null
  paid_at?: string | null
  delivered_at?: string | null
  created_at?: string
  updated_at?: string
}

export interface Category {
  id: number
  name: string
  slug?: string
  description?: string | null
  icon?: string | null
  sort_order?: number
  is_visible?: boolean
  created_at?: string
  updated_at?: string
}

export interface Coupon {
  id: number
  code: string
  discount_type: string
  discount_value: number
  min_order_amount: number
  max_uses: number
  used_count: number
  is_active: boolean
  valid_from?: string | null
  valid_until?: string | null
  created_at?: string
  updated_at?: string
}

export interface AuditLog {
  id: number
  user_id?: number
  user?: User
  action: string
  resource?: string
  resource_id?: string | number
  details?: string | Record<string, unknown>
  ip?: string
  created_at: string
}

export interface PaginationParams {
  page?: number
  per_page?: number
  search?: string
  status?: string
}

export type SettingsMap = Record<string, string | number | boolean | null>
