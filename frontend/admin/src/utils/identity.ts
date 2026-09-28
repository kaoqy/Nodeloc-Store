// 后台的标签页与门牌也要说同一家店：店名来自同一个 status 接口，写进与商店前台
// 共用的 localStorage 键，所以打开「系统设置 · 某某小店 管理后台」不必等请求回来。
import { computed, readonly, ref } from 'vue'

const NAME_KEY = 'store-name'
const DEFAULT_NAME = 'Nodeloc Store'
const SUFFIX = '管理后台'

const nameState = ref(localStorage.getItem(NAME_KEY) || DEFAULT_NAME)
const logoState = ref('')
let section = ''

export const shopName = readonly(nameState)
export const shopLogo = readonly(logoState)
export const shopInitials = computed(() => nameState.value.trim().slice(0, 1).toUpperCase() || 'N')

export function applyShopIdentity(name?: string, logo?: string) {
  const value = (name ?? '').trim()
  if (value) {
    nameState.value = value
    localStorage.setItem(NAME_KEY, value)
  }
  logoState.value = (logo ?? '').trim()
  setPageTitle()
}

export function setPageTitle(part?: string) {
  if (part !== undefined) section = part.trim()
  const shop = `${nameState.value} ${SUFFIX}`
  document.title = section ? `${section} · ${shop}` : shop
}
