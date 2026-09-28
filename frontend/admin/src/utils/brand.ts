// One hex from 后台设置 · 外观 paints the whole shop: style.css derives every
// accent token from --brand, so this only has to set the source colour and pick
// the ink that stays readable on top of it.
const STORAGE_KEY = 'store-brand'
const HEX = /^#[0-9a-f]{6}$/i

// Both SPAs share this origin and this key on purpose: a shop that recolours
// repaints its back office, its login page and the boot splash at once.
export function applyBrand(primary?: string | null, persist = true) {
  const value = typeof primary === 'string' ? primary.trim().toLowerCase() : ''
  if (!HEX.test(value)) return
  const root = document.documentElement
  root.style.setProperty('--brand', value)
  root.style.setProperty('--on-accent', inkOn(value))
  if (persist) localStorage.setItem(STORAGE_KEY, value)
}

// Bright brands (yellows, mint) need the dark ink; everything else keeps the
// near-white one the palette ships with.
function inkOn(hex: string): string {
  const rgb = parseInt(hex.slice(1), 16)
  const brightness = (((rgb >> 16) & 255) * 299 + ((rgb >> 8) & 255) * 587 + (rgb & 255) * 114) / 1000
  return brightness > 150 ? '#16161a' : '#fffaf7'
}
