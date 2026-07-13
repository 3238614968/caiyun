import { buildProductImageSource } from './product-image'

describe('product image utils', () => {
  it('returns empty source for empty input', () => {
    expect(buildProductImageSource()).toEqual({ src: '' })
  })

  it('derives avif companion for optimized product images', () => {
    expect(buildProductImageSource('/images/products-modern/demo.webp')).toEqual({
      src: '/images/products-modern/demo.webp',
      avifSrc: '/images/products-modern/demo.avif'
    })
  })

  it('keeps non-optimized images unchanged', () => {
    expect(buildProductImageSource('/images/products/demo.png')).toEqual({
      src: '/images/products/demo.png'
    })
  })
})
