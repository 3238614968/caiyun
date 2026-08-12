import type { OperationResponse } from './generated/operation-contract'

export type {
  OperationAcceptedResponse,
  OperationResponse,
  OperationStatus,
  OperationUpdate,
  OperationUpdatedEvent
} from './generated/operation-contract'

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

// ApiContractError distinguishes a successful HTTP response whose payload does
// not satisfy the documented API envelope from a business/API failure.  This is
// intentionally surfaced to callers rather than fabricating a successful
// operation with an empty ID.
export class ApiContractError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'ApiContractError'
  }
}

export function requireApiData<T>(value: T | ApiResponse<T>, subject = 'API response'): T {
  const data = isApiResponse<T>(value) ? value.data : value
  if (data === undefined || data === null) {
    throw new ApiContractError(`${subject} is missing data`)
  }
  return data
}

export function unwrapOperationResponse(value: OperationResponse | ApiResponse<OperationResponse>): OperationResponse {
	const operation = requireApiData(value, 'operation response')
	if (!operation.operation_id || !operation.type || !operation.status) {
		throw new ApiContractError('operation response is incomplete')
	}
	return operation
}
