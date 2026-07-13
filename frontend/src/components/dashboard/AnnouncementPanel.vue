<template>
  <el-card
    shadow="hover"
    class="announcement-card"
  >
    <template #header>
      <div class="card-header">
        <div class="announcement-title">
          <el-icon><Bell /></el-icon>
          <span>公告</span>
          <el-tag
            v-if="unreadCount > 0"
            type="danger"
            size="small"
          >
            {{ unreadCount }} 条未读
          </el-tag>
        </div>
        <el-button
          v-if="announcements.length > 0"
          type="primary"
          size="small"
          class="announcement-read-all-btn"
          @click="$emit('mark-all-read')"
        >
          全部标为已读
        </el-button>
      </div>
    </template>

    <div
      v-loading="loading"
      class="announcement-list"
    >
      <el-empty
        v-if="!loading && announcements.length === 0"
        description="暂无公告"
        :image-size="80"
      />
      <div
        v-for="item in announcements"
        :key="item.id"
        class="announcement-item"
        :class="{ unread: !isRead(item.id), top: item.is_top }"
        @click="$emit('open', item)"
      >
        <div class="announcement-main">
          <div class="announcement-line">
            <el-tag
              v-if="item.is_top"
              type="danger"
              size="small"
            >
              置顶
            </el-tag>
            <el-tag
              v-if="item.is_popup"
              type="warning"
              size="small"
            >
              弹窗
            </el-tag>
            <el-tag
              v-if="!isRead(item.id)"
              type="success"
              size="small"
            >
              未读
            </el-tag>
            <span class="announcement-name">{{ item.title }}</span>
          </div>
          <div class="announcement-preview">
            {{ item.content }}
          </div>
        </div>
        <div class="announcement-date">
          {{ formatDateTime(item.created_at) }}
        </div>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import type { Announcement } from '../../api/announcement'

defineProps<{
  announcements: Announcement[]
  loading: boolean
  unreadCount: number
  isRead: (id: number) => boolean
}>()

defineEmits<{
  (event: 'mark-all-read'): void
  (event: 'open', announcement: Announcement): void
}>()

const formatDateTime = (value: string) => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}
</script>

<style scoped>
.announcement-card :deep(.el-card__body) {
  padding: 0;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #1e40af;
  font-weight: 600;
}

.announcement-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #1e3a8a;
  font-size: 18px;
  font-weight: 700;
}

.announcement-read-all-btn {
  color: #ffffff;
  font-weight: 700;
  background: #2563eb;
  border-color: #2563eb;
  box-shadow: 0 8px 18px rgba(37, 99, 235, 0.24);
}

.announcement-read-all-btn:hover,
.announcement-read-all-btn:focus {
  color: #ffffff;
  background: #1d4ed8;
  border-color: #1d4ed8;
}

.announcement-list {
  min-height: 96px;
  max-height: 280px;
  overflow-y: auto;
}

.announcement-item {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
  cursor: pointer;
  transition: background 0.2s ease, transform 0.2s ease;
}

.announcement-item:last-child {
  border-bottom: none;
}

.announcement-item:hover {
  background: rgba(239, 246, 255, 0.72);
}

.announcement-item.unread {
  background: rgba(236, 253, 245, 0.52);
}

.announcement-item.top {
  border-left: 3px solid #ef4444;
}

.announcement-main {
  min-width: 0;
  flex: 1;
}

.announcement-line {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.announcement-name {
  font-size: 15px;
  font-weight: 700;
  color: #1e3a8a;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.announcement-preview {
  margin-top: 6px;
  color: #334155;
  font-size: 14px;
  font-weight: 500;
  line-height: 1.55;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.announcement-date {
  flex-shrink: 0;
  color: #475569;
  font-size: 13px;
  font-weight: 500;
  white-space: nowrap;
  padding-top: 2px;
}
</style>
