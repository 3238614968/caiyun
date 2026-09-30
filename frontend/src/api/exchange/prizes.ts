import request from '../axios'
import { ApiContractError, requireApiData, type ApiResponse } from '../response'

import type { PendingPrize as PendingPrizeContract, PendingPrizeList as PendingPrizeListContract } from '../generated/operation-contract'

export type PendingPrize = PendingPrizeContract
export type PendingPrizeList = PendingPrizeListContract

export function getPendingPrizes(accountID: number, refresh = false, signal?: AbortSignal): Promise<PendingPrizeList> {
  return request<PendingPrizeList | ApiResponse<PendingPrizeList>>({
    url: '/api/v1/exchange/prizes',
    params: { account_id: accountID, refresh: refresh ? 1 : undefined },
    signal
  }).then(response => {
    const data = requireApiData(response, '待领奖品列表')
    if (data.account_id !== accountID || !Array.isArray(data.prizes)) throw new ApiContractError('待领奖品列表格式或账号不匹配')
    return data
  })
}
