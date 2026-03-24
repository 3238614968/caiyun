<template>
  <el-breadcrumb separator="/">
    <el-breadcrumb-item :to="{ path: '/' }">
      <el-icon><HomeFilled /></el-icon>
      <span class="breadcrumb-text">首页</span>
    </el-breadcrumb-item>
    <el-breadcrumb-item v-if="currentRoute.meta?.title && currentRoute.meta.title !== '首页'">
      <span class="breadcrumb-text current">{{ currentRoute.meta.title }}</span>
    </el-breadcrumb-item>
  </el-breadcrumb>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { HomeFilled } from '@element-plus/icons-vue'

const route = useRoute()

const currentRoute = computed(() => route)
</script>

<style scoped>
.el-breadcrumb {
  font-size: 14px;
}

.breadcrumb-text {
  margin-left: 6px;
}

.breadcrumb-text.current {
  color: #3b82f6;
  font-weight: 500;
}

:deep(.el-breadcrumb__inner) {
  display: flex;
  align-items: center;
  color: #666;
}

:deep(.el-breadcrumb__inner.is-link:hover) {
  color: #3b82f6;
}

:deep(.el-breadcrumb__separator) {
  color: #c0c4cc;
}
</style>
