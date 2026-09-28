import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { siteStatus } from '../api/system'
import { applyBrand } from '../utils/brand'
import { applyShopIdentity } from '../utils/identity'
import type { FooterLink } from '../types'

export const useSiteStore = defineStore('site', () => {
  const name = ref('Nodeloc Store')
  const slogan = ref('')
  const logo = ref('')
  const initialized = ref(true)
  const version = ref('')

  // Everything below is the shop owner's wording, typed in 后台设置. The
  // storefront only renders it.
  const footerText = ref('')
  const footerNote = ref('')
  const footerLinks = ref<FooterLink[]>([])
  const announcement = ref('')
  const registrationEnabled = ref(true)
  const checkinEnabled = ref(true)
  const couponsEnabled = ref(true)

  async function load() {
    try {
      const status = await siteStatus()
      initialized.value = status.initialized !== false
      version.value = status.version
      if (status.app) {
        name.value = status.app.name || name.value
        slogan.value = status.app.slogan || ''
        logo.value = status.app.logo || ''
        footerText.value = status.app.footer_text || ''
        footerNote.value = status.app.footer_note || ''
        footerLinks.value = status.app.footer_links ?? []
        announcement.value = status.app.announcement || ''
      }
      if (status.features) {
        registrationEnabled.value = status.features.registration !== false
        checkinEnabled.value = status.features.checkin !== false
        couponsEnabled.value = status.features.coupons !== false
      }
      // The owner's chosen colour is style, not copy: it repaints the tokens in
      // style.css and is remembered for the next boot splash. The locale rides
      // on <html lang>, which is what screen readers and CJK font fallback read.
      applyBrand(status.theme?.primary)
      if (status.theme?.locale) document.documentElement.lang = status.theme.locale
      // The name and the summary are what the browser tab and a search result
      // show, so they travel with the site identity rather than the page body.
      applyShopIdentity(status.app?.name, status.app?.description)
    } catch {
      // The status endpoint is optional; the storefront still works without it.
    }
  }

  const initials = computed(() => name.value.trim().slice(0, 1).toUpperCase() || 'N')
  const hasFooter = computed(
    () => Boolean(footerText.value || footerNote.value || footerLinks.value.length),
  )

  return {
    name,
    slogan,
    logo,
    initialized,
    version,
    footerText,
    footerNote,
    footerLinks,
    announcement,
    registrationEnabled,
    checkinEnabled,
    couponsEnabled,
    initials,
    hasFooter,
    load,
  }
})
