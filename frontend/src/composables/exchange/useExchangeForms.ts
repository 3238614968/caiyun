import { computed, ref } from 'vue'
import type { Product } from '@/api/exchange'

export interface ExchangeTaskForm {
  account_id: number | null
  account_ids: number[]
  exchange_rule_id: number | null
  exchange_rule_ids: number[]
  exchange_account_id: number | null
  exchange_account_ids: number[]
  product_id: number | null
  task_type: 'fixed' | 'long_term'
  max_attempts: number
  scheduled_exchange_time: string
  restock_cycle: 'daily' | 'weekly' | 'monthly' | 'once'
  restock_weekday: number | null
  restock_day_of_month: number | null
  restock_times: string[]
  custom_cron: string
  calendar_policy: 'all' | 'workday' | 'holiday'
  holiday_dates: string[]
  workday_dates: string[]
}

export interface ExchangeRuleForm {
  account_id: number | null
  product_id: number | null
  remark: string
  exchange_time_1: string
  exchange_time_2: string
  is_active: boolean
}

export type ExchangeAccountForm = ExchangeRuleForm

export function syncCloudAccountSelection(form: ExchangeTaskForm) {
  form.account_ids = Array.from(new Set((form.account_ids || []).filter(Boolean)))
  form.account_id = form.account_ids[0] ?? form.account_id ?? null
}

export function clearExchangeRuleSelection(form: ExchangeTaskForm) {
  form.exchange_rule_id = null
  form.exchange_rule_ids = []
  form.exchange_account_id = null
  form.exchange_account_ids = []
}

export function syncExchangeRuleSelection(form: ExchangeTaskForm) {
  const mergedIds = Array.from(new Set([
    ...(form.exchange_rule_ids || []),
    ...(form.exchange_account_ids || []),
    form.exchange_rule_id || 0,
    form.exchange_account_id || 0
  ].filter(Boolean)))

  const primaryId = form.exchange_rule_id ?? form.exchange_account_id ?? mergedIds[0] ?? null
  form.exchange_rule_ids = mergedIds
  form.exchange_rule_id = primaryId
  form.exchange_account_ids = [...mergedIds]
  form.exchange_account_id = primaryId
}

function createTaskForm(): ExchangeTaskForm {
  return {
    account_id: null,
    account_ids: [],
    exchange_rule_id: null,
    exchange_rule_ids: [],
    exchange_account_id: null,
    exchange_account_ids: [],
    product_id: null,
    task_type: 'fixed',
    max_attempts: 1,
    scheduled_exchange_time: '10:00:00',
    restock_cycle: 'daily',
    restock_weekday: new Date().getDay(),
    restock_day_of_month: new Date().getDate(),
    restock_times: [],
    custom_cron: '',
    calendar_policy: 'all',
    holiday_dates: [],
    workday_dates: []
  }
}

function createAccountForm(): ExchangeRuleForm {
  return {
    account_id: null,
    product_id: null,
    remark: '',
    exchange_time_1: '10:00:00',
    exchange_time_2: '16:00:00',
    is_active: true
  }
}

export function useExchangeForms() {
  const taskDialogVisible = ref(false)
  const accountDialogVisible = ref(false)
  const immediateExchangeDialogVisible = ref(false)
  const editingAccountId = ref<number | null>(null)

  const selectedProduct = ref<Product | null>(null)
  const taskForm = ref<ExchangeTaskForm>(createTaskForm())
  const accountForm = ref<ExchangeRuleForm>(createAccountForm())

  const isEditingAccount = computed(() => editingAccountId.value !== null)

  const resetTaskForm = () => {
    taskForm.value = createTaskForm()
  }

  const resetAccountForm = () => {
    accountForm.value = createAccountForm()
  }

  return {
    taskDialogVisible,
    accountDialogVisible,
    immediateExchangeDialogVisible,
    editingAccountId,
    selectedProduct,
    taskForm,
    accountForm,
    isEditingAccount,
    resetTaskForm,
    resetAccountForm
  }
}
