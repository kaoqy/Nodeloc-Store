import { onScopeDispose, type Ref } from 'vue'

/**
 * Escape closes a dialog because that is the key a keyboard user reaches for
 * when a form is wrong — the scrim already does it for a mouse. Watching the
 * ref keeps one listener per dialog instead of re-binding on every open.
 */
export function closeOnEscape<T, Off extends T>(dialog: Ref<T>, off: Off) {
  const onKey = (event: KeyboardEvent) => {
    if (event.key !== 'Escape' || !dialog.value) return
    // A native dropdown swallows Escape to close itself; the dialog behind it
    // must stay open or the key cancels the field the operator was choosing.
    if (event.target instanceof HTMLSelectElement) return
    dialog.value = off
  }
  window.addEventListener('keydown', onKey)
  onScopeDispose(() => window.removeEventListener('keydown', onKey))
}
