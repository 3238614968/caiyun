<template>
  <el-dialog
    v-model="visible"
    title="立即兑换"
    :width="isMobile ? '95%' : '480px'"
    :close-on-click-modal="false"
  >
    <el-form label-position="top">
      <el-form-item label="兑换商品">
        <el-input :model-value="selectedProduct?.prize_name" disabled />
      </el-form-item>
      <el-form-item label="选择账号" required>
        <el-select v-model="form.exchange_account_id" placeholder="请选择兑换账号" style="width: 100%;">
          <el-option
            v-for="acc in accounts"
            :key="acc.id"
            :label="acc.remark || acc.phone"
            :value="acc.id"
          />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="success" @click="$emit('submit')">立即兑换</el-button>
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
