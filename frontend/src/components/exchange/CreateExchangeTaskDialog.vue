<template>
  <el-dialog v-model="visible" title="创建抢兑任务" :width="isMobile ? '90%' : '500px'">
    <el-form :model="form" :label-width="isMobile ? '100px' : '120px'">
      <el-form-item label="商品名称">
        <span>{{ selectedProduct?.prize_name }}</span>
      </el-form-item>
      <el-form-item label="云朵价格">
        <span>{{ selectedProduct?.p_order }}云朵</span>
      </el-form-item>
      <el-form-item label="兑换账号" required>
        <el-select v-model="form.exchange_account_id" placeholder="请选择兑换账号" style="width: 100%;">
          <el-option
            v-for="acc in accounts"
            :key="acc.id"
            :label="acc.remark || acc.phone"
            :value="acc.id"
          />
        </el-select>
      </el-form-item>
      <el-form-item label="任务类型" required>
        <el-radio-group v-model="form.task_type">
          <el-radio label="fixed">固定次数</el-radio>
          <el-radio label="long_term">长期抢兑</el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item v-if="form.task_type === 'fixed'" label="最大次数">
        <el-input-number v-model="form.max_attempts" :min="1" :max="100" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" @click="$emit('submit')">确定</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import type { ExchangeTaskForm } from '@/composables/exchange/useExchangeForms'

const visible = defineModel<boolean>({ required: true })
const form = defineModel<ExchangeTaskForm>('form', { required: true })

defineProps<{
  isMobile: boolean
  selectedProduct: any | null
  accounts: any[]
}>()

defineEmits<{
  submit: []
}>()
</script>
