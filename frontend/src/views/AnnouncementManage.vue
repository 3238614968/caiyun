<template>
  <div class="announcement-manage">
    <div class="page-header">
      <div class="header-title">
        <el-icon :size="isMobile ? 20 : 24">
          <Bell />
        </el-icon>
        <span>公告管理</span>
      </div>
      <el-button
        type="primary"
        :size="isMobile ? 'small' : 'default'"
        @click="showAddDialog"
      >
        <el-icon><Plus /></el-icon>
        {{ isMobile ? '发布' : '发布公告' }}
      </el-button>
    </div>

    <el-card class="announcement-list">
      <!-- 桌面端表格 -->
      <el-table
        v-if="!isMobile"
        v-loading="loading"
        :data="announcements"
        stripe
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
              @click="viewAnnouncement(row)"
            >
              查看
            </el-button>
            <el-button
              size="small"
              type="primary"
              @click="editAnnouncement(row)"
            >
              编辑
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="deleteAnnouncement(row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <!-- 移动端卡片列表 -->
      <div
        v-else
        class="mobile-card-list"
      >
        <el-card
          v-for="item in announcements"
          :key="item.id"
          class="mobile-announcement-card"
          shadow="hover"
        >
          <div class="mobile-announcement-header">
            <span class="mobile-announcement-title">{{ item.title }}</span>
            <div class="mobile-announcement-tags">
              <el-tag
                v-if="item.is_top"
                type="danger"
                size="small"
                effect="dark"
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
                :type="item.is_published ? 'success' : 'info'"
                size="small"
              >
                {{ item.is_published ? '已发布' : '已下架' }}
              </el-tag>
            </div>
          </div>
          <div class="mobile-announcement-time">
            {{ formatDate(item.created_at) }}
          </div>
          <div class="mobile-announcement-actions">
            <el-button
              size="small"
              @click="viewAnnouncement(item)"
            >
              查看
            </el-button>
            <el-button
              size="small"
              type="primary"
              @click="editAnnouncement(item)"
            >
              编辑
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="deleteAnnouncement(item)"
            >
              删除
            </el-button>
          </div>
        </el-card>
      </div>
    </el-card>

    <!-- 添加/编辑公告对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? '编辑公告' : '发布公告'"
      :width="isMobile ? '95%' : '700px'"
      class="announcement-dialog"
    >
      <el-form
        ref="formRef"
        :model="form"
        label-position="top"
        :rules="rules"
      >
        <el-form-item
          label="公告标题"
          prop="title"
        >
          <el-input
            v-model="form.title"
            placeholder="请输入公告标题"
            maxlength="100"
            show-word-limit
          />
        </el-form-item>
        <el-form-item
          label="公告内容"
          prop="content"
        >
          <el-input
            v-model="form.content"
            type="textarea"
            :rows="6"
            placeholder="请输入公告内容"
            maxlength="2000"
            show-word-limit
          />
        </el-form-item>
        <el-form-item>
          <div class="form-options">
            <el-checkbox
              v-model="form.is_popup"
              label="弹窗显示"
              border
            />
            <el-checkbox
              v-model="form.is_top"
              label="置顶"
              border
            />
            <el-checkbox
              v-if="isEditing"
              v-model="form.is_published"
              label="发布状态"
              border
            />
          </div>
        </el-form-item>
        <el-form-item
          v-if="form.is_popup"
          class="tip-item"
        >
          <el-alert
            title="弹窗公告说明"
            type="info"
            :closable="false"
            description="开启弹窗后，仅置顶且未读的弹窗公告会自动弹出一次；其他已发布公告会展示在首页公告列表中。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="dialogVisible = false">
          取消
        </el-button>
        <el-button
          type="primary"
          :loading="submitting"
          @click="submitForm"
        >
          确定
        </el-button>
      </template>
    </el-dialog>

    <!-- 查看公告对话框 -->
    <el-dialog
      v-model="viewDialogVisible"
      title="公告详情"
      :width="isMobile ? '95%' : '600px'"
      class="view-dialog"
    >
      <div class="view-content">
        <h3 class="view-title">
          {{ currentAnnouncement?.title }}
        </h3>
        <div class="view-meta">
          <el-tag
            v-if="currentAnnouncement?.is_top"
            type="danger"
            size="small"
          >
            置顶
          </el-tag>
          <el-tag
            v-if="currentAnnouncement?.is_popup"
            type="warning"
            size="small"
          >
            弹窗
          </el-tag>
          <span class="view-time">{{ formatDate(currentAnnouncement?.created_at) }}</span>
        </div>
        <div class="view-body">
          {{ currentAnnouncement?.content }}
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Bell, Plus } from '@element-plus/icons-vue'
import {
  getAllAnnouncements,
  createAnnouncement,
  updateAnnouncement,
  deleteAnnouncement as apiDeleteAnnouncement,
  type Announcement,
  type CreateAnnouncementRequest,
  type UpdateAnnouncementRequest
} from '@/api/announcement'

