import type { RouteLocationNormalized } from 'vue-router'

/**
 * 后台导航的唯一数据源。
 *
 * 之前侧栏、面包屑、页面标题、全局搜索各写一份菜单数据，改一处忘一处就会
 * 出现「侧栏有这个页面但搜索找不到」「标题写错但侧栏没错」这类不一致。
 * 现在所有导航信息都在这里定义一次，其余地方只做筛选与展示。
 */

export interface NavItem {
  /** 路由路径；子页面链接到父页面时用 path 字段即可 */
  path: string
  label: string
  /** 侧栏与搜索结果里的一句话说明 */
  hint: string
  /** AdminIcon 的图标名 */
  icon: string
  /** 需要的 Casbin 权限，格式 resource:action */
  permission: string
  /** 备选权限：任一命中即可见（例如客服中心同时承载工单与 AI 配置） */
  altPermission?: string
  /** 搜索关键词，方便中英文都能搜到 */
  keywords?: string[]
}

export interface NavGroup {
  label: string
  items: NavItem[]
}

/**
 * 分组顺序按店主一天的动线排：先看今天的生意，再处理单子，然后维护内容，
 * 最后才是配置与权限。
 */
export const NAV_GROUPS: NavGroup[] = [
  {
    label: '概览',
    items: [
      {
        path: '/', label: '总览', hint: '今天的生意与待办', icon: 'dashboard',
        permission: 'stats:view', keywords: ['dashboard', 'home', '首页'],
      },
    ],
  },
  {
    label: '经营',
    items: [
      {
        path: '/orders', label: '订单管理', hint: '支付、发货与退款', icon: 'orders',
        permission: 'orders:view', keywords: ['order', '订单', '发货'],
      },
      {
        path: '/products', label: '商品管理', hint: '上架、定价与详情', icon: 'products',
        permission: 'products:view', keywords: ['product', '商品'],
      },
      {
        path: '/cards', label: '卡密库存', hint: '导入、生成与补货', icon: 'cards',
        permission: 'cards:view', keywords: ['card', '卡密', '库存', '补货'],
      },
      {
        path: '/categories', label: '分类管理', hint: '商品分类与排序', icon: 'categories',
        permission: 'categories:view', keywords: ['category', '分类'],
      },
      {
        path: '/coupons', label: '优惠券', hint: '优惠码与使用范围', icon: 'coupons',
        permission: 'coupons:view', keywords: ['coupon', '优惠码', '折扣'],
      },
      {
        path: '/activities', label: '活动营销', hint: '折扣、满减与领券', icon: 'activities',
        permission: 'activities:view', keywords: ['activity', '活动', '促销'],
      },
      {
        path: '/plugins', label: '插件管理', hint: '交付提供者与映射', icon: 'plugins',
        permission: 'plugins:view', keywords: ['plugin', '插件'],
      },
    ],
  },
  {
    label: '客服与 AI',
    items: [
      {
        path: '/service', label: '客服中心', hint: '工单队列与 AI 接待', icon: 'support',
        permission: 'tickets:view', altPermission: 'ai:view',
        keywords: ['ticket', '工单', '客服', 'ai'],
      },
      {
        path: '/knowledge', label: '知识库', hint: 'AI 回答的依据', icon: 'knowledge',
        permission: 'knowledge:view', keywords: ['knowledge', '知识库', '文档'],
      },
    ],
  },
  {
    label: '客户',
    items: [
      {
        path: '/users', label: '用户管理', hint: '账号、积分与转账', icon: 'users',
        permission: 'users:view', keywords: ['user', '用户', '会员'],
      },
      {
        path: '/notifications', label: '通知中心', hint: '站内信与广播', icon: 'notifications',
        permission: 'notifications:view', keywords: ['notification', '通知'],
      },
    ],
  },
  {
    label: '系统',
    items: [
      {
        path: '/config', label: '配置中心', hint: 'AI、通知、风控与保留策略', icon: 'settings',
        permission: 'config_center:view', altPermission: 'ai:view',
        keywords: ['config', '配置', '设置'],
      },
      {
        path: '/settings', label: '系统设置', hint: '站点、登录与支付凭据', icon: 'settings',
        permission: 'settings:view', keywords: ['settings', '设置', 'oauth', 'payment'],
      },
      {
        path: '/roles', label: '权限与管理员', hint: '角色与权限矩阵', icon: 'roles',
        permission: 'roles:view', keywords: ['role', '角色', '权限', '管理员'],
      },
      {
        path: '/logs', label: '操作日志', hint: '谁改了什么', icon: 'logs',
        permission: 'logs:view', keywords: ['log', '日志', '审计'],
      },
    ],
  },
]

