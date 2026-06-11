export interface ApiResponse<T> {
  code: number
  message: string
  data?: T
}

export function isApiResponse<T>(value: unknown): value is ApiResponse<T> {
  return !!value &&
    typeof value === 'object' &&
    'code' in value &&
    'message' in value
}

export function unwrapApiData<T>(value: T | ApiResponse<T>, fallback: T): T {
  if (isApiResponse<T>(value)) {
    return value.data ?? fallback
  }
  return value ?? fallback
}
