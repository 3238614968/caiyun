export interface ProductImageSource {
  src: string
  avifSrc?: string
}

const MODERN_IMAGE_PREFIX = '/images/products-modern/'

export function buildProductImageSource(src?: string | null): ProductImageSource {
  const normalizedSrc = String(src || '').trim()
  if (!normalizedSrc) {
    return { src: '' }
  }

  if (normalizedSrc.startsWith(MODERN_IMAGE_PREFIX) && normalizedSrc.endsWith('.webp')) {
    return {
      src: normalizedSrc,
      avifSrc: normalizedSrc.replace(/\.webp$/i, '.avif')
    }
  }

  return { src: normalizedSrc }
}
