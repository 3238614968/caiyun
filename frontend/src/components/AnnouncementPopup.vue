<template>
  <el-dialog
    v-model="visible"
    title="公告"
    width="500px"
    :close-on-click-modal="false"
    :close-on-press-escape="false"
    class="announcement-popup"
    align-center
  >
    <div class="popup-content">
      <div class="popup-icon">
        <el-icon
          :size="48"
          color="#409EFF"
        >
          <Bell />
        </el-icon>
      </div>
      <h3 class="popup-title">
        {{ announcement?.title }}
      </h3>
      <div class="popup-body">
        {{ announcement?.content }}
      </div>
      <div
        v-if="announcement?.created_at"
        class="popup-time"
      >
        发布时间：{{ formatDate(announcement.created_at) }}
      </div>
    </div>
    <template #footer>
      <div class="popup-footer">
        <span class="popup-hint">确认后本公告不会再次自动弹出</span>
        <el-button
          type="primary"
          @click="closePopup"
        >
          我知道了
        </el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { Bell } from '@element-plus/icons-vue'
import type { Announcement } from '@/api/announcement'

const props = defineProps<{
  modelValue: boolean
  announcement: Announcement | null
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  'dismiss': []
}>()

const visible = ref(props.modelValue)

watch(() => props.modelValue, (val) => {
  visible.value = val
})

watch(visible, (val) => {
  emit('update:modelValue', val)
})

const formatDate = (date: string) => {
  return new Date(date).toLocaleString('zh-CN')
}

const closePopup = () => {
  visible.value = false
  emit('dismiss')
}
</script>

<style scoped>
.announcement-popup :deep(.el-dialog__header) {
  display: none;
}

.announcement-popup :deep(.el-dialog__body) {
  padding: 30px;
}

.popup-content {
  text-align: center;
}

.popup-icon {
  width: 80px;
  height: 80px;
  background: linear-gradient(135deg, #e3f2fd 0%, #bbdefb 100%);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto 20px;
}

.popup-title {
  font-size: 20px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px 0;
}

.popup-body {
  font-size: 14px;
  line-height: 1.8;
  color: #606266;
  text-align: left;
  white-space: pre-wrap;
  max-height: 300px;
  overflow-y: auto;
  padding: 0 10px;
}

.popup-time {
  margin-top: 16px;
  font-size: 12px;
  color: #909399;
}

.popup-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.popup-hint {
  font-size: 12px;
  color: #909399;
}
</style>
