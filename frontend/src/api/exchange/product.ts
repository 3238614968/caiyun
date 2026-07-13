import request from '../axios'
import { unwrapApiData, type ApiResponse } from '../response'
import type { GetProductCategoriesResponse, SearchProductsResponse, UpdateProductsResponse } from './types'

export function searchProducts(keyword: string, limit?: number): Promise<SearchProductsResponse> {
  const fallback: SearchProductsResponse = { products: [], total: 0 }
  return request<SearchProductsResponse | ApiResponse<SearchProductsResponse>>({
    url: '/api/products/search',
    method: 'get',
    params: { keyword, limit }
  }).then((res) => unwrapApiData(res, fallback))
}

export function getProductCategories(): Promise<GetProductCategoriesResponse> {
  const fallback: GetProductCategoriesResponse = { categories: [] }
  return request<GetProductCategoriesResponse | ApiResponse<GetProductCategoriesResponse>>({
    url: '/api/products/categories',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

export function updateProducts(accountId?: number): Promise<UpdateProductsResponse> {
  return request<UpdateProductsResponse>({
    url: '/api/products/update',
    method: 'post',
    data: { account_id: accountId }
  })
}
