<template>
  <el-dialog
    v-model="visible"
    title="账号详情"
    width="600px"
  >
    <el-descriptions
      :column="2"
      border
    >
      <el-descriptions-item label="手机号">
        {{ account.phone }}
      </el-descriptions-item>
      <el-descriptions-item label="云朵数">
        {{ account.cloud_count }}
      </el-descriptions-item>
      <el-descriptions-item label="平台">
        {{ account.platform }}
      </el-descriptions-item>
      <el-descriptions-item label="状态">
        <el-tag :type="account.is_active ? 'success' : 'danger'">
          {{ account.is_active ? '激活' : '停用' }}
        </el-tag>
      </el-descriptions-item>
      <el-descriptions-item label="过期时间">
        {{ formatExpireTime(account.expire_at) }}
      </el-descriptions-item>
      <el-descriptions-item label="备注">
        {{ account.remark || '-' }}
      </el-descriptions-item>
      <el-descriptions-item
        label="创建时间"
        :span="2"
      >
        {{ formatDate(account.created_at) }}
      </el-descriptions-item>
    </el-descriptions>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { Account } from '@/api/account'

const props = defineProps<{
  modelValue: boolean
  account: Account
  formatDate: (date: string) => string
  formatExpireTime: (expireAt: number) => string
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (value: boolean) => emit('update:modelValue', value)
})
</script>
