<template>
  <div class="rewards-section">
    <el-empty description="领奖专区功能开发中，敬请期待...">
      <template #image>
        <el-icon
          :size="60"
          color="#909399"
        >
          <Present />
        </el-icon>
      </template>
    </el-empty>
  </div>
</template>

<script setup lang="ts">
import { Present } from '@element-plus/icons-vue'
</script>

<style scoped>
.rewards-section { padding-top:0; min-height:220px; display:flex; align-items:center; justify-content:center; }
</style>
