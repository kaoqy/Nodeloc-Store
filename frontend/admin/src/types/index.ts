export interface User {
  id: number
  username?: string
  name?: string
  email: string
  avatar?: string
  role: string
  points: number
  is_admin?: boolean
  is_active?: boolean
  created_at?: string
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
  description?: string
  price: number
  stock?: number
  status?: string
  category_id?: number | null
  category?: Category
  image?: string
  created_at?: string
  updated_at?: string
}

export interface Card {
  id: number
  product_id: number
  content: string
  status?: string
  order_no?: string | null
  created_at?: string
}

export interface Order {
  id?: number
  order_no: string
  user_id?: number
  user?: User
  product?: Product
  product_name?: string
  quantity?: number
  total_amount: number
  status: string
  delivery_content?: string
  created_at?: string
  updated_at?: string
}

export interface Category {
  id: number
  name: string
  description?: string
  sort_order?: number
  is_active?: boolean
  created_at?: string
}

export interface Coupon {
  id: number
  code: string
  name?: string
  type?: string
  value: number
  min_amount?: number
  usage_limit?: number
  used_count?: number
  is_active?: boolean
  starts_at?: string
  expires_at?: string
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
