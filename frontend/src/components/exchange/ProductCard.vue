<template>
  <el-card
    shadow="hover"
    class="product-card"
    :class="{ 'sold-out': isSoldOut, mobile: isMobile }"
  >
    <div class="product-layout" :class="{ mobile: isMobile }">
      <div class="product-image" :class="{ mobile: isMobile, 'has-image': imageUrl }">
        <img
          v-if="imageUrl"
          :src="imageUrl"
          :alt="product.prize_name"
          class="product-img"
          @error="handleProductImageError"
        />
        <div class="product-image-fallback">
          <el-icon :size="isMobile ? 36 : 48" color="#409EFF"><Present /></el-icon>
        </div>
      </div>

      <div class="product-content" :class="{ mobile: isMobile }">
        <div class="product-title">{{ product.prize_name }}</div>
        <div class="product-meta" :class="{ mobile: isMobile }">
          <el-tag size="small" effect="plain" type="info">{{ product.category }}</el-tag>
          <span class="product-price">
            <svg class="cloud-icon" viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
              <path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z"/>
            </svg>
            <span class="price-value">{{ product.p_order }}</span>
          </span>
        </div>

        <div class="product-stock">
          <div class="stock-header">
            <span class="stock-label">
              <el-icon><Box /></el-icon>
              库存
            </span>
            <span class="stock-value" :class="{ low: product.daily_remainder_count <= 10, empty: product.daily_remainder_count === 0 }">
              {{ product.daily_remainder_count }}/{{ stockLimit }}
            </span>
          </div>
          <el-progress
            :percentage="stockPercentage"
            :status="stockStatus"
            :stroke-width="isMobile ? 6 : 10"
            :show-text="false"
            class="stock-progress"
          />
        </div>

        <el-button
          :type="actionType"
          class="exchange-btn"
          :size="isMobile ? 'small' : 'default'"
          @click="handleAction"
        >
          {{ actionText }}
        </el-button>
      </div>
    </div>
  </el-card>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { Box, Present } from '@element-plus/icons-vue'
import type { Product } from '@/api/exchange'

type ProductWithDailyCount = Product & { daily_count?: number }

const props = defineProps<{
  product: ProductWithDailyCount
  imageUrl?: string
  isMobile: boolean
  immediateEnabled: boolean
}>()

const emit = defineEmits<{
  reserve: [product: ProductWithDailyCount]
  immediate: [product: ProductWithDailyCount]
  createTask: [product: ProductWithDailyCount]
}>()

const isSoldOut = computed(() => props.product.stock_status === 'sold_out' || props.product.daily_remainder_count === 0)
const stockLimit = computed(() => props.product.daily_count || props.product.daily_limit_count || '-')
const stockPercentage = computed(() => {
  const limit = props.product.daily_limit_count || 0
  if (limit <= 0) {
    return props.product.daily_remainder_count > 0 ? 50 : 0
  }
  return Math.round((props.product.daily_remainder_count / limit) * 100)
})
const stockStatus = computed(() => {
  if (isSoldOut.value) return 'exception'
  if (stockPercentage.value <= 20) return 'exception'
  if (stockPercentage.value <= 50) return 'warning'
  return 'success'
})
const actionType = computed(() => {
  if (isSoldOut.value) return 'warning'
  return props.immediateEnabled ? 'success' : 'primary'
})
const actionText = computed(() => {
  if (isSoldOut.value) return '预定'
  return props.immediateEnabled ? '立即兑换' : '立即抢兑'
})

const handleProductImageError = (event: Event) => {
  const image = event.target as HTMLImageElement | null
  if (image) {
    image.style.display = 'none'
  }
}

const handleAction = () => {
  if (isSoldOut.value) {
    emit('reserve', props.product)
    return
  }
  if (props.immediateEnabled) {
    emit('immediate', props.product)
    return
  }
  emit('createTask', props.product)
}
</script>

<style scoped>
.product-card {
  border-radius: 22px;
  overflow: hidden;
  border: 1px solid rgba(255, 255, 255, .78);
  background: rgba(255, 255, 255, .84);
  box-shadow: 0 16px 32px rgba(37, 99, 235, .09);
  backdrop-filter: blur(12px);
  transition: transform .2s ease, box-shadow .2s ease, border-color .2s ease;
}

.product-card :deep(.el-card__body) {
  padding: 14px;
  height: 100%;
}

.product-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 20px 38px rgba(37, 99, 235, .14);
  border-color: rgba(59, 130, 246, .32);
}

.product-card.sold-out {
  opacity: .76;
}

.product-card.sold-out .product-image {
  filter: grayscale(100%);
}

.product-layout {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.product-layout.mobile {
  flex-direction: row;
  align-items: stretch;
  gap: 12px;
}

.product-image {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 122px;
  margin: -14px -14px 14px;
  overflow: hidden;
  border-radius: 22px 22px 0 0;
  background: linear-gradient(135deg, rgba(224, 242, 254, .72) 0%, rgba(239, 246, 255, .5) 100%);
  border-bottom: 1px solid rgba(255, 255, 255, .72);
}

.product-image.mobile {
  width: 88px;
  height: 88px;
  margin: 0;
  border-radius: 16px;
  flex-shrink: 0;
  border-bottom: none;
}

.product-img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  padding: 14px;
  transition: transform .2s ease;
  position: relative;
  z-index: 1;
}

.product-card:hover .product-img {
  transform: scale(1.05);
}

.product-image-fallback {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, rgba(224, 242, 254, .52), rgba(186, 230, 253, .3));
  z-index: 0;
}

.product-image.has-image .product-image-fallback {
  z-index: -1;
}

.product-content {
  display: flex;
  flex-direction: column;
  gap: 10px;
  height: 100%;
}

.product-content.mobile {
  gap: 8px;
}

.product-title {
  font-size: 15px;
  font-weight: 700;
  color: #1e3a8a;
  line-height: 1.45;
  min-height: 44px;
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.product-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.product-price {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  margin-left: auto;
  font-size: 14px;
  font-weight: 700;
  color: #2563eb;
  background: rgba(239, 246, 255, .92);
  padding: 5px 10px;
  border-radius: 999px;
  border: 1px solid rgba(191, 219, 254, .9);
}

.cloud-icon {
  width: 16px;
  height: 16px;
  color: #2563eb;
}

.product-stock {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: auto;
  padding: 10px 12px;
  border-radius: 14px;
  background: rgba(248, 250, 252, .84);
  border: 1px solid rgba(226, 232, 240, .8);
}

.stock-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
}

.stock-label {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #64748b;
}

.stock-value {
  font-size: 13px;
  font-weight: 700;
  color: #059669;
}

.stock-value.low {
  color: #d97706;
}

.stock-value.empty {
  color: #dc2626;
}

.stock-progress {
  border-radius: 999px;
  overflow: hidden;
}

.exchange-btn {
  width: 100%;
  min-height: 40px;
  border-radius: 14px;
  font-weight: 700;
}

@media (max-width: 768px) {
  .product-layout.mobile {
    flex-direction: row;
  }

  .product-image.mobile {
    width: 84px;
    height: 84px;
  }

  .product-title {
    min-height: auto;
    font-size: 14px;
  }
}
</style>
