export interface User {
  id: number | string
  username: string
  email?: string | null
  avatar?: string | null
  oauth_bound?: boolean
  created_at?: string
}

export interface Category {
  id: number | string
  name: string
  slug: string
  description?: string | null
}

export interface Product {
  id: number | string
  name: string
  slug: string
  description: string
  price: number
  original_price?: number | null
  cover_image?: string | null
  category?: Category | null
  category_id?: number | string | null
  stock?: number | null
  status?: string
  created_at?: string
  updated_at?: string
}

export interface Card {
  id: number | string
  code?: string
  content?: string
  status?: string
  delivered_at?: string | null
}

export interface Order {
  id: number | string
  order_no: string
  product?: Product | null
  product_name?: string
  amount: number
  quantity?: number
  status: string
  payment_status?: string
  delivery_status?: string
  description?: string | null
  cards?: Card[]
  created_at: string
  paid_at?: string | null
  delivered_at?: string | null
}

export interface Notification {
  id: number | string
  title: string
  content: string
  type?: string
  read: boolean
  created_at: string
}

export interface AuthTokens {
  access_token: string
  token_type?: string
  expires_in?: number
}

export interface AuthResponse {
  user: User
  tokens: AuthTokens
}

export interface ProductListResponse {
  data: Product[]
  current_page?: number
  last_page?: number
  total?: number
  per_page?: number
}