const loading = ref(false)
const announcements = ref<Announcement[]>([])
const dialogVisible = ref(false)
const viewDialogVisible = ref(false)
const isEditing = ref(false)
const submitting = ref(false)
const currentAnnouncement = ref<Announcement | null>(null)
const formRef = ref()

// 移动端检测
const isMobile = ref(false)
const checkMobile = () => {
  isMobile.value = window.innerWidth <= 768
}

const form = ref({
  id: 0,
  title: '',
  content: '',
  is_popup: false,
  is_top: false,
  is_published: true
})

const rules = {
  title: [{ required: true, message: '请输入公告标题', trigger: 'blur' }],
  content: [{ required: true, message: '请输入公告内容', trigger: 'blur' }]
}

const formatDate = (date: string | undefined) => {
  if (!date) return ''
  return new Date(date).toLocaleString('zh-CN')
}

const loadAnnouncements = async () => {
  loading.value = true
  try {
    const res: any = await getAllAnnouncements()
    announcements.value = res.announcements || []
  } catch (error: any) {
    ElMessage.error('加载公告失败：' + error.message)
  } finally {
    loading.value = false
  }
}

const showAddDialog = () => {
  isEditing.value = false
  form.value = {
    id: 0,
    title: '',
    content: '',
    is_popup: false,
    is_top: false,
    is_published: true
  }
  dialogVisible.value = true
}

const editAnnouncement = (row: Announcement) => {
  isEditing.value = true
  form.value = {
    id: row.id,
    title: row.title,
    content: row.content,
    is_popup: row.is_popup,
    is_top: row.is_top,
    is_published: row.is_published
  }
  dialogVisible.value = true
}

const viewAnnouncement = (row: Announcement) => {
  currentAnnouncement.value = row
  viewDialogVisible.value = true
}

const submitForm = async () => {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    if (isEditing.value) {
      const data: UpdateAnnouncementRequest = {
        title: form.value.title,
        content: form.value.content,
        is_popup: form.value.is_popup,
        is_top: form.value.is_top,
        is_published: form.value.is_published
      }
      await updateAnnouncement(form.value.id, data)
      ElMessage.success('更新成功')
    } else {
      const data: CreateAnnouncementRequest = {
        title: form.value.title,
        content: form.value.content,
        is_popup: form.value.is_popup,
        is_top: form.value.is_top
      }
      await createAnnouncement(data)
      ElMessage.success('发布成功')
    }
    dialogVisible.value = false
    loadAnnouncements()
  } catch (error: any) {
    ElMessage.error(isEditing.value ? '更新失败：' : '发布失败：' + error.message)
  } finally {
    submitting.value = false
  }
}

const deleteAnnouncement = async (row: Announcement) => {
  try {
    await ElMessageBox.confirm('确定要删除这条公告吗？', '确认删除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await apiDeleteAnnouncement(row.id)
    ElMessage.success('删除成功')
    loadAnnouncements()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

onMounted(() => {
  checkMobile()
  window.addEventListener('resize', checkMobile)
  loadAnnouncements()
})

onUnmounted(() => {
  window.removeEventListener('resize', checkMobile)
})
</script>

<style scoped>
.announcement-manage {
  padding: 20px;
}

.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}

.announcement-list {
  min-height: 400px;
}

.title-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ml-2 {
  margin-left: 8px;
}

.form-options {
  display: flex;
  gap: 16px;
}

/* 移动端样式 */
.mobile-card-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.mobile-announcement-card {
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.7);
  backdrop-filter: blur(10px);
  -webkit-backdrop-filter: blur(10px);
  border: 1px solid rgba(255, 255, 255, 0.6);
}

.mobile-announcement-header {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 8px;
}

.mobile-announcement-title {
  font-size: 15px;
  font-weight: 600;
  color: #1e40af;
  line-height: 1.4;
}

.mobile-announcement-tags {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}

.mobile-announcement-time {
  font-size: 12px;
  color: #6b7280;
  margin-bottom: 12px;
}

.mobile-announcement-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.tip-item {
  margin-bottom: 0;
}

.view-content {
  padding: 10px;
}

.view-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px 0;
  line-height: 1.4;
}

.view-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #e4e7ed;
}

.view-time {
  color: #909399;
  font-size: 13px;
}

.view-body {
  font-size: 14px;
  line-height: 1.8;
  color: #606266;
  white-space: pre-wrap;
}

/* 移动端适配 */
@media (max-width: 768px) {
  .announcement-manage {
    padding: 12px;
  }

  .page-header {
    margin-bottom: 16px;
  }

  .header-title {
    font-size: 18px;
  }
}
</style>
