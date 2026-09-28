export interface User {
  id: number
  username: string
  nickname?: string
  avatar_url?: string
  bio?: string
  email?: string | null
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
  oauth_trust_level?: number | null
  oauth_has_email?: boolean
  created_at?: string
  updated_at?: string
}

export interface AuthTokens {
  access_token: string
  refresh_token?: string
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
  // Delivered volume, maintained by the server. Read-only in the back office.
  sold_count?: number
  is_featured?: boolean
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
  // Present on /admin/cards, which spans products and names each row.
  product_name?: string
  product_slug?: string
  order_no?: string
}

export interface Order {
  id: number
  order_no: string
  user_id?: number
  user?: User
  product?: Product
  product_id?: number
  quantity: number
  unit_price?: number
  total_amount: number
  discount_amount?: number
  coupon_code?: string | null
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
  description?: string | null
  scope?: string
  category_id?: number | null
  product_id?: number | null
  per_user_limit?: number
  created_at?: string
  updated_at?: string
}

export interface Notification {
  id: number
  user_id: number
  type: string
  title: string
  content?: string | null
  link?: string | null
  is_read: boolean
  created_at: string
}

export interface AuditLog {
  id: number
  actor_id?: number | null
  action: string
  target?: string | null
  detail?: string | null
  ip?: string | null
  created_at: string
}

export interface PageParams {
  limit?: number
  offset?: number
  status?: string
  q?: string
  user_id?: number
  /** "undelivered": paid orders still owed a delivery, regardless of status. */
  attention?: string
}

export interface Page<T> {
  data: T[]
  total: number
}

export interface RevenuePoint {
  date: string
  revenue: number
  orders: number
  users: number
}

export interface ProductStat {
  product_id: number
  name: string
  slug: string
  orders: number
  revenue: number
}

export interface BuyerStat {
  user_id: number
  name: string
  orders: number
  revenue: number
}

export interface StockAlert {
  product_id: number
  name: string
  slug: string
  available: number
  sold: number
}

export interface FunnelCount {
  key: string
  label: string
  count: number
}

export interface RecentOrder {
  order_no: string
  product: string
  buyer: string
  amount: number
  status: string
  fulfillment_status: string
  created_at: string
  paid_at?: string
}

export interface CategoryStat {
  name: string
  products: number
  orders: number
  revenue: number
}

export interface CouponStat {
  coupon_id: number
  code: string
  uses: number
  discount: number
  revenue: number
}

export interface CardProductHealth {
  product_id: number
  name: string
  slug: string
  available: number
  sold: number
  disabled: number
  sell_through: number
}

export interface CardHealth {
  available: number
  sold: number
  disabled: number
  total: number
  by_product: CardProductHealth[] | null
}

export interface Engagement {
  checkins_period: number
  checkin_users_period: number
  points_issued_period: number
  points_held: number
  bound_users: number
  active_week: number
}

export interface DashboardStats {
  revenue_total: number
  revenue_period: number
  orders_total: number
  orders_pending: number
  orders_waiting: number
  products_total: number
  users_total: number
  cards_available: number
  period_days: number
  revenue_series: RevenuePoint[]

  revenue_prev: number
  revenue_delta: number
  paid_prev: number
  paid_delta: number
  orders_prev: number
  new_users_prev: number
  new_users_delta: number
  orders_period: number
  paid_period: number
  conversion: number
  aov: number
  refunded_period: number
  orders_manual_pending: number
  delivered_period: number
  new_users_period: number
  active_buyers_period: number
  repeat_buyers_period: number

  // Go omits an empty slice as null, so every collection below is optional.
  top_products: ProductStat[] | null
  top_buyers: BuyerStat[] | null
  stock_alerts: StockAlert[] | null
  funnel: FunnelCount[] | null
  recent_orders: RecentOrder[] | null

  stock_alert_threshold: number
  category_sales: CategoryStat[] | null
  top_coupons: CouponStat[] | null
  coupon_uses_period: number
  coupon_discount_period: number
  coupons_total: number
  coupons_active: number
  card_health: CardHealth
  engagement: Engagement
}

export type SettingsMap = Record<string, string | number | boolean | null>

export interface FooterLink {
  label: string
  url: string
}

export interface RuntimeSettings {
  app: {
    site_name: string
    site_slogan: string
    site_description: string
    site_logo: string
    scheme: string
    domain: string
    footer_text?: string
    footer_note?: string
    footer_links?: FooterLink[] | null
    announcement?: string
  }
  oauth: {
    enabled: boolean
    base_url: string
    client_id: string
    client_secret: string
    redirect_uri: string
    scopes: string
  }
  payment: {
    enabled: boolean
    payment_id: string
    token: string
    secret_key: string
  }
  features: {
    enabled_registration: boolean
    enabled_checkin?: boolean
    enabled_coupons?: boolean
    stock_alert_threshold?: number
  }
  theme: { theme_primary: string; default_locale: string }
}
