// 后台设置里的「网站名称」与「网站描述」是店铺的门面，但浏览器标签页和搜索
// 结果读的是 document.title 与 description meta，而不是页面上的文字，所以它们
// 得跟着店铺走，而不是永远写死 Nodeloc Store。
const NAME_KEY = 'store-name'
const DEFAULT_NAME = 'Nodeloc Store'

// The last name the status API returned, or the one it returned on the previous
// visit. A route guard needs it before any store instance exists.
let shopName = localStorage.getItem(NAME_KEY) || DEFAULT_NAME

// The page's own words, kept so a late-arriving shop name repaints the current
// route instead of flattening it back to the store front door.
let section = ''

// The tab's icon is the shop's face: the mark the build ships with until the
// owner uploads a logo, theirs from then on.
const builtInIcon = document.head.querySelector<HTMLLinkElement>('link[rel="icon"]')?.href || ''

// Both SPAs share this origin and this key, same as the accent colour: the tab
// a buyer opens and the tab the owner works in should read alike.
export function applyShopIdentity(name?: string, description?: string, logo?: string) {
  const value = (name ?? '').trim()
  if (value) {
    shopName = value
    localStorage.setItem(NAME_KEY, value)
  }
  // An owner who left the field blank keeps the description the build ships
  // with; an empty summary is worse for both readers and crawlers.
  const summary = (description ?? '').trim()
  if (summary) {
    const meta = document.head.querySelector('meta[name="description"]')
    if (meta) meta.setAttribute('content', summary)
  }
  const icon = document.head.querySelector<HTMLLinkElement>('link[rel="icon"]')
  if (icon) icon.href = (logo ?? '').trim() || builtInIcon
  setPageTitle()
}

// Called with a page's own words (商品名、订单号…), or with nothing to go back to
// the bare shop name. Navigation clears it so a product title cannot haunt the
// next route.
export function setPageTitle(part?: string) {
  if (part !== undefined) section = part.trim()
  document.title = section ? `${section} · ${shopName}` : shopName
}
