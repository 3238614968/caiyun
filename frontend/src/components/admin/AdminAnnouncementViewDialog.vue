<template>
  <el-dialog
    v-model="visible"
    title="公告详情"
    width="600px"
    class="view-dialog"
  >
    <div class="view-content">
      <h3 class="view-title">
        {{ announcement?.title }}
      </h3>
      <div class="view-meta">
        <el-tag
          v-if="announcement?.is_top"
          type="danger"
          size="small"
        >
          置顶
        </el-tag>
        <el-tag
          v-if="announcement?.is_popup"
          type="warning"
          size="small"
        >
          弹窗
        </el-tag>
        <span class="view-time">{{ formatDate(announcement?.created_at) }}</span>
      </div>
      <div class="view-body">
        {{ announcement?.content }}
      </div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import type { Announcement } from '@/api/announcement'

const visible = defineModel<boolean>({ required: true })

defineProps<{
  announcement: Announcement | null
  formatDate: (date?: string) => string
}>()
</script>

<style scoped>
.view-content { padding:10px 4px 4px; }
.view-title { font-size:20px; font-weight:700; color:#0f172a; margin:0 0 16px; line-height:1.45; }
.view-meta { display:flex; align-items:center; gap:10px; flex-wrap:wrap; margin-bottom:20px; padding-bottom:14px; border-bottom:1px solid rgba(148,163,184,.2); }
.view-time { color:#64748b; font-size:13px; }
.view-body { font-size:14px; line-height:1.8; color:#475569; white-space:pre-wrap; }
@media (max-width: 768px) {
  .view-title { font-size:18px; }
}
</style>
