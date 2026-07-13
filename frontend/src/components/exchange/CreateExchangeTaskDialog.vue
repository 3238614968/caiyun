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
      title="可直接选择一个或多个云盘账号创建抢兑任务，系统会自动复用或生成抢兑规则。"
    />

    <el-form
      :model="form"
      :label-width="isMobile ? '96px' : '128px'"
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
        label="任务类型"
        required
      >
        <el-radio-group v-model="form.task_type">
          <el-radio label="fixed">
            单次/固定次数
          </el-radio>
          <el-radio label="long_term">
            长期抢兑
          </el-radio>
        </el-radio-group>
      </el-form-item>
      <el-form-item
        v-if="form.task_type === 'fixed'"
        label="最大次数"
      >
        <el-input-number
          v-model="form.max_attempts"
          :min="1"
          :max="100"
        />
      </el-form-item>
      <el-form-item
        v-else
        label="长期策略"
      >
        <span class="hint-text">长期任务会持续尝试，适合售罄商品预定和周期性补货商品。</span>
      </el-form-item>

      <el-form-item label="指定抢兑时间">
        <el-time-picker
          v-model="form.scheduled_exchange_time"
          value-format="HH:mm:ss"
          format="HH:mm"
          placeholder="选择抢兑时间"
          style="width: 180px;"
        />
        <span class="inline-hint">为空时使用抢兑规则时间；配置多个时间点或 cron 时优先生效</span>
      </el-form-item>

      <el-form-item
        v-if="form.task_type === 'long_term'"
        label="补货周期"
      >
        <div class="cycle-row">
          <el-select
            v-model="form.restock_cycle"
            style="width: 160px;"
          >
            <el-option
              label="每日"
              value="daily"
            />
            <el-option
              label="每周"
              value="weekly"
            />
            <el-option
              label="每月"
              value="monthly"
            />
            <el-option
              label="仅一次"
              value="once"
            />
          </el-select>
          <el-select
            v-if="form.restock_cycle === 'weekly'"
            v-model="form.restock_weekday"
            style="width: 140px;"
            placeholder="选择星期"
          >
            <el-option
              v-for="day in weekdayOptions"
              :key="day.value"
              :label="day.label"
              :value="day.value"
            />
          </el-select>
          <el-input-number
            v-if="form.restock_cycle === 'monthly'"
            v-model="form.restock_day_of_month"
            :min="1"
            :max="31"
            controls-position="right"
          />
        </div>
      </el-form-item>

      <el-form-item
        v-if="form.task_type === 'long_term'"
        label="多个时间点"
      >
        <el-select
          v-model="form.restock_times"
          multiple
          filterable
          allow-create
          default-first-option
          placeholder="输入 HH:mm 后回车，例如 10:00、16:00"
          style="width: 100%;"
        />
      </el-form-item>

      <el-form-item
        v-if="form.task_type === 'long_term'"
        label="自定义 Cron"
      >
        <el-input
          v-model="form.custom_cron"
          placeholder="可选，例如 30 10 * * 5；填写后优先于时间点"
          clearable
        />
      </el-form-item>

      <el-form-item
        v-if="form.task_type === 'long_term'"
        label="日历策略"
      >
        <div class="cycle-row">
          <el-select
            v-model="form.calendar_policy"
            style="width: 180px;"
          >
            <el-option
              label="每天执行"
              value="all"
            />
            <el-option
              label="仅工作日"
              value="workday"
            />
            <el-option
              label="仅节假日/周末"
              value="holiday"
            />
          </el-select>
          <span class="inline-hint">节假日默认按周末识别，可用下方日期覆盖</span>
        </div>
      </el-form-item>

      <el-form-item
        v-if="form.task_type === 'long_term'"
        label="节假日覆盖"
      >
        <el-select
          v-model="form.holiday_dates"
          multiple
          filterable
          allow-create
          default-first-option
          placeholder="输入 YYYY-MM-DD 后回车"
          style="width: 100%;"
        />
      </el-form-item>

      <el-form-item
        v-if="form.task_type === 'long_term'"
        label="调休工作日"
      >
        <el-select
          v-model="form.workday_dates"
          multiple
          filterable
          allow-create
          default-first-option
          placeholder="输入 YYYY-MM-DD 后回车"
          style="width: 100%;"
        />
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
import { computed } from 'vue'
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

const weekdayOptions = [
  { label: '周日', value: 0 },
  { label: '周一', value: 1 },
  { label: '周二', value: 2 },
  { label: '周三', value: 3 },
  { label: '周四', value: 4 },
  { label: '周五', value: 5 },
  { label: '周六', value: 6 }
]

const hasCloudAccounts = computed(() => props.userAccounts.length > 0)
const currentProduct = computed(() => props.products.find(product => product.id === form.value.product_id) || props.selectedProduct)
const selectedAccountCount = computed(() => (
  hasCloudAccounts.value ? form.value.account_ids.length : form.value.exchange_rule_ids.length
))
const canSubmit = computed(() => Boolean(form.value.product_id && selectedAccountCount.value > 0))

const handleCloudAccountChange = () => {
  syncCloudAccountSelection(form.value)
  clearExchangeRuleSelection(form.value)
}

const handleExchangeRuleChange = () => {
  form.value.account_id = null
  form.value.account_ids = []
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
.hint-text, .inline-hint { color:#64748b; font-size:13px; line-height:1.6; }
.inline-hint { margin-left:10px; }
.cycle-row { display:flex; align-items:center; gap:10px; flex-wrap:wrap; }
</style>
