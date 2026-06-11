import { http } from './axios'
import { unwrapApiData, type ApiResponse } from './response'

export interface Announcement {
  id: number
  title: string
  content: string
  is_popup: boolean
  is_top: boolean
  is_published: boolean
  popup_count: number
  created_at: string
  updated_at: string
}

export interface CreateAnnouncementRequest {
  title: string
  content: string
  is_popup: boolean
  is_top: boolean
}

export interface UpdateAnnouncementRequest {
  title: string
  content: string
  is_popup: boolean
  is_top: boolean
  is_published: boolean
}

export interface AnnouncementListResponse {
  announcements: Announcement[]
  total: number
}

export interface AnnouncementDetailResponse {
  announcement: Announcement | null
}

export interface PopupAnnouncementResponse {
  has_popup: boolean
  announcements: Announcement[]
}

const emptyList: AnnouncementListResponse = {
  announcements: [],
  total: 0
}

// 获取已发布的公告列表
export function getAnnouncements(): Promise<AnnouncementListResponse> {
  return http.get<AnnouncementListResponse | ApiResponse<AnnouncementListResponse>>('/api/announcements')
    .then((res) => unwrapApiData(res, emptyList))
}

// 获取弹窗公告
export function getPopupAnnouncement(): Promise<PopupAnnouncementResponse> {
  const fallback: PopupAnnouncementResponse = { has_popup: false, announcements: [] }
  return http.get<PopupAnnouncementResponse | ApiResponse<PopupAnnouncementResponse>>('/api/announcements/popup')
    .then((res) => unwrapApiData(res, fallback))
}

// 管理员：获取所有公告
export function getAllAnnouncements(): Promise<AnnouncementListResponse> {
  return http.get<AnnouncementListResponse | ApiResponse<AnnouncementListResponse>>('/api/admin/announcements')
    .then((res) => unwrapApiData(res, emptyList))
}

// 管理员：创建公告
export function createAnnouncement(data: CreateAnnouncementRequest): Promise<AnnouncementDetailResponse> {
  const fallback: AnnouncementDetailResponse = { announcement: null }
  return http.post<AnnouncementDetailResponse | ApiResponse<AnnouncementDetailResponse>>('/api/admin/announcements', data)
    .then((res) => unwrapApiData(res, fallback))
}

// 管理员：更新公告
export function updateAnnouncement(id: number, data: UpdateAnnouncementRequest): Promise<AnnouncementDetailResponse> {
  const fallback: AnnouncementDetailResponse = { announcement: null }
  return http.put<AnnouncementDetailResponse | ApiResponse<AnnouncementDetailResponse>>(`/api/admin/announcements/${id}`, data)
    .then((res) => unwrapApiData(res, fallback))
}

// 管理员：删除公告
export function deleteAnnouncement(id: number): Promise<{ message: string }> {
  return http.delete<{ message: string }>(`/api/admin/announcements/${id}`)
}

// 管理员：获取公告详情
export function getAnnouncementDetail(id: number): Promise<AnnouncementDetailResponse> {
  const fallback: AnnouncementDetailResponse = { announcement: null }
  return http.get<AnnouncementDetailResponse | ApiResponse<AnnouncementDetailResponse>>(`/api/admin/announcements/${id}`)
    .then((res) => unwrapApiData(res, fallback))
}
