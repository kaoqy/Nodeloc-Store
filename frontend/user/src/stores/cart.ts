import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import type { Product } from '../types'

export interface CartItem {
  product: Product
  quantity: number
}

export const useCartStore = defineStore('cart', () => {
  const items = ref<CartItem[]>([])
  const count = computed(() => items.value.reduce((sum, item) => sum + item.quantity, 0))
  const total = computed(() => items.value.reduce((sum, item) => sum + item.product.price * item.quantity, 0))

  function add(product: Product, quantity = 1) {
    const existing = items.value.find((item) => item.product.id === product.id)
    if (existing) existing.quantity += quantity
    else items.value.push({ product, quantity })
  }

  function remove(productId: Product['id']) {
    items.value = items.value.filter((item) => item.product.id !== productId)
  }

  function clear() {
    items.value = []
  }

  return { items, count, total, add, remove, clear }
})
