<template>
  <div class="page-header">
    <div class="header-main">
      <div class="header-title-section">
        <el-icon :size="24" color="#2563eb"><Present /></el-icon>
        <div class="header-text">
          <span class="title">兑换中心</span>
          <span class="subtitle">使用云朵兑换心仪商品</span>
        </div>
      </div>
      <div class="header-actions">
        <el-button v-if="activeTab === 'accounts'" type="primary" :icon="Plus" @click="$emit('addAccount')">
          添加兑换账号
        </el-button>
      </div>
    </div>

    <div class="stats-row">
      <div class="stat-item">
        <div class="stat-icon-bg blue">
          <el-icon><User /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-num">{{ accountsCount }}</span>
          <span class="stat-name">兑换账号</span>
        </div>
      </div>
      <div class="stat-item">
        <div class="stat-icon-bg indigo">
          <el-icon><Timer /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-num">{{ tasksCount }}</span>
          <span class="stat-name">抢兑任务</span>
        </div>
      </div>
      <div class="stat-item">
        <div class="stat-icon-bg success">
          <el-icon><CircleCheck /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-num">{{ totalSuccess }}</span>
          <span class="stat-name">成功</span>
        </div>
      </div>
      <div class="stat-item">
        <div class="stat-icon-bg danger">
          <el-icon><CircleClose /></el-icon>
        </div>
        <div class="stat-content">
          <span class="stat-num">{{ totalFail }}</span>
          <span class="stat-name">失败</span>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { CircleCheck, CircleClose, Plus, Present, Timer, User } from '@element-plus/icons-vue'

defineProps<{
  activeTab: string
  accountsCount: number
  tasksCount: number
  totalSuccess: number
  totalFail: number
}>()

defineEmits<{
  addAccount: []
}>()
</script>

<style scoped>
.page-header {
  background: linear-gradient(135deg, rgba(255, 255, 255, .92), rgba(239, 246, 255, .86));
  border-radius: 28px;
  padding: 24px;
  margin-bottom: 20px;
  box-shadow: 0 18px 40px rgba(37, 99, 235, .08);
  border: 1px solid rgba(255, 255, 255, .78);
  backdrop-filter: blur(14px);
}

.header-main {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  margin-bottom: 20px;
  flex-wrap: wrap;
}

.header-title-section {
  display: flex;
  align-items: center;
  gap: 12px;
}

.header-text {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.title {
  font-size: 28px;
  font-weight: 800;
  color: #1e3a8a;
  line-height: 1;
}

.subtitle {
  font-size: 14px;
  color: #64748b;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.stats-row {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.stat-item {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 16px;
  border-radius: 20px;
  background: rgba(255, 255, 255, .72);
  border: 1px solid rgba(255, 255, 255, .82);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, .72);
}

.stat-icon-bg {
  width: 48px;
  height: 48px;
  border-radius: 16px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  flex-shrink: 0;
}

.stat-icon-bg.blue {
  background: linear-gradient(135deg, #3b82f6, #2563eb);
}

.stat-icon-bg.indigo {
  background: linear-gradient(135deg, #6366f1, #4f46e5);
}

.stat-icon-bg.success {
  background: linear-gradient(135deg, #10b981, #059669);
}

.stat-icon-bg.danger {
  background: linear-gradient(135deg, #f43f5e, #dc2626);
}

.stat-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.stat-num {
  font-size: 24px;
  font-weight: 800;
  color: #0f172a;
}

.stat-name {
  font-size: 13px;
  color: #64748b;
  font-weight: 600;
}

@media (max-width: 768px) {
  .page-header {
    border-radius: 20px;
    padding: 16px;
  }

  .header-main {
    margin-bottom: 14px;
  }

  .header-actions {
    width: 100%;
  }

  .header-actions :deep(.el-button) {
    width: 100%;
  }

  .stats-row {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .stat-item {
    padding: 14px;
    border-radius: 16px;
  }

  .stat-icon-bg {
    width: 42px;
    height: 42px;
  }
}

@media (max-width: 520px) {
  .stats-row {
    grid-template-columns: 1fr;
  }
}
</style>
