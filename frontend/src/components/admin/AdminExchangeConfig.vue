<template>
  <div class="admin-exchange-config">
    <el-row :gutter="20">
      <el-col :span="12">
        <el-card
          shadow="hover"
          class="config-card"
        >
          <template #header>
            <div class="config-header">
              <span>基础配置</span>
            </div>
          </template>
          <el-form
            :model="config"
            label-width="150px"
          >
            <el-form-item label="抢兑功能开关">
              <el-switch
                :model-value="config.enabled"
                @update:model-value="updateBool('enabled', $event)"
              />
            </el-form-item>
            <el-form-item label="自动更新商品库">
              <el-switch
                :model-value="config.auto_update_products"
                @update:model-value="updateBool('auto_update_products', $event)"
              />
              <span class="form-hint">每天早上 8 点自动更新</span>
            </el-form-item>
            <el-form-item
              label="抢兑并发数"
              required
            >
              <el-input-number
                :model-value="config.concurrency"
                :min="1"
                :max="50"
                @update:model-value="updateNumber('concurrency', $event)"
              />
              <span class="form-hint">同时执行的抢兑任务数</span>
            </el-form-item>
            <el-form-item label="立即兑换功能">
              <el-switch
                :model-value="config.immediate_exchange_enabled"
                @update:model-value="updateBool('immediate_exchange_enabled', $event)"
              />
              <span class="form-hint">启用后用户可直接兑换，无需创建任务</span>
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                @click="$emit('save')"
              >
                保存配置
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>

      <el-col :span="12">
        <el-card
          shadow="hover"
          class="config-card"
        >
          <template #header>
            <div class="config-header">
              <span>兑换月卡配置</span>
              <el-tag
                v-if="config.exchange_monthly_enabled"
                type="success"
              >
                已启用
              </el-tag>
              <el-tag
                v-else
                type="info"
              >
                已禁用
              </el-tag>
            </div>
          </template>
          <el-form
            :model="config"
            label-width="150px"
          >
            <el-form-item label="兑换月卡开关">
              <el-switch
                :model-value="config.exchange_monthly_enabled"
                @update:model-value="updateBool('exchange_monthly_enabled', $event)"
              />
              <span class="form-hint">启用后自动兑换月卡</span>
            </el-form-item>
            <el-form-item label="自动兑换时间">
              <el-time-picker
                :model-value="config.exchange_time"
                format="HH:mm"
                value-format="HH:mm"
                placeholder="选择时间"
                class="wide-control"
                @update:model-value="updateString('exchange_time', $event)"
              />
            </el-form-item>
            <el-form-item label="月卡商品ID">
              <el-input
                :model-value="config.monthly_prize_id"
                placeholder="请输入月卡商品ID"
                class="wide-control"
                @update:model-value="updateString('monthly_prize_id', $event)"
              />
              <span class="sub-hint">默认1001，可从商品中心查看</span>
            </el-form-item>
            <el-form-item>
              <el-button
                type="primary"
                @click="$emit('save')"
              >
                保存配置
              </el-button>
              <el-button
                type="success"
                :loading="monthlyExchangeLoading"
                @click="$emit('executeMonthly')"
              >
                立即执行兑换
              </el-button>
            </el-form-item>
          </el-form>
        </el-card>
      </el-col>
    </el-row>

    <el-card
      shadow="hover"
      class="config-card product-card"
    >
      <template #header>
        <div class="config-header">
          <span>商品中心管理</span>
        </div>
      </template>
      <div class="product-management">
        <p class="product-tip">
          手动更新商品中心数据，需要选择一个有效的云盘账号作为数据获取源。
        </p>
        <el-form :inline="true">
          <el-form-item label="选择账号">
            <el-select
              :model-value="selectedAccountId"
              placeholder="请选择云盘账号"
              class="source-account-select"
              :disabled="sourceAccounts.length === 0"
              @update:model-value="updateSelectedAccount"
            >
              <el-option
                v-for="acc in sourceAccounts"
                :key="acc.id"
                :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
                :value="acc.id"
              />
            </el-select>
          </el-form-item>
          <el-form-item>
            <el-button
              type="primary"
              :loading="updateProductsLoading"
              @click="$emit('updateProducts')"
            >
              <el-icon><Refresh /></el-icon>
              更新商品数据
            </el-button>
          </el-form-item>
        </el-form>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { Refresh } from '@element-plus/icons-vue'
import type { Account } from '@/api/account'
import type { ExchangeConfig } from '@/api/exchange'

type BoolKey = 'enabled' | 'auto_update_products' | 'immediate_exchange_enabled' | 'exchange_monthly_enabled'
type NumberKey = 'concurrency'
type StringKey = 'exchange_time' | 'monthly_prize_id'

defineProps<{
  config: ExchangeConfig
  sourceAccounts: Account[]
  selectedAccountId: number | null
  updateProductsLoading: boolean
  monthlyExchangeLoading: boolean
}>()

const emit = defineEmits<{
  change: [patch: Partial<ExchangeConfig>]
  'update:selectedAccountId': [accountId: number | null]
  save: []
  executeMonthly: []
  updateProducts: []
}>()

const updateBool = (key: BoolKey, value: string | number | boolean) => {
  emit('change', { [key]: Boolean(value) })
}

const updateNumber = (key: NumberKey, value: number | null | undefined) => {
  emit('change', { [key]: Number(value || 1) })
}

const updateString = (key: StringKey, value: string | number | null | undefined) => {
  emit('change', { [key]: value == null ? '' : String(value) })
}

const updateSelectedAccount = (value: string | number | boolean | null | undefined) => {
  emit('update:selectedAccountId', value == null || value === '' ? null : Number(value))
}
</script>

<style scoped>
.config-card { margin-bottom: 20px; }
.product-card { margin-top: 20px; }
.config-header { display: flex; justify-content: space-between; align-items: center; }
.form-hint { margin-left: 10px; font-size: 12px; color: #999; }
.sub-hint { font-size: 12px; color: #999; }
.wide-control { width: 100%; }
.product-management { padding: 10px 0; }
.product-tip { color: #666; margin-bottom: 16px; }
.source-account-select { width: 250px; }
</style>
