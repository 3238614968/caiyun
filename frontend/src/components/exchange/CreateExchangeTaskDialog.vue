<template>
  <el-dialog
    v-model="visible"
    title="创建抢兑任务"
    :width="isMobile ? '92%' : '640px'"
  >
    <el-alert
      class="task-tip"
      type="info"
      :closable="false"
      show-icon
      title="选择商品与账号，添加抢兑时间即可。"
    />

    <el-form
      :model="form"
      :label-width="isMobile ? '80px' : '104px'"
    >
      <el-form-item
        label="兑换商品"
        required
      >
        <el-select
          v-model="form.product_id"
          placeholder="请选择商品"
          filterable
          clearable
          style="width: 100%;"
        >
          <el-option
            v-for="product in products"
            :key="product.id"
            :label="`${product.prize_name} (${product.p_order}云朵)`"
            :value="product.id"
          >
            <span>{{ product.prize_name }}</span>
            <span class="option-meta">{{ product.p_order }}云朵 · {{ product.category || '未分类' }}</span>
          </el-option>
        </el-select>
      </el-form-item>

      <el-form-item
        v-if="currentProduct"
        label="商品信息"
      >
        <div class="product-summary">
          <span>{{ currentProduct.prize_name }}</span>
          <el-tag
            size="small"
            type="primary"
          >
            {{ currentProduct.p_order }}云朵
          </el-tag>
          <el-tag
            size="small"
            :type="currentProduct.stock_status === 'available' ? 'success' : 'warning'"
          >
            {{ currentProduct.stock_status === 'available' ? '当前可兑' : '可预定' }}
          </el-tag>
        </div>
      </el-form-item>

      <el-form-item
        v-if="hasCloudAccounts"
        label="抢兑账号"
        required
      >
        <div class="account-select-block">
          <el-select
            v-model="form.account_ids"
            placeholder="可搜索手机号/备注，支持多选"
            filterable
            clearable
            multiple
            collapse-tags
            collapse-tags-tooltip
            reserve-keyword
            :loading="userAccountsLoading"
            style="width: 100%;"
            @change="handleCloudAccountChange"
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
          <div class="account-actions">
            <span class="selected-count">已选择 {{ selectedAccountCount }} 个账号</span>
            <el-button
              link
              type="primary"
              @click="selectAllCloudAccounts"
            >
              全选可用账号
            </el-button>
            <el-button
              link
              @click="clearSelectedAccounts"
            >
              清空
            </el-button>
          </div>
        </div>
      </el-form-item>

      <el-form-item
        v-else-if="accounts.length > 0"
        label="抢兑规则"
        required
      >
        <div class="account-select-block">
          <el-select
            v-model="form.exchange_rule_ids"
            placeholder="请选择已有抢兑规则，支持多选"
            filterable
            clearable
            multiple
            collapse-tags
            collapse-tags-tooltip
            style="width: 100%;"
            @change="handleExchangeRuleChange"
          >
            <el-option
              v-for="acc in accounts"
              :key="acc.id"
              :label="acc.remark || acc.phone"
              :value="acc.id"
              :disabled="acc.is_active === false"
            />
          </el-select>
          <div class="account-actions">
            <span class="selected-count">已选择 {{ selectedAccountCount }} 个抢兑规则</span>
            <el-button
              link
              type="primary"
              @click="selectAllExchangeRules"
            >
              全选可用规则
            </el-button>
            <el-button
              link
              @click="clearSelectedAccounts"
            >
              清空
            </el-button>
          </div>
        </div>
      </el-form-item>

      <el-alert
        v-else
        type="warning"
        :closable="false"
        title="暂无可用云盘账号，请先到账号管理页面添加账号。"
      />

      <el-form-item
        label="抢兑时间"
        required
      >
        <div class="time-select-block">
          <div class="selected-times">
            <el-tag
              v-for="time in form.restock_times"
              :key="time"
              closable
              size="large"
              @close="removeTime(time)"
            >
              {{ time.slice(0, 5) }}
            </el-tag>
            <span
              v-if="form.restock_times.length === 0"
              class="hint-text"
            >请添加至少一个时间点</span>
          </div>
          <div class="time-input-row">
            <input
              v-model="pendingTime"
              type="time"
              step="60"
              aria-label="新增抢兑时间"
              class="time-picker"
            >
            <el-button
              :disabled="!pendingTime"
              @click="addTime"
            >
              添加时间
            </el-button>
          </div>
          <span class="hint-text">支持多个时间点，每天执行，提前一分钟预热。</span>
        </div>
      </el-form-item>

      <el-form-item label="长期抢兑">
        <div class="long-term-option">
          <el-switch
            v-model="longTerm"
            aria-label="长期抢兑"
          />
          <span class="hint-text">{{ longTerm ? '每天按所选时间持续抢兑' : '只执行一次抢兑' }}</span>
        </div>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">
        取消
      </el-button>
      <el-button
        type="primary"
        :disabled="!canSubmit"
        @click="$emit('submit')"
      >
        确定
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { normalizeExchangeTime } from '@/utils/exchange-schedule'
import { clearExchangeRuleSelection, syncCloudAccountSelection, syncExchangeRuleSelection, type ExchangeTaskForm } from '@/composables/exchange/useExchangeForms'
import type { Account } from '@/api/account'
import type { ExchangeRule, Product } from '@/api/exchange'

