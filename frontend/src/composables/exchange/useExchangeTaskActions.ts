import { computed, ref, type Ref } from 'vue'
import { ElMessage } from 'element-plus'
import {
  createExchangeTask,
  immediateExchange,
  type CreateExchangeTaskItemResult,
  type CreateExchangeTaskRequest,
  type ExchangeRule,
  type Product
} from '@/api/exchange'
import type { Account } from '@/api/account'
import { operationQueuedMessage } from '@/api/operation'
import {
  clearExchangeRuleSelection,
  syncExchangeRuleSelection,
  type ExchangeTaskForm
} from '@/composables/exchange/useExchangeForms'
import {
  buildCreateTaskResultDisplay,
  buildRetryCreateTaskPayload,
  type BatchCreateTaskResultDisplay
} from '@/utils/exchange-task-results'

export type CreateTaskOptions = {
  showSuccess?: boolean
  payload?: CreateExchangeTaskRequest | null
  forceResultDialog?: boolean
  successMessage?: string
}

interface UseExchangeTaskActionsOptions {
  isAdmin: Ref<boolean>
  products: Ref<Product[]>
  accounts: Ref<ExchangeRule[]>
  userAccounts: Ref<Account[]>
  selectedProduct: Ref<Product | null>
  taskForm: Ref<ExchangeTaskForm>
  resetTaskForm: () => void
  loadProducts: () => Promise<void>
  loadAccounts: () => Promise<void>
  loadTasks: () => Promise<void>
  loadUserAccounts: (force?: boolean) => Promise<void>
  syncDefaultProductSelection: () => void
}