/** 扁平化后的导航项，供搜索与标题查询使用。 */
export const NAV_ITEMS: NavItem[] = NAV_GROUPS.flatMap((group) => group.items)

/** 详情页、表单页这类不在侧栏出现的路径，也要有中文标题。 */
const EXTRA_TITLES: { match: RegExp; label: string }[] = [
  { match: /^\/products\/new$/, label: '新建商品' },
  { match: /^\/products\/\d+\/edit$/, label: '编辑商品' },
  { match: /^\/cards\/\d+$/, label: '卡密库存' },
  { match: /^\/orders\/[^/]+$/, label: '订单详情' },
  { match: /^\/users\/\d+$/, label: '用户详情' },
  { match: /^\/activities\/new$/, label: '新建活动' },
  { match: /^\/activities\/\d+$/, label: '活动数据' },
  { match: /^\/activities\/\d+\/edit$/, label: '编辑活动' },
  { match: /^\/service\/agents$/, label: '客服与快捷回复' },
  { match: /^\/forbidden$/, label: '权限不足' },
  { match: /^\/setup$/, label: '初始化' },
  { match: /^\/login$/, label: '登录' },
]

/** 面包屑里要把路由参数换成中文的段落。 */
const SEGMENT_LABELS: Record<string, string> = {
  new: '新建',
  edit: '编辑',
  cards: '卡密',
  agents: '客服与快捷回复',
}

/**
 * titleOf 返回一个路由应显示的中文标题。
 * 详情页（订单号、用户 ID）优先用调用方提供的动态标题。
 */
export function titleOf(path: string, dynamic?: string): string {
  if (dynamic) return dynamic
  for (const extra of EXTRA_TITLES) {
    if (extra.match.test(path)) return extra.label
  }
  const exact = NAV_ITEMS.find((item) => item.path === path)
  if (exact) return exact.label
  const first = '/' + (path.split('/').filter(Boolean)[0] ?? '')
  return NAV_ITEMS.find((item) => item.path === first)?.label ?? '后台'
}

export interface Crumb {
  label: string
  path: string
  last: boolean
}

/** breadcrumbsOf 把路径拆成面包屑；参数段用中文占位而不是把 ID 直接摆出来。 */
export function breadcrumbsOf(route: RouteLocationNormalized): Crumb[] {
  const segments = route.path.split('/').filter(Boolean)
  if (!segments.length) return [{ label: '总览', path: '/', last: true }]
  return segments.map((segment, index) => {
    const isNumeric = /^\d+$/.test(segment)
    const label = SEGMENT_LABELS[segment] ?? (isNumeric ? '详情' : titleOf('/' + segment))
    return {
      label,
      path: '/' + segments.slice(0, index + 1).join('/'),
      last: index === segments.length - 1,
    }
  })
}

/** searchNav 用关键词匹配导航项，中英文与拼音首字母都能命中。 */
export function searchNav(query: string): NavItem[] {
  const needle = query.trim().toLowerCase()
  if (!needle) return []
  const scored: { item: NavItem; score: number }[] = []
  for (const item of NAV_ITEMS) {
    let score = 0
    const label = item.label.toLowerCase()
    if (label === needle) score = 100
    else if (label.startsWith(needle)) score = 80
    else if (label.includes(needle)) score = 60
    else if (item.path.toLowerCase().includes(needle)) score = 40
    else if (item.hint.toLowerCase().includes(needle)) score = 30
    else if ((item.keywords ?? []).some((word) => word.toLowerCase().includes(needle))) score = 50
    if (score > 0) scored.push({ item, score })
  }
  return scored.sort((a, b) => b.score - a.score).map((entry) => entry.item)
}
