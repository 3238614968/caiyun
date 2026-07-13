import request from '../axios'
import { unwrapApiData, type ApiResponse } from '../response'
import type { ExportExchangeRecordsParams, GetExchangeRecordsParams, GetExchangeRecordsResponse } from './types'
import { normalizeExchangeRecord } from './normalizers'

export function getExchangeRecords(params: GetExchangeRecordsParams): Promise<GetExchangeRecordsResponse> {
  const fallback: GetExchangeRecordsResponse = { records: [], total: 0, stats: { success: 0, failed: 0 } }
  return request<GetExchangeRecordsResponse | ApiResponse<GetExchangeRecordsResponse>>({
    url: '/api/exchange/records',
    method: 'get',
    params
  }).then((res) => {
    const payload = unwrapApiData(res, fallback)
    return { ...payload, records: (payload.records || []).map(normalizeExchangeRecord) }
  })
}

export function exportExchangeRecords(params: ExportExchangeRecordsParams): Promise<Blob> {
  return request<Blob>({
    url: '/api/exchange/records/export',
    method: 'get',
    params,
    responseType: 'blob'
  })
}
