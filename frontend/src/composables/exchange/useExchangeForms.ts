import { computed, ref } from 'vue'
import type { Product } from '@/api/exchange'

export interface ExchangeTaskForm {
  exchange_account_id: number
  product_id: number
  task_type: 'fixed' | 'long_term'
  max_attempts: number
}

export interface ExchangeAccountForm {
  account_id: number | null
  product_id: number | null
  remark: string
  exchange_time_1: string
  exchange_time_2: string
  is_active: boolean
}

function createTaskForm(): ExchangeTaskForm {
  return {
    exchange_account_id: 0,
    product_id: 0,
    task_type: 'fixed',
    max_attempts: 1
  }
}

function createAccountForm(): ExchangeAccountForm {
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
  const accountForm = ref<ExchangeAccountForm>(createAccountForm())

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
