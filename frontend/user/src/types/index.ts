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
  total_checkins?: number
  last_checkin_date?: string | null
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
  icon?: string | null
  sort_order?: number
  is_visible?: boolean
  // The storefront's category list is counted server-side, so a chip can show
  // how many products a buyer would actually reach.
  product_count?: number
}

export type ProductType = 'card' | 'manual'

export interface ProductFormField {
  key: string
  label: string
  type: 'text' | 'select'
  required: boolean
  placeholder?: string
  options?: string[]
  max_length?: number
}

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
  sold_count?: number
  is_featured?: boolean
  auto_deliver: boolean
  is_published: boolean
  sort_order?: number
  category_id?: number | null
  category?: Category | null
  delivery_instructions?: string | null
  form_schema?: ProductFormField[] | null
  created_at?: string
  updated_at?: string
}

export interface StoreStats {
  products: number
  stock: number
  sales: number
  categories: number
}

export interface Order {
  id: number
  order_no: string
  user_id: number
  product_id: number
  quantity: number
  unit_price: number
  discount_amount?: number
  coupon_code?: string
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
  form_values?: string
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

export interface CouponQuote {
  code: string
  accepted: boolean
  discount: number
  payable: number
  original_total: number
  description?: string
  // note is the rule the preview could not decide on its own — a guest's
  // 每人限用, which the checkout that follows still enforces.
  note?: string
}

/**
 * A promotion the shop put on its own shelf: the buyer can read the code and
 * what it is worth before typing anything. remaining is the live quota left.
 */
export interface StorefrontCoupon {
  code: string
  discount_type: string
  discount_value: number
  min_order_amount: number
  scope: string
  product_id?: number | null
  category_id?: number | null
  per_user_limit: number
  description?: string
  valid_until?: string | null
  remaining?: number | null
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

export interface CheckinStatus {
  enabled: boolean
  checked_in_today: boolean
  consecutive_days: number
  total_checkins: number
  points: number
}

export interface CheckinResult extends CheckinStatus {
  reward: number
  user: User
}

export interface CheckinRecord {
  id: number
  checkin_date: string
  reward_points: number
  consecutive_days: number
}

export interface PointEntry {
  id: number
  delta: number
  balance_after: number
  reason: string
  created_at: string
}

export interface MyPermissions {
  role: string
  is_staff: boolean
  permissions: string[]
}

export interface AppNotification {
  id: number
  type: string
  title: string
  content?: string | null
  link?: string | null
  is_read: boolean
  created_at: string
}

export interface FooterLink {
  label: string
  url: string
}

export interface SiteStatus {
  initialized: boolean
  version: string
  app?: {
    name: string
    slogan?: string
    description?: string
    logo?: string
    footer_text?: string
    footer_note?: string
    footer_links?: FooterLink[]
    announcement?: string
  }
  features?: {
    registration?: boolean
    checkin?: boolean
    coupons?: boolean
    oauth?: boolean
    payments?: boolean
  }
  theme?: {
    primary?: string
    locale?: string
  }
}