export function useExchangeTaskActions(options: UseExchangeTaskActionsOptions) {
  const batchTaskResultDialogVisible = ref(false)
  const batchTaskResultDisplay = ref<BatchCreateTaskResultDisplay | null>(null)
  const lastCreateTaskPayload = ref<CreateExchangeTaskRequest | null>(null)
  const lastCreateTaskResults = ref<CreateExchangeTaskItemResult[]>([])
  const retryFailedCreateTaskLoading = ref(false)

  const selectableCloudAccounts = computed(() => options.userAccounts.value.filter((acc) => acc.is_active !== false))
  const selectableExchangeAccounts = computed(() => options.accounts.value.filter((acc) => acc.is_active !== false))

  const hasRetryableCreateTaskFailures = computed(() => Boolean(
    buildRetryCreateTaskPayload(lastCreateTaskPayload.value, lastCreateTaskResults.value)
  ))

  const resetBatchTaskResultState = () => {
    batchTaskResultDialogVisible.value = false
    batchTaskResultDisplay.value = null
    lastCreateTaskPayload.value = null
    lastCreateTaskResults.value = []
  }

  const applyDefaultTaskAccount = () => {
    const cloudAccount = selectableCloudAccounts.value[0]
    if (cloudAccount) {
      options.taskForm.value.account_id = cloudAccount.id
      options.taskForm.value.account_ids = [cloudAccount.id]
      clearExchangeRuleSelection(options.taskForm.value)
      return true
    }

    const exchangeAccount = selectableExchangeAccounts.value[0]
    if (exchangeAccount) {
      options.taskForm.value.account_id = null
      options.taskForm.value.account_ids = []
      options.taskForm.value.exchange_rule_id = exchangeAccount.id
      options.taskForm.value.exchange_rule_ids = [exchangeAccount.id]
      syncExchangeRuleSelection(options.taskForm.value)
      return true
    }

    options.taskForm.value.account_id = null
    options.taskForm.value.account_ids = []
    clearExchangeRuleSelection(options.taskForm.value)
    return false
  }

  const getTaskAccountCount = () => (
    selectableCloudAccounts.value.length > 0
      ? selectableCloudAccounts.value.length
      : selectableExchangeAccounts.value.length
  )

  const prepareTaskForm = async (product?: Product | null, taskType: 'fixed' | 'long_term' = 'fixed', maxAttempts = 1) => {
    options.resetTaskForm()

    if (options.products.value.length === 0) {
      await options.loadProducts()
    } else {
      options.syncDefaultProductSelection()
    }

    if (options.products.value.length === 0) {
      ElMessage.warning('暂无可用商品，请先刷新商品列表')
      return false
    }

    options.selectedProduct.value = product || null
    const targetProduct = product || options.products.value[0]
    options.taskForm.value.product_id = targetProduct?.id ?? null
    options.taskForm.value.task_type = taskType
    options.taskForm.value.max_attempts = maxAttempts

    if (!options.isAdmin.value) {
      await options.loadUserAccounts(true)
    }

    if (!applyDefaultTaskAccount()) {
      ElMessage.warning('暂无可用云盘账号，请先到账号页面添加并启用账号')
      return false
    }

    return true
  }

  const buildTaskPayload = (allowBatch = true): CreateExchangeTaskRequest | null => {
    const productId = Number(options.taskForm.value.product_id || 0)
    if (!productId) {
      ElMessage.warning('请选择商品')
      return null
    }

    syncExchangeRuleSelection(options.taskForm.value)
    options.taskForm.value.account_ids = Array.from(new Set((options.taskForm.value.account_ids || []).filter(Boolean)))
    options.taskForm.value.account_id = options.taskForm.value.account_ids[0] ?? options.taskForm.value.account_id ?? null

    const payload: CreateExchangeTaskRequest = {
      product_id: productId,
      task_type: options.taskForm.value.task_type,
      max_attempts: options.taskForm.value.task_type === 'long_term'
        ? Math.max(Number(options.taskForm.value.max_attempts) || 10, 10)
        : Math.max(Number(options.taskForm.value.max_attempts) || 1, 1),
      scheduled_exchange_time: options.taskForm.value.scheduled_exchange_time || undefined,
      restock_cycle: options.taskForm.value.task_type === 'long_term' ? options.taskForm.value.restock_cycle : undefined,
      restock_weekday: options.taskForm.value.task_type === 'long_term' && options.taskForm.value.restock_cycle === 'weekly' ? options.taskForm.value.restock_weekday : undefined,
      restock_day_of_month: options.taskForm.value.task_type === 'long_term' && options.taskForm.value.restock_cycle === 'monthly' ? options.taskForm.value.restock_day_of_month : undefined,
      restock_times: options.taskForm.value.task_type === 'long_term' && options.taskForm.value.restock_times.length > 0 ? options.taskForm.value.restock_times : undefined,
      custom_cron: options.taskForm.value.task_type === 'long_term' ? options.taskForm.value.custom_cron.trim() || undefined : undefined,
      calendar_policy: options.taskForm.value.task_type === 'long_term' ? options.taskForm.value.calendar_policy : undefined,
      holiday_dates: options.taskForm.value.task_type === 'long_term' && options.taskForm.value.holiday_dates.length > 0 ? options.taskForm.value.holiday_dates : undefined,
      workday_dates: options.taskForm.value.task_type === 'long_term' && options.taskForm.value.workday_dates.length > 0 ? options.taskForm.value.workday_dates : undefined
    }

    const accountIds = options.taskForm.value.account_ids
    const exchangeRuleIds = options.taskForm.value.exchange_rule_ids
    const exchangeRuleId = options.taskForm.value.exchange_rule_id ?? options.taskForm.value.exchange_account_id ?? exchangeRuleIds[0] ?? null

    if (allowBatch && accountIds.length > 0) {
      payload.account_ids = accountIds
      payload.account_id = accountIds[0]
    } else if (allowBatch && exchangeRuleIds.length > 0) {
      payload.exchange_rule_ids = exchangeRuleIds
      payload.exchange_rule_id = exchangeRuleIds[0]
      payload.exchange_account_ids = exchangeRuleIds
      payload.exchange_account_id = exchangeRuleIds[0]
    } else if (options.taskForm.value.account_id || accountIds[0]) {
      payload.account_id = options.taskForm.value.account_id || accountIds[0]
    } else if (exchangeRuleId) {
      payload.exchange_rule_id = exchangeRuleId
      payload.exchange_account_id = exchangeRuleId
    } else {
      ElMessage.warning('请选择抢兑账号')
      return null
    }

    return payload
  }

  const openCreateTaskResultDialog = (results: CreateExchangeTaskItemResult[], payload: CreateExchangeTaskRequest) => {
    if (results.length === 0) return

    lastCreateTaskPayload.value = JSON.parse(JSON.stringify(payload)) as CreateExchangeTaskRequest
    lastCreateTaskResults.value = [...results]
    batchTaskResultDisplay.value = buildCreateTaskResultDisplay(results, {
      userAccounts: options.userAccounts.value,
      rules: options.accounts.value
    })
    batchTaskResultDialogVisible.value = true
  }

  const createTask = async (createOptions: CreateTaskOptions = {}) => {
    const payload = createOptions.payload || buildTaskPayload()
    if (!payload) return false

    try {
      const res = await createExchangeTask(payload)
      const created = res.created || res.tasks?.length || 1
      const warning = res.errors?.length ? `，${res.errors.length} 项创建失败` : ''

      if (createOptions.showSuccess !== false) {
        ElMessage.success(`${createOptions.successMessage || '创建任务成功'}：${created} 个${warning}`)
      }

      if (createOptions.forceResultDialog || (res.results?.length || 0) > 1 || (res.errors?.length || 0) > 0) {
        openCreateTaskResultDialog(res.results || [], payload)
      } else {
        resetBatchTaskResultState()
      }

      await Promise.all([options.loadTasks(), options.loadAccounts()])
      return true
    } catch (error: any) {
      // HTTP 4xx/5xx errors are already rendered by the global Axios interceptor.
      // Only add a local message for errors that did not receive an HTTP response.
      if (!error?.response) {
        ElMessage.error('创建任务失败：' + (error?.message || '网络异常'))
      }
      return false
    }
  }

  const retryFailedCreateTaskItems = async () => {
    const retryPayload = buildRetryCreateTaskPayload(lastCreateTaskPayload.value, lastCreateTaskResults.value)
    if (!retryPayload) {
      ElMessage.info('当前没有失败项可重试')
      return
    }

    retryFailedCreateTaskLoading.value = true
    try {
      const success = await createTask({
        payload: retryPayload,
        forceResultDialog: true,
        successMessage: '失败项重试完成'
      })

      if (success && !hasRetryableCreateTaskFailures.value) {
        ElMessage.success('失败项已全部重试完成')
      }
    } finally {
      retryFailedCreateTaskLoading.value = false
    }
  }

  const confirmImmediateExchange = async () => {
    const payload = buildTaskPayload(false)
    if (!payload) return false

    try {
      const operation = await immediateExchange(payload)
      ElMessage.success(operationQueuedMessage(operation, '兑换任务'))
      await Promise.all([options.loadTasks(), options.loadAccounts()])
      return true
    } catch (error: any) {
      if (!error?.response) {
        ElMessage.error('兑换失败：' + (error?.message || '网络异常'))
      }
      return false
    }
  }

  return {
    batchTaskResultDialogVisible,
    batchTaskResultDisplay,
    retryFailedCreateTaskLoading,
    selectableCloudAccounts,
    selectableExchangeAccounts,
    hasRetryableCreateTaskFailures,
    getTaskAccountCount,
    prepareTaskForm,
    buildTaskPayload,
    createTask,
    retryFailedCreateTaskItems,
    confirmImmediateExchange,
    resetBatchTaskResultState
  }
}
