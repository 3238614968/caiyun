import { http } from './axios'

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

// 获取已发布的公告列表
export function getAnnouncements() {
  return http.get('/api/announcements')
}

// 获取弹窗公告
export function getPopupAnnouncement() {
  return http.get('/api/announcements/popup')
}

// 管理员：获取所有公告
export function getAllAnnouncements() {
  return http.get('/api/admin/announcements')
}

// 管理员：创建公告
export function createAnnouncement(data: CreateAnnouncementRequest) {
  return http.post('/api/admin/announcements', data)
}

// 管理员：更新公告
export function updateAnnouncement(id: number, data: UpdateAnnouncementRequest) {
  return http.put(`/api/admin/announcements/${id}`, data)
}

// 管理员：删除公告
export function deleteAnnouncement(id: number) {
  return http.delete(`/api/admin/announcements/${id}`)
}

// 管理员：获取公告详情
export function getAnnouncementDetail(id: number) {
  return http.get(`/api/admin/announcements/${id}`)
}
