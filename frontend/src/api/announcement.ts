import { http } from './axios'
import { unwrapApiData, type ApiResponse } from './response'
import type {
  Announcement as AnnouncementContract,
  AnnouncementList as AnnouncementListContract,
  AnnouncementPopupList as AnnouncementPopupListContract,
  AnnouncementResponse as AnnouncementResponseContract,
  CreateAnnouncementRequest as CreateAnnouncementRequestContract,
  UpdateAnnouncementRequest as UpdateAnnouncementRequestContract
} from './generated/operation-contract'

export type Announcement = AnnouncementContract
export type CreateAnnouncementRequest = CreateAnnouncementRequestContract
export type UpdateAnnouncementRequest = UpdateAnnouncementRequestContract
export type AnnouncementListResponse = AnnouncementListContract
// Null is retained only for the existing fallback UI state; successful responses use AnnouncementResponseContract.
export type AnnouncementDetailResponse = AnnouncementResponseContract | { announcement: null }
export type PopupAnnouncementResponse = AnnouncementPopupListContract

const emptyList: AnnouncementListResponse = {
  announcements: [],
  total: 0
}

// 获取已发布的公告列表
export function getAnnouncements(): Promise<AnnouncementListResponse> {
  return http.get<AnnouncementListResponse | ApiResponse<AnnouncementListResponse>>('/api/v1/announcements')
    .then((res) => unwrapApiData(res, emptyList))
}

// 获取弹窗公告
export function getPopupAnnouncement(): Promise<PopupAnnouncementResponse> {
  const fallback: PopupAnnouncementResponse = { has_popup: false, announcements: [] }
  return http.get<PopupAnnouncementResponse | ApiResponse<PopupAnnouncementResponse>>('/api/v1/announcements/popup')
    .then((res) => unwrapApiData(res, fallback))
}

// 管理员：获取所有公告
export function getAllAnnouncements(): Promise<AnnouncementListResponse> {
  return http.get<AnnouncementListResponse | ApiResponse<AnnouncementListResponse>>('/api/v1/admin/announcements')
    .then((res) => unwrapApiData(res, emptyList))
}

// 管理员：创建公告
export function createAnnouncement(data: CreateAnnouncementRequest): Promise<AnnouncementDetailResponse> {
  const fallback: AnnouncementDetailResponse = { announcement: null }
  return http.post<AnnouncementDetailResponse | ApiResponse<AnnouncementDetailResponse>>('/api/v1/admin/announcements', data)
    .then((res) => unwrapApiData(res, fallback))
}

// 管理员：更新公告
export function updateAnnouncement(id: number, data: UpdateAnnouncementRequest): Promise<AnnouncementDetailResponse> {
  const fallback: AnnouncementDetailResponse = { announcement: null }
  return http.put<AnnouncementDetailResponse | ApiResponse<AnnouncementDetailResponse>>(`/api/v1/admin/announcements/${id}`, data)
    .then((res) => unwrapApiData(res, fallback))
}

// 管理员：删除公告
export function deleteAnnouncement(id: number): Promise<{ message: string }> {
  return http.delete<{ message: string }>(`/api/v1/admin/announcements/${id}`)
}

// 管理员：获取公告详情
export function getAnnouncementDetail(id: number): Promise<AnnouncementDetailResponse> {
  const fallback: AnnouncementDetailResponse = { announcement: null }
  return http.get<AnnouncementDetailResponse | ApiResponse<AnnouncementDetailResponse>>(`/api/v1/admin/announcements/${id}`)
    .then((res) => unwrapApiData(res, fallback))
}
