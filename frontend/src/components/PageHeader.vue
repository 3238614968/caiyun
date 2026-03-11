<template>
  <div class="page-header">
    <div class="header-left">
      <h2 class="page-title">{{ title }}</h2>
      <p v-if="subtitle" class="page-subtitle">{{ subtitle }}</p>
    </div>
    <div class="header-right">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
interface Props {
  title: string
  subtitle?: string
}

defineProps<Props>()
</script>

<style scoped>
.page-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
  padding: 20px 24px;
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(10px);
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
}

.page-title {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
  color: #1e293b;
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.page-subtitle {
  margin: 4px 0 0;
  font-size: 14px;
  color: #64748b;
}

.header-right {
  display: flex;
  gap: 12px;
}
</style>
