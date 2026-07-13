<template>
  <div class="product-section">
    <div class="filter-area">
      <div class="filter-head">
        <div class="filter-copy">
          <span class="filter-title">商品筛选</span>
          <span class="filter-hint">当前展示 {{ products.length }} 个商品</span>
        </div>
        <el-tag
          size="small"
          effect="plain"
          type="primary"
        >
          {{ category || '全部分类' }}
        </el-tag>
      </div>

      <div class="filter-controls">
        <el-input
          v-model="keyword"
          placeholder="搜索商品名称..."
          clearable
          :prefix-icon="Search"
          class="search-input"
          @change="$emit('search')"
        />

        <div
          class="category-list"
          :class="{ 'mobile-scroll': isMobile }"
        >
          <el-radio-group
            v-model="category"
            size="small"
          >
            <el-radio-button label="">
              全部
            </el-radio-button>
            <el-radio-button
              v-for="cat in categories"
              :key="cat"
              :label="cat"
            >
              {{ cat }}
            </el-radio-button>
          </el-radio-group>
        </div>
      </div>
    </div>

    <div
      class="product-grid"
      :class="{ 'mobile-grid': isMobile }"
    >
      <el-empty
        v-if="products.length === 0"
        description="暂无商品"
      />
      <ProductCard
        v-for="product in products"
        :key="product.id"
        :product="product"
        :image-source="getProductImageSource(product)"
        :is-mobile="isMobile"
        :immediate-enabled="!!exchangeConfig?.immediate_exchange_enabled"
        @reserve="$emit('reserve', product)"
        @immediate="$emit('immediate', product)"
        @create-task="$emit('createTask', product)"
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { Search } from '@element-plus/icons-vue'
import type { ExchangeConfig } from '@/api/exchange'
import type { ProductImageSource } from '@/utils/product-image'
import ProductCard from '@/components/exchange/ProductCard.vue'

const keyword = defineModel<string>('keyword', { required: true })
const category = defineModel<string>('category', { required: true })

defineProps<{
  isMobile: boolean
  products: any[]
  categories: string[]
  exchangeConfig: ExchangeConfig | null
  getProductImageSource: (product: any) => ProductImageSource
}>()

defineEmits<{
  search: []
  reserve: [product: any]
  immediate: [product: any]
  createTask: [product: any]
}>()
</script>

<style scoped>
.product-section { display:flex; flex-direction:column; gap:18px; }
.filter-area { background: rgba(255,255,255,.72); border:1px solid rgba(255,255,255,.78); border-radius:20px; padding:18px; box-shadow:0 12px 30px rgba(37,99,235,.08); display:flex; flex-direction:column; gap:16px; }
.filter-head { display:flex; justify-content:space-between; gap:12px; align-items:center; }
.filter-copy { display:flex; flex-direction:column; gap:4px; }
.filter-title { font-size:16px; font-weight:700; color:#0f172a; }
.filter-hint { font-size:12px; color:#64748b; }
.filter-controls { display:grid; grid-template-columns:minmax(220px,320px) 1fr; gap:14px; align-items:center; }
.search-input :deep(.el-input__wrapper) { border-radius:999px; }
.category-list { min-width:0; }
.category-list :deep(.el-radio-group) { display:flex; flex-wrap:wrap; gap:8px; width:100%; align-items:center; }
.category-list :deep(.el-radio-button) { margin:0; }
.category-list :deep(.el-radio-button__inner) { border-radius:999px!important; padding:8px 16px; font-size:13px; border-left: var(--el-border)!important; box-shadow: 0 4px 12px rgba(37,99,235,.06); }
.product-grid { display:grid; grid-template-columns: repeat(auto-fill, minmax(248px,1fr)); gap:18px; }
.product-grid.mobile-grid { grid-template-columns:1fr; }
@media (max-width: 1280px) {
  .filter-controls { grid-template-columns:1fr; }
  .product-grid { grid-template-columns: repeat(auto-fill, minmax(228px,1fr)); }
}
@media (max-width: 768px) {
  .filter-area { padding:14px; gap:12px; }
  .filter-head { align-items:flex-start; flex-direction:column; }
  .category-list { overflow-x:auto; padding-bottom:4px; }
  .category-list :deep(.el-radio-group) { flex-wrap:nowrap; width:max-content; }
}
</style>
