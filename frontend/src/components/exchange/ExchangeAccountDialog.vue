<template>
  <el-dialog
    v-model="visible"
    :title="isEditingAccount ? '编辑兑换账号' : '添加兑换账号'"
    :width="width"
    :top="top"
    class="account-dialog"
    :close-on-click-modal="false"
  >
    <div class="dialog-content">
      <div class="form-section">
        <div class="section-title">
          <el-icon><User /></el-icon>
          <span>基本信息</span>
        </div>
        <el-form :model="form" label-position="top">
          <el-row :gutter="16">
            <el-col :span="isMobile ? 24 : 12">
              <el-form-item label="云盘账号" required>
                <el-select
                  v-if="!isAdmin"
                  v-model="form.account_id"
                  placeholder="请选择云盘账号"
                  style="width: 100%;"
                  :disabled="isEditingAccount"
                  :loading="userAccountsLoading"
                  :no-data-text="userAccountsLoading ? '正在加载云盘账号...' : '暂无云盘账号，请先到账号页面添加'"
                  :size="controlSize"
                >
                  <el-option
                    v-for="acc in userAccounts"
                    :key="acc.id"
                    :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
                    :value="acc.id"
                  />
                </el-select>
                <el-select
                  v-else
                  v-model="form.account_id"
                  placeholder="搜索并选择云盘账号"
                  style="width: 100%;"
                  :disabled="isEditingAccount"
                  :size="controlSize"
                  filterable
                  remote
                  :remote-method="(keyword: string) => $emit('searchAccounts', keyword)"
                  :loading="accountSearchLoading"
                >
                  <el-option
                    v-for="acc in allAccountsSearchResults"
                    :key="acc.id"
                    :label="acc.remark ? `${acc.remark} (${acc.phone}) [${acc.username}]` : `${acc.phone} [${acc.username}]`"
                    :value="acc.id"
                  />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="isMobile ? 24 : 12">
              <el-form-item label="选择商品" required>
                <el-select
                  v-model="form.product_id"
                  placeholder="选择要兑换的商品"
                  style="width: 100%;"
                  filterable
                  :size="controlSize"
                >
                  <el-option
                    v-for="product in products"
                    :key="product.id"
                    :label="`${product.prize_name} (${product.p_order}云朵)`"
                    :value="product.id"
                  />
                </el-select>
              </el-form-item>
            </el-col>
          </el-row>
          <el-form-item label="备注">
            <el-input
              v-model="form.remark"
              placeholder="给这个账号添加备注（可选）"
              :size="controlSize"
            />
          </el-form-item>
        </el-form>
      </div>

      <div class="form-section">
        <div class="section-title">
          <el-icon><Timer /></el-icon>
          <span>抢兑时间设置</span>
        </div>
        <el-form :model="form" label-position="top">
          <el-row :gutter="16">
            <el-col :span="isMobile ? 24 : 12">
              <el-form-item label="第一次抢兑" required>
                <el-time-picker
                  v-model="form.exchange_time_1"
                  format="HH:mm"
                  value-format="HH:mm:ss"
                  placeholder="选择时间"
                  style="width: 100%;"
                  :size="controlSize"
                />
              </el-form-item>
            </el-col>
            <el-col :span="isMobile ? 24 : 12">
              <el-form-item label="第二次抢兑" required>
                <el-time-picker
                  v-model="form.exchange_time_2"
                  format="HH:mm"
                  value-format="HH:mm:ss"
                  placeholder="选择时间"
                  style="width: 100%;"
                  :size="controlSize"
                />
              </el-form-item>
            </el-col>
          </el-row>
        </el-form>
      </div>

      <div v-if="isEditingAccount" class="form-section">
        <div class="section-title">
          <el-icon><Setting /></el-icon>
          <span>账号状态</span>
        </div>
        <el-form :model="form" label-position="top">
          <el-form-item>
            <el-switch
              v-model="form.is_active"
              active-text="启用抢兑"
              inactive-text="暂停抢兑"
              :size="controlSize"
            />
          </el-form-item>
        </el-form>
      </div>
    </div>
    <template #footer>
      <div class="dialog-footer">
        <el-button :size="controlSize" @click="visible = false">取消</el-button>
        <el-button type="primary" :size="controlSize" @click="$emit('submit')">确定</el-button>
      </div>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { Setting, Timer, User } from '@element-plus/icons-vue'
import type { AccountSearchItem } from '@/api/account'
import type { ExchangeAccountForm } from '@/composables/exchange/useExchangeForms'

const visible = defineModel<boolean>({ required: true })
const form = defineModel<ExchangeAccountForm>('form', { required: true })

defineProps<{
  isMobile: boolean
  isAdmin: boolean
  isEditingAccount: boolean
  width: string
  top: string
  controlSize: 'large' | 'default' | 'small'
  userAccounts: any[]
  userAccountsLoading: boolean
  allAccountsSearchResults: AccountSearchItem[]
  accountSearchLoading: boolean
  products: any[]
}>()

defineEmits<{
  searchAccounts: [keyword: string]
  submit: []
}>()
</script>

<style scoped>
.dialog-content { max-height: min(70vh, 720px); overflow-y: auto; padding: 4px; }
.form-section { background: rgba(248,250,252,.85); border: 1px solid rgba(226,232,240,.9); border-radius: 18px; padding: 16px; margin-bottom: 14px; }
.section-title { display: flex; align-items: center; gap: 8px; margin-bottom: 14px; font-size: 15px; font-weight: 700; color: #0f172a; }
.section-title .el-icon { color: #2563eb; }
.dialog-footer { display: flex; justify-content: flex-end; gap: 10px; }
@media (max-width: 768px) {
  .dialog-content { max-height: 76vh; padding: 0; }
  .form-section { padding: 14px; border-radius: 16px; }
  .dialog-footer { flex-direction: column-reverse; }
  .dialog-footer :deep(.el-button) { width: 100%; margin-left: 0; }
}
</style>
