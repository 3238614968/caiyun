import request from './axios'
import { unwrapApiData, unwrapOperationResponse, type ApiResponse, type OperationResponse } from './response'
import { operationHeaders } from './operation'
import type {
  CloudStat as CloudStatContract,
  CloudStatList as CloudStatListContract,
  DashboardAccountRank as DashboardAccountRankContract,
  DashboardData as DashboardDataContract,
  DashboardTrendPoint as DashboardTrendPointContract,
  QueueStatus as QueueStatusContract,
  TaskLog as TaskLogContract,
  TaskLogList as TaskLogListContract,
  TaskStatus as TaskStatusContract,
  TaskStatusItem as TaskStatusItemContract,
  TotalCloudCount as TotalCloudCountContract,
  TrendData as TrendDataContract
} from './generated/operation-contract'

// 高频任务读取 DTO 由 OpenAPI 契约生成，避免前后端字段静默漂移。
export type TaskLog = TaskLogContract

// 统计读取 DTO 也由 OpenAPI 生成，手写查询参数保留在客户端边界。
export type CloudStats = CloudStatContract
export type DashboardData = DashboardDataContract
export type TrendPoint = DashboardTrendPointContract
export type AccountRank = DashboardAccountRankContract
export type CloudStatsResponse = CloudStatListContract
export type TrendDataResponse = TrendDataContract
export type TotalCloudCountResponse = TotalCloudCountContract

export type TaskLogsResponse = TaskLogListContract

// 云朵统计响应
export interface TaskLogQuery {
  account_id?: number
  task_type?: string
  status?: string
  page?: number
  page_size?: number
}

// 获取任务日志
export function getTaskLogs(accountIdOrQuery?: number | TaskLogQuery, page: number = 1, pageSize: number = 20): Promise<TaskLogsResponse> {
  const query: TaskLogQuery = typeof accountIdOrQuery === 'object' && accountIdOrQuery !== null
    ? { ...accountIdOrQuery }
    : { account_id: accountIdOrQuery, page, page_size: pageSize }
  const normalizedPage = query.page ?? page
  const normalizedPageSize = query.page_size ?? pageSize
  const fallback: TaskLogsResponse = { task_logs: [], total: 0, page: normalizedPage, page_size: normalizedPageSize }

  return request<TaskLogsResponse | ApiResponse<TaskLogsResponse>>({
    url: '/api/v1/tasks/logs',
    method: 'get',
    params: {
      account_id: query.account_id,
      task_type: query.task_type,
      status: query.status,
      page: normalizedPage,
      page_size: normalizedPageSize
    }
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取仪表盘数据
export function getDashboard(): Promise<DashboardData> {
  const fallback: DashboardData = {
    total_cloud: 0,
    account_count: 0,
    today_gained: 0,
    yesterday_diff: 0,
    week_diff: 0,
    success_rate: 0,
    trend_data: [],
    account_ranking: []
  }

  return request<{ data: DashboardData } | ApiResponse<DashboardData>>({
    url: '/api/v1/stats/dashboard',
    method: 'get'
  }).then((res) => {
    const unified = unwrapApiData(res as ApiResponse<DashboardData>, fallback)
    if ('total_cloud' in unified) {
      return unified
    }
    return res?.data ?? fallback
  })
}

// 获取云朵统计
export function getCloudStats(accountId?: number, page: number = 1, pageSize: number = 10): Promise<CloudStatsResponse> {
  const fallback: CloudStatsResponse = { cloud_stats: [], total: 0, page, page_size: pageSize }
  return request<CloudStatsResponse | ApiResponse<CloudStatsResponse>>({
    url: '/api/v1/stats/cloud',
    method: 'get',
    params: { account_id: accountId, page, page_size: pageSize }
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取趋势数据
export function getTrendData(days: number = 7): Promise<TrendDataResponse> {
  const fallback: TrendDataResponse = { trend_data: [] }
  return request<TrendDataResponse | ApiResponse<TrendDataResponse>>({
    url: '/api/v1/stats/trend',
    method: 'get',
    params: { days }
  }).then((res) => unwrapApiData(res, fallback))
}

// 触发所有账号的任务
export function triggerAllTasks(): Promise<OperationResponse> {
  return request<OperationResponse | ApiResponse<OperationResponse>>({ url: '/api/v1/tasks/trigger-all', method: 'post', headers: operationHeaders() }).then(unwrapOperationResponse)
}

// 计算统计数据
export function calculateStats(): Promise<{ message: string }> {
  return request({
    url: '/api/v1/stats/calculate',
    method: 'post'
  })
}

// 获取总云朵数
export function getTotalCloudCount(): Promise<TotalCloudCountResponse> {
  const fallback: TotalCloudCountResponse = { total_cloud: 0 }
  return request<TotalCloudCountResponse | ApiResponse<TotalCloudCountResponse>>({
    url: '/api/v1/stats/total-cloud',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

// 任务状态 DTO 以契约字段为准；前端保留历史 status 联合类型以兼容推送事件。
export type TaskStatus = Omit<TaskStatusItemContract, 'status'> & {
  status: TaskStatusItemContract['status'] | 'retrying'
}
export type TaskStatusResponse = Omit<TaskStatusContract, 'tasks'> & { tasks: TaskStatus[] }

export type QueueStatus = QueueStatusContract

// 获取队列状态
export function getQueueStatus(): Promise<QueueStatus> {
  const fallback: QueueStatus = {
    backend: 'unknown',
    backend_meta: undefined,
    is_healthy: false,
    errors: [],
    queue_length: 0,
    processing_count: 0,
    delayed_count: 0,
    dead_letter_count: 0,
    active_workers: 0,
    pending_tasks: 0,
    completed_tasks: 0,
    successful_tasks: 0,
    failed_tasks: 0
  }

  return request<QueueStatus | ApiResponse<QueueStatus>>({
    url: '/api/v1/tasks/queue-status',
    method: 'get'
  }).then((res) => unwrapApiData(res, fallback))
}

// 获取任务状态
export function getTaskStatus(accountId?: number): Promise<TaskStatus[]> {
  // 后端返回: { tasks: TaskStatus[] }
  const fallback: TaskStatusResponse = { tasks: [] }
  return request<TaskStatusResponse | ApiResponse<TaskStatusResponse>>({
    url: '/api/v1/tasks/status',
    method: 'get',
    params: { account_id: accountId }
  }).then((res) => unwrapApiData(res, fallback).tasks)
}
