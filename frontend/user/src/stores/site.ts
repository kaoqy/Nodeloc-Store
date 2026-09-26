import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import { siteStatus } from '../api/system'

export const useSiteStore = defineStore('site', () => {
  const name = ref('Nodeloc Store')
  const slogan = ref('')
  const logo = ref('')
  const initialized = ref(true)
  const version = ref('')

  async function load() {
    try {
      const status = await siteStatus()
      initialized.value = status.initialized !== false
      version.value = status.version
      if (status.app) {
        name.value = status.app.name || name.value
        slogan.value = status.app.slogan || ''
        logo.value = status.app.logo || ''
      }
    } catch {
      // The status endpoint is optional; the storefront still works without it.
    }
  }

  const initials = computed(() => name.value.trim().slice(0, 1).toUpperCase() || 'N')

  return { name, slogan, logo, initialized, version, initials, load }
})
