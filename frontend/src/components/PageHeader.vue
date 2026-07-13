<template>
  <div class="page-header">
    <div class="header-left">
      <h2 class="page-title">
        {{ title }}
      </h2>
      <p
        v-if="subtitle"
        class="page-subtitle"
      >
        {{ subtitle }}
      </p>
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
  align-items: flex-start;
  gap: 16px;
  flex-wrap: wrap;
  margin-bottom: 24px;
  padding: clamp(18px, 2vw, 24px);
  background: rgba(255, 255, 255, 0.84);
  backdrop-filter: blur(14px);
  border-radius: 22px;
  box-shadow: 0 14px 36px rgba(37, 99, 235, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.72);
}

.header-left {
  min-width: 0;
}

.page-title {
  margin: 0;
  font-size: clamp(22px, 3vw, 28px);
  font-weight: 700;
  letter-spacing: -0.02em;
  color: #1e293b;
  background: linear-gradient(135deg, #2563eb 0%, #0ea5e9 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.page-subtitle {
  margin: 6px 0 0;
  font-size: 14px;
  line-height: 1.6;
  color: #64748b;
  max-width: 640px;
}

.header-right {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .page-header {
    margin-bottom: 16px;
    padding: 16px;
    border-radius: 18px;
  }

  .page-title {
    font-size: 21px;
  }

  .page-subtitle {
    font-size: 13px;
  }

  .header-right {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
