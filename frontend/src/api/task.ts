import request from './axios'

// 任务日志接口
export interface TaskLog {
  id: number
  user_id: number
  account_id: number
  task_type: string
  status: string
  message: string
  cloud_gained: number
  execution_time: number
  created_at: string
}

// 云朵统计接口
export interface CloudStats {
  id: number
  user_id: number
  account_id: number
  date: string
  cloud_count: number
  cloud_diff: number
  cloud_diff_week: number
  created_at: string
  updated_at: string
}

// 仪表盘数据接口
export interface DashboardData {
  total_cloud: number
  account_count: number
  today_gained: number
  yesterday_diff: number
  week_diff: number
  success_rate: number
  trend_data: TrendPoint[]
  account_ranking: AccountRank[]
}

// 趋势点接口
export interface TrendPoint {
  date: string
  cloud_count: number
}

// 账号排名接口
export interface AccountRank {
  account_id: number
  phone: string
  remark: string
  cloud_count: number
}

// 任务日志响应
export interface TaskLogsResponse {
  task_logs: TaskLog[]
  total: number
  page: number
  page_size: number
}

// 云朵统计响应
export interface CloudStatsResponse {
  cloud_stats: CloudStats[]
  total: number
  page: number
  page_size: number
}

// 获取任务日志
export function getTaskLogs(accountId?: number, page: number = 1, pageSize: number = 20): Promise<TaskLogsResponse> {
  return request({
    url: '/api/tasks/logs',
    method: 'get',
    params: { account_id: accountId, page, page_size: pageSize }
  })
}

// 获取仪表盘数据
export function getDashboard(): Promise<DashboardData> {
  // 后端返回: { data: DashboardData }
  return request<{ data: DashboardData }>({
    url: '/api/stats/dashboard',
    method: 'get'
  }).then((res) => {
    // 防御：后端异常/被代理返回 HTML 等情况下，避免前端崩溃
    return res?.data ?? {
      total_cloud: 0,
      account_count: 0,
      today_gained: 0,
      yesterday_diff: 0,
      week_diff: 0,
      success_rate: 0,
      trend_data: [],
      account_ranking: []
    }
  })
}

// 获取云朵统计
export function getCloudStats(accountId?: number, page: number = 1, pageSize: number = 10): Promise<CloudStatsResponse> {
  return request({
    url: '/api/stats/cloud',
    method: 'get',
    params: { account_id: accountId, page, page_size: pageSize }
  })
}

// 获取趋势数据
export function getTrendData(days: number = 7): Promise<{ trend_data: TrendPoint[] }> {
  return request({
    url: '/api/stats/trend',
    method: 'get',
    params: { days }
  })
}

// 触发所有账号的任务
export function triggerAllTasks(): Promise<{ message: string }> {
  return request({
    url: '/api/tasks/trigger-all',
    method: 'post'
  })
}

// 计算统计数据
export function calculateStats(): Promise<{ message: string }> {
  return request({
    url: '/api/stats/calculate',
    method: 'post'
  })
}

// 获取总云朵数
export function getTotalCloudCount(): Promise<{ total_cloud: number }> {
  return request({
    url: '/api/stats/total-cloud',
    method: 'get'
  })
}

// 任务状态接口
export interface TaskStatus {
  account_id: number
  task_type: string
  status: 'pending' | 'running' | 'success' | 'failed' | 'retrying'
  progress: number
  message: string
  start_time?: string
  end_time?: string
}

// 队列状态接口
export interface QueueStatus {
  queue_length: number
  active_workers: number
  pending_tasks: number
  completed_tasks: number
  successful_tasks: number
  failed_tasks: number
}

// 获取队列状态
export function getQueueStatus(): Promise<QueueStatus> {
  return request({
    url: '/api/tasks/queue-status',
    method: 'get'
  })
}

// 获取任务状态
export function getTaskStatus(accountId?: number): Promise<TaskStatus[]> {
  // 后端返回: { tasks: TaskStatus[] }
  return request<{ tasks: TaskStatus[] }>({
    url: '/api/tasks/status',
    method: 'get',
    params: { account_id: accountId }
  }).then((res) => (Array.isArray(res?.tasks) ? res.tasks : []))
}
