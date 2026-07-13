<template>
  <div class="announcement-content">
    <div class="announcement-header">
      <el-button
        type="primary"
        @click="$emit('create')"
      >
        <el-icon><Plus /></el-icon>
        发布公告
      </el-button>
    </div>
    <div
      v-loading="loading"
      class="responsive-data-shell"
    >
      <el-table
        v-if="!isMobile"
        :data="announcements"
        stripe
        style="width: 100%"
      >
        <el-table-column
          type="index"
          width="50"
        />
        <el-table-column
          prop="title"
          label="标题"
          min-width="200"
        >
          <template #default="{ row }">
            <div class="title-cell">
              <el-tag
                v-if="row.is_top"
                type="danger"
                size="small"
                effect="dark"
              >
                置顶
              </el-tag>
              <el-tag
                v-if="row.is_popup"
                type="warning"
                size="small"
                class="ml-2"
              >
                弹窗
              </el-tag>
              <span class="title-text">{{ row.title }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column
          prop="is_published"
          label="状态"
          width="100"
        >
          <template #default="{ row }">
            <el-tag :type="row.is_published ? 'success' : 'info'">
              {{ row.is_published ? '已发布' : '已下架' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="created_at"
          label="创建时间"
          width="180"
        >
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column
          label="操作"
          width="200"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              size="small"
              @click="$emit('view', row)"
            >
              查看
            </el-button>
            <el-button
              size="small"
              type="primary"
              @click="$emit('edit', row)"
            >
              编辑
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="$emit('delete', row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div
        v-else
        class="mobile-admin-list"
      >
        <el-empty
          v-if="announcements.length === 0"
          description="暂无公告"
        />
        <template v-else>
          <el-card
            v-for="row in announcements"
            :key="row.id"
            class="mobile-admin-card"
            shadow="never"
          >
            <div class="mobile-admin-card-head">
              <div>
                <div class="mobile-admin-card-title">
                  {{ row.title }}
                </div>
                <div class="mobile-admin-inline-tags">
                  <el-tag
                    v-if="row.is_top"
                    type="danger"
                    size="small"
                    effect="dark"
                  >
                    置顶
                  </el-tag>
                  <el-tag
                    v-if="row.is_popup"
                    type="warning"
                    size="small"
                  >
                    弹窗
                  </el-tag>
                </div>
              </div>
              <el-tag :type="row.is_published ? 'success' : 'info'">
                {{ row.is_published ? '已发布' : '已下架' }}
              </el-tag>
            </div>
            <div class="mobile-admin-card-grid">
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">创建时间</span>
                <span class="mobile-admin-card-value">{{ formatDate(row.created_at) }}</span>
              </div>
              <div class="mobile-admin-card-row full">
                <span class="mobile-admin-card-label">内容预览</span>
                <span class="mobile-admin-card-value multiline">{{ row.content || '-' }}</span>
              </div>
            </div>
            <div class="mobile-admin-card-actions">
              <el-button @click="$emit('view', row)">
                查看
              </el-button>
              <el-button
                type="primary"
                @click="$emit('edit', row)"
              >
                编辑
              </el-button>
              <el-button
                type="danger"
                @click="$emit('delete', row)"
              >
                删除
              </el-button>
            </div>
          </el-card>
        </template>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Plus } from '@element-plus/icons-vue'
import type { Announcement } from '@/api/announcement'

defineProps<{
  isMobile: boolean
  loading: boolean
  announcements: Announcement[]
  formatDate: (date?: string) => string
}>()

defineEmits<{
  create: []
  view: [row: Announcement]
  edit: [row: Announcement]
  delete: [row: Announcement]
}>()
</script>

<style scoped>
.announcement-content { display: grid; gap: 16px; }
.announcement-header { display: flex; justify-content: flex-end; }
.responsive-data-shell { min-height: 120px; }
.title-cell { display: flex; align-items: center; gap: 6px; min-width: 0; }
.title-text { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.ml-2 { margin-left: 4px; }
.mobile-admin-list { display: grid; gap: 12px; }
.mobile-admin-card { border-radius: 18px; border: 1px solid rgba(226, 232, 240, 0.9); background: rgba(255, 255, 255, 0.94); box-shadow: 0 14px 28px rgba(37, 99, 235, 0.08); }
.mobile-admin-card :deep(.el-card__body) { padding: 16px; }
.mobile-admin-card-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.mobile-admin-card-title { font-size: 15px; font-weight: 700; color: #0f172a; line-height: 1.4; word-break: break-word; }
.mobile-admin-inline-tags { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.mobile-admin-card-grid { display: grid; gap: 10px; }
.mobile-admin-card-row { display: grid; grid-template-columns: 84px minmax(0, 1fr); gap: 10px; align-items: start; }
.mobile-admin-card-row.full { grid-template-columns: 1fr; }
.mobile-admin-card-label { font-size: 12px; color: #64748b; font-weight: 600; }
.mobile-admin-card-value { font-size: 13px; color: #334155; word-break: break-word; }
.mobile-admin-card-value.multiline { display: -webkit-box; -webkit-line-clamp: 3; -webkit-box-orient: vertical; overflow: hidden; line-height: 1.55; }
.mobile-admin-card-actions { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; margin-top: 14px; padding-top: 12px; border-top: 1px solid rgba(226, 232, 240, 0.8); }
.mobile-admin-card-actions :deep(.el-button) { margin-left: 0; min-width: 0; padding-left: 6px; padding-right: 6px; }
@media (max-width: 768px) {
  .announcement-header { justify-content: stretch; }
  .announcement-header :deep(.el-button) { width: 100%; justify-content: center; }
  .mobile-admin-card-row { grid-template-columns: 78px minmax(0, 1fr); }
  .mobile-admin-card-actions { grid-template-columns: 1fr; }
}
</style>
