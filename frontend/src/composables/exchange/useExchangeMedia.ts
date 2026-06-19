import { ref } from 'vue'
import type { Product } from '@/api/exchange'

export function useExchangeMedia() {
  const isMobile = ref(false)
  const localImageMap = ref<Record<string, string>>({})

  const checkMobile = () => {
    isMobile.value = window.innerWidth <= 768
  }

  const loadLocalImageMap = async () => {
    try {
      const response = await fetch('/images/products/image_mapping.json')
      if (response.ok) {
        localImageMap.value = await response.json()
        console.log('本地图片映射加载成功:', Object.keys(localImageMap.value).length, '个商品')
      }
    } catch (error) {
      console.log('本地图片映射加载失败，将使用远程图片')
    }
  }

  const getProductImageUrl = (product: Product | null | undefined): string => {
    if (!product) return ''
    const prizeId = String(product.prize_id)
    if (localImageMap.value[prizeId]) {
      return localImageMap.value[prizeId]
    }
    return product.image_url || ''
  }

  return {
    isMobile,
    localImageMap,
    checkMobile,
    loadLocalImageMap,
    getProductImageUrl
  }
}
