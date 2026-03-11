<template>
  <el-form :inline="true" :model="model" class="search-form">
    <slot />
    <el-form-item>
      <el-button type="primary" @click="handleSearch">
        <el-icon><Search /></el-icon>
        搜索
      </el-button>
      <el-button @click="handleReset">
        <el-icon><Refresh /></el-icon>
        重置
      </el-button>
    </el-form-item>
  </el-form>
</template>

<script setup lang="ts">
import { Search, Refresh } from '@element-plus/icons-vue'

interface Props {
  model: Record<string, any>
}

defineProps<Props>()

const emit = defineEmits<{
  (e: 'search'): void
  (e: 'reset'): void
}>()

const handleSearch = () => {
  emit('search')
}

const handleReset = () => {
  emit('reset')
}
</script>

<style scoped>
.search-form {
  margin-bottom: 20px;
  padding: 20px;
  background: rgba(255, 255, 255, 0.8);
  border-radius: 12px;
  backdrop-filter: blur(10px);
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

:deep(.el-button--primary) {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.3);
}

:deep(.el-button--primary:hover) {
  background: linear-gradient(135deg, #2563eb 0%, #0284c7 100%);
  box-shadow: 0 6px 20px rgba(59, 130, 246, 0.4);
}
</style>
