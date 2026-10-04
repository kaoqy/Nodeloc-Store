import { ref } from 'vue'

/**
 * One hairline bar covers route changes and API calls, since either can be the
 * slow step. It reveals after a beat so instant work never flashes.
 *
 * Navigation is a flag rather than a counter: a guard that redirects aborts the
 * first navigation without settling it, and a counted abort leaks a stuck bar.
 */
const REVEAL_AFTER_MS = 180

const navigating = ref(false)
const requests = ref(0)

export const progressing = ref(false)
export const requestPending = ref(false)
let revealTimer: number | undefined
let pendingTimer: number | undefined

function sync() {
  if (navigating.value || requests.value > 0) {
    if (revealTimer === undefined) {
      revealTimer = window.setTimeout(() => {
        progressing.value = true
      }, REVEAL_AFTER_MS)
    }
    if (pendingTimer === undefined) {
      pendingTimer = window.setTimeout(() => {
        requestPending.value = true
      }, 420)
    }
    return
  }
  window.clearTimeout(revealTimer)
  revealTimer = undefined
  window.clearTimeout(pendingTimer)
  pendingTimer = undefined
  progressing.value = false
  requestPending.value = false
}

export function beginNavigation() {
  navigating.value = true
  sync()
}

export function endNavigation() {
  navigating.value = false
  sync()
}

export function beginRequest() {
  requests.value += 1
  sync()
}

export function endRequest() {
  requests.value = Math.max(0, requests.value - 1)
  sync()
}
