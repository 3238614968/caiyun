<template>
  <el-dialog
    v-model="visible"
    title="立即兑换"
    :width="isMobile ? '95%' : '480px'"
    :close-on-click-modal="false"
  >
    <el-form label-position="top">
      <el-form-item label="兑换商品">
        <el-input
          :model-value="selectedProduct?.prize_name"
          disabled
        />
      </el-form-item>
      <el-form-item
        v-if="userAccounts.length > 0"
        label="选择抢兑账号"
        required
      >
        <el-select
          v-model="form.account_id"
          placeholder="搜索手机号/备注选择云盘账号"
          filterable
          clearable
          reserve-keyword
          :loading="userAccountsLoading"
          style="width: 100%;"
          @change="form.exchange_rule_id = null; form.exchange_account_id = null"
        >
          <el-option
            v-for="acc in userAccounts"
            :key="acc.id"
            :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
            :value="acc.id"
            :disabled="acc.is_active === false"
          >
            <span>{{ acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone }}</span>
            <el-tag
              size="small"
              :type="acc.is_active === false ? 'danger' : 'success'"
              class="account-state"
            >
              {{ acc.is_active === false ? '停用' : '可用' }}
            </el-tag>
          </el-option>
        </el-select>
      </el-form-item>
      <el-form-item
        v-else
        label="选择抢兑规则"
        required
      >
        <el-select
          v-model="form.exchange_rule_id"
          placeholder="请选择已有抢兑规则"
          filterable
          clearable
          style="width: 100%;"
          @change="form.account_id = null; form.account_ids = []; form.exchange_account_id = form.exchange_rule_id"
        >
          <el-option
            v-for="acc in accounts"
            :key="acc.id"
            :label="acc.remark || acc.phone"
            :value="acc.id"
            :disabled="acc.is_active === false"
          />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">
        取消
      </el-button>
      <el-button
        type="success"
        :disabled="!canSubmit"
        @click="$emit('submit')"
      >
        立即兑换
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import type { ExchangeTaskForm } from '@/composables/exchange/useExchangeForms'
import type { Account } from '@/api/account'
import type { ExchangeRule, Product } from '@/api/exchange'

const visible = defineModel<boolean>({ required: true })
const form = defineModel<ExchangeTaskForm>('form', { required: true })

defineProps<{
  isMobile: boolean
  selectedProduct: Product | null
  accounts: ExchangeRule[]
  userAccounts: Account[]
  userAccountsLoading: boolean
}>()

defineEmits<{
  submit: []
}>()

const canSubmit = computed(() => Boolean(form.value.account_id || form.value.exchange_rule_id || form.value.exchange_account_id))
</script>

<style scoped>
.account-state { float:right; margin-top:3px; }
</style>
