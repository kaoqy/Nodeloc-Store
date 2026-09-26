export interface User {
  id: number
  username: string
  email?: string | null
  nickname?: string
  avatar_url?: string
  bio?: string
  role?: string
  is_admin?: boolean
  is_active?: boolean
  points: number
  consecutive_days?: number
  created_at?: string
  last_login_at?: string | null
  oauth_provider?: string | null
  oauth_uid?: string | null
  oauth_username?: string | null
  oauth_name?: string | null
  oauth_avatar?: string | null
  oauth_trust_level?: number | null
  oauth_has_email?: boolean
}

export interface Category {
  id: number
  name: string
  slug: string
  description?: string | null
}

export type ProductType = 'card' | 'manual'

export interface Product {
  id: number
  slug: string
  name: string
  summary?: string | null
  description?: string | null
  image_path?: string | null
  product_type: ProductType
  require_contact: boolean
  price: number
  original_price?: number | null
  stock_visible: boolean
  stock_count: number
  auto_deliver: boolean
  is_published: boolean
  sort_order?: number
  category_id?: number | null
  category?: Category | null
  created_at?: string
}

export interface Order {
  id: number
  order_no: string
  user_id: number
  product_id: number
  quantity: number
  unit_price: number
  total_amount: number
  status: string
  fulfillment_status: string
  transaction_id?: string | null
  platform_fee?: number | null
  merchant_points?: number | null
  paid_at?: string | null
  delivered_at?: string | null
  customer_contact?: string | null
  customer_note?: string | null
  delivery_content?: string | null
  delivery_note?: string | null
  product?: Product | null
  user?: { id: number; username: string } | null
  created_at: string
}

export interface PaymentOrder {
  id: number
  order_no: string
  amount: number
  status: string
  payment_url?: string | null
  provider_transaction_id?: string | null
}

export interface AuthTokens {
  access_token: string
  refresh_token?: string
  token_type?: string
  expires_at?: string
}

export interface AuthResponse {
  user: User
  tokens: AuthTokens
}

export interface SiteStatus {
  initialized: boolean
  version: string
  app?: {
    name: string
    slogan?: string
    description?: string
    logo?: string
  }
}
