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

// Transfer is one 店家主动转账: NL the shop sent to a buyer's NodeLoc account.
// Both outcomes are kept, so a refusal is still something to point at later.
export interface Transfer {
  id: number
  user_id: number
  username: string
  to_user_id: string
  to_username?: string
  reference: string
  provider?: string
  provider_transaction_id?: string | null
  amount: number
  status: string
  note?: string
  detail?: string
  operator_id: number
  operator_name?: string
  created_at?: string
  completed_at?: string | null
}

export interface LoginResponse {
  user: User
  tokens: AuthTokens
}

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
  form_schema?: string | null
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
  form_values?: string
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
  advertised?: boolean
  scope?: string
  category_id?: number | null
  product_id?: number | null
  per_user_limit?: number
  /** Orders still holding this code's quota, paid ones and live checkouts. */
  held?: number
  remaining?: number | null
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
  actor_name?: string
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
  // Buyers who already paid for this product and have no key to receive yet.
  waiting: number
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

  // 工单与客服指标
  tickets_total: number
  tickets_pending_human: number
  tickets_unread: number
  tickets_urgent: number
  tickets_overdue: number
  ticket_resolve_rate: number
  ticket_satisfaction: number
  ticket_avg_minutes: number
  activities_running: number
  activity_participants: number
  auto_delivery_failed: number
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
  smtp: {
    enabled: boolean
    host: string
    port: number
    username: string
    password: string
    secure: string
    from: string
  }
  payment: {
    enabled: boolean
    payment_id: string
    // 支付 API 地址：留空则跟随 OAuth 域名。
    base_url?: string
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

// ── Plugins ──────────────────────────────────────────────────────────

/** One input on a plugin's configuration form; password fields are write-only. */
export interface PluginConfigField {
  key: string
  label: string
  type: 'text' | 'textarea' | 'password' | 'select' | 'number' | 'bool'
  required: boolean
  placeholder?: string
  help?: string
  options?: string[]
}

export interface PluginManifest {
  key: string
  name: string
  description: string
  version: string
  author: string
  capabilities: string[]
  config_schema: PluginConfigField[]
}

/** A provider the shop may enroll, with its current enrollment state. */
export interface PluginCatalogEntry {
  manifest: PluginManifest
  installed: boolean
  enabled: boolean
  plugin_id?: number
  settings?: Record<string, string>
  /** Keys whose value is already stored, so the form can say 「已保存」. */
  secret_fields?: string[]
  binding_count: number
  installed_at?: string
}

export interface Plugin {
  id: number
  key: string
  name: string
  description?: string
  version: string
  author?: string
  is_enabled: boolean
  settings?: string
  config_schema?: string
  capabilities?: string
  installed_at?: string
  /** Transient: the provider's complaint about the configuration just saved. */
  validation_warning?: string
}

/** Maps one purchase-form answer to a provider-side delivery item. */
export interface PluginBinding {
  id: number
  plugin_id: number
  product_id: number
  value: string
  match_field?: string
  remote_name?: string
  remote_ref: string
  extra?: string
  is_enabled: boolean
  sort_order: number
  plugin_name?: string
  plugin_key?: string
  product_name?: string
  product_slug?: string
}