const visible = defineModel<boolean>({ required: true })
const form = defineModel<ExchangeTaskForm>('form', { required: true })

const props = defineProps<{
  isMobile: boolean
  selectedProduct: Product | null
  accounts: ExchangeRule[]
  products: Product[]
  userAccounts: Account[]
  userAccountsLoading: boolean
}>()

defineEmits<{
  submit: []
}>()

const pendingTime = ref('16:00')
const longTerm = computed({
  get: () => form.value.task_type === 'long_term',
  set: (value: boolean) => { form.value.task_type = value ? 'long_term' : 'fixed' }
})
watch(visible, (open) => { if (open) pendingTime.value = '16:00' })

const hasCloudAccounts = computed(() => props.userAccounts.length > 0)
const currentProduct = computed(() => props.products.find(product => product.id === form.value.product_id) || props.selectedProduct)
const selectedAccountCount = computed(() => (
  hasCloudAccounts.value ? form.value.account_ids.length : form.value.exchange_rule_ids.length
))
const canSubmit = computed(() => Boolean(form.value.product_id && selectedAccountCount.value > 0 && form.value.restock_times.length > 0))

const addTime = () => {
  if (!pendingTime.value) return
  try {
    const time = normalizeExchangeTime(pendingTime.value)
    if (form.value.restock_times.includes(time)) {
      ElMessage.info('该时间已添加')
      return
    }
    if (form.value.restock_times.length >= 100) {
      ElMessage.warning('抢兑时间不能超过 100 个')
      return
    }
    form.value.restock_times = [...form.value.restock_times, time].sort()
    pendingTime.value = ''
  } catch (error) {
    ElMessage.warning((error as Error).message)
  }
}

const removeTime = (time: string) => {
  form.value.restock_times = form.value.restock_times.filter(value => value !== time)
}

const handleCloudAccountChange = () => {
  syncCloudAccountSelection(form.value)
  clearExchangeRuleSelection(form.value)
}

const handleExchangeRuleChange = () => {
  form.value.account_id = null
  form.value.account_ids = []
  form.value.exchange_rule_id = form.value.exchange_rule_ids[0] ?? null
  form.value.exchange_account_id = null
  form.value.exchange_account_ids = []
  syncExchangeRuleSelection(form.value)
}

const selectAllCloudAccounts = () => {
  form.value.account_ids = props.userAccounts.filter(acc => acc.is_active !== false).map(acc => acc.id)
  handleCloudAccountChange()
}

const selectAllExchangeRules = () => {
  form.value.exchange_rule_ids = props.accounts.filter(acc => acc.is_active !== false).map(acc => acc.id)
  form.value.exchange_rule_id = form.value.exchange_rule_ids[0] ?? null
  handleExchangeRuleChange()
}

const clearSelectedAccounts = () => {
  form.value.account_id = null
  form.value.account_ids = []
  clearExchangeRuleSelection(form.value)
}
</script>

<style scoped>
.task-tip { margin-bottom: 16px; }
.product-summary { display:flex; align-items:center; gap:8px; flex-wrap:wrap; color:#334155; }
.option-meta { float:right; color:#94a3b8; font-size:12px; margin-left:12px; }
.account-state { float:right; margin-top:3px; }
.account-select-block { width:100%; display:flex; flex-direction:column; gap:8px; }
.account-actions { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.selected-count { color:#64748b; font-size:13px; margin-right:auto; }
.hint-text { color:#64748b; font-size:13px; line-height:1.6; }
.time-select-block { width:100%; display:flex; flex-direction:column; gap:10px; }
.selected-times { display:flex; flex-wrap:wrap; gap:8px; }
.time-input-row, .long-term-option { display:flex; align-items:center; gap:12px; flex-wrap:wrap; }
.time-picker { width:160px; height:32px; box-sizing:border-box; padding:0 10px; border:1px solid #dcdfe6; border-radius:4px; background:white; color:#334155; font:inherit; outline:none; }
.time-picker:focus { border-color:#409eff; }
@media (max-width:520px) {
  .time-input-row { gap:8px; }
  .time-picker { width:124px; }
}
</style>
