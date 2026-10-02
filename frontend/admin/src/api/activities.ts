import client from './client'

export interface ActivityRule {
  id?: number
  activity_id?: number
  rule_type: string
  config: string
  is_enabled: boolean
  sort_order: number
}

export interface Activity {
  id: number
  name: string
  subtitle?: string
  description?: string
  cover_image?: string
  banner_image?: string
  type: string
  rules?: string
  product_ids?: string
  category_ids?: string
  user_scope: string
  user_role_scope?: string
  start_at?: string | null
  end_at?: string | null
  sort_order: number
  stock_limit: number
  stock_used: number
  quota_limit: number
  quota_used: number
  per_user_limit: number
  require_login: boolean
  allow_stacking: boolean
  auto_apply: boolean
  status: string
  remark?: string
  terminate_reason?: string
  show_on_home: boolean
  show_in_list: boolean
  notify_users: boolean
  notify_text?: string
  rule_list?: ActivityRule[]
  order_count?: number
  user_count?: number
  discount_total?: number
  revenue_total?: number
  conversion_rate?: number
  created_at?: string
  updated_at?: string
}

export interface ActivityStats {
  activity_id: number
  participants: number
  orders: number
  paid_orders: number
  discount_total: number
  revenue_total: number
  stock_used: number
  stock_limit: number
  quota_used: number
  quota_limit: number
  conversion_rate: number
  refund_count: number
}

export interface ActivityLogRow {
  id: number
  activity_id: number
  actor_id?: number
  action: string
  detail?: string
  before?: string
  after?: string
  ip?: string
  result: string
  created_at?: string
}

export interface ActivityRecordRow {
  id: number
  activity_id: number
  user_id: number
  order_id?: number
  product_id?: number
  quantity: number
  original_amount: number
  discount_amount: number
  payable_amount: number
  status: string
  coupon_code?: string
  created_at?: string
}

export interface ActivityOverview {
  total: number
  running: number
  participants: number
  discounts: number
}

export interface ActivityListQuery {
  status?: string
  type?: string
  q?: string
  sort?: string
  limit?: number
  offset?: number
}

export const activityTypeOptions = [
  { value: 'limited_discount', label: '限时折扣' },
  { value: 'store_discount', label: '全场折扣' },
  { value: 'product_discount', label: '指定商品折扣' },
  { value: 'category_discount', label: '指定分类折扣' },
  { value: 'full_reduction', label: '满减' },
  { value: 'full_quantity', label: '满件优惠' },
  { value: 'bulk_discount', label: '批量购买优惠' },
  { value: 'coupon_activity', label: '优惠码活动' },
  { value: 'new_user', label: '新人优惠' },
  { value: 'first_purchase', label: '首次购买优惠' },
  { value: 'member_only', label: '会员专属活动' },
  { value: 'limited_activity', label: '限量活动' },
  { value: 'seckill', label: '商品秒杀' },
  { value: 'coupon_claim', label: '优惠券领取活动' },
]

export const activityStatusOptions = [
  { value: 'draft', label: '草稿' },
  { value: 'scheduled', label: '未开始' },
  { value: 'running', label: '进行中' },
  { value: 'paused', label: '已暂停' },
  { value: 'ended', label: '已结束' },
  { value: 'offline', label: '已下架' },
]

export const listActivities = (query: ActivityListQuery = {}) =>
  client
    .get<{ data: Activity[]; total: number }>('/admin/activities', {
      params: {
        status: query.status && query.status !== 'all' ? query.status : undefined,
        type: query.type && query.type !== 'all' ? query.type : undefined,
        q: query.q?.trim() || undefined,
        sort: query.sort || undefined,
        limit: query.limit,
        offset: query.offset || undefined,
      },
    })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const getActivityOverview = () =>
  client.get<ActivityOverview>('/admin/activity-overview').then((r) => r.data)

export const getActivity = (id: number) =>
  client.get<{ data: Activity }>('/admin/activities/' + id).then((r) => r.data.data)

export const createActivity = (activity: Partial<Activity>, rules: ActivityRule[]) =>
  client
    .post<{ data: Activity }>('/admin/activities', { activity, rules })
    .then((r) => r.data.data)

export const updateActivity = (id: number, activity: Partial<Activity>, rules: ActivityRule[]) =>
  client
    .put<{ data: Activity }>('/admin/activities/' + id, { activity, rules })
    .then((r) => r.data.data)

export const deleteActivity = (id: number) => client.delete('/admin/activities/' + id)

export const duplicateActivity = (id: number) =>
  client.post<{ data: Activity }>('/admin/activities/' + id + '/duplicate').then((r) => r.data.data)

export const setActivityStatus = (id: number, status: string, reason = '') =>
  client
    .post<{ data: Activity }>('/admin/activities/' + id + '/status', { status, reason })
    .then((r) => r.data.data)

export const getActivityStats = (id: number) =>
  client.get<{ data: ActivityStats }>('/admin/activities/' + id + '/stats').then((r) => r.data.data)

export const listActivityRecords = (id: number, limit = 20, offset = 0) =>
  client
    .get<{ data: ActivityRecordRow[]; total: number }>('/admin/activities/' + id + '/records', {
      params: { limit, offset: offset || undefined },
    })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))

export const listActivityLogs = (id: number, limit = 20, offset = 0) =>
  client
    .get<{ data: ActivityLogRow[]; total: number }>('/admin/activities/' + id + '/logs', {
      params: { limit, offset: offset || undefined },
    })
    .then((r) => ({ data: r.data.data ?? [], total: r.data.total ?? 0 }))
