<template>
  <div class="not-found-container">
    <div class="not-found-content">
      <h1 class="error-code">
        404
      </h1>
      <h2 class="error-title">
        页面未找到
      </h2>
      <p class="error-description">
        抱歉，您访问的页面不存在或已被移除。
      </p>
      <button
        class="back-home-btn"
        @click="goHome"
      >
        返回首页
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from 'vue-router'

const router = useRouter()

function goHome() {
  router.push('/')
}
</script>

<style scoped>
.not-found-container {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 100vh;
  background-color: #f5f7fa;
}

.not-found-content {
  text-align: center;
  padding: 2rem;
}

.error-code {
  font-size: 6rem;
  font-weight: 700;
  color: #1890ff;
  margin: 0;
  line-height: 1;
}

.error-title {
  font-size: 1.5rem;
  color: #333;
  margin: 1rem 0 0.5rem;
}

.error-description {
  font-size: 1rem;
  color: #666;
  margin: 0 0 2rem;
}

.back-home-btn {
  padding: 0.6rem 2rem;
  font-size: 1rem;
  color: #fff;
  background-color: #1890ff;
  border: none;
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.2s;
}

.back-home-btn:hover {
  background-color: #40a9ff;
}
</style>
