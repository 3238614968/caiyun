<template>
  <el-form
    :inline="true"
    :model="model"
    class="search-form"
    @submit.prevent="handleSearch"
  >
    <slot />
    <el-form-item>
      <el-button
        type="primary"
        native-type="submit"
      >
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

</style>
