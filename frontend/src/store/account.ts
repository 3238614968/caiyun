import { defineStore } from 'pinia'
import { ref } from 'vue'
import {
  getAccounts,
  getAccount,
  createAccount,
  updateAccount,
  deleteAccount,
  setAccountStatus,
  refreshAccountToken,
  triggerAccountTask,
  type Account,
  type CreateAccountRequest,
  type UpdateAccountRequest
} from '../api/account'

export const useAccountStore = defineStore('account', () => {
  const accounts = ref<Account[]>([])
  const currentAccount = ref<Account | null>(null)
  const loading = ref(false)

  // 获取账号列表
  const fetchAccounts = async (page: number = 1, pageSize: number = 10, phone: string = '') => {
    loading.value = true
    try {
      const data = await getAccounts(page, pageSize, phone)
      accounts.value = data.accounts
      return data
    } finally {
      loading.value = false
    }
  }

  // 获取账号详情
  const fetchAccount = async (id: number) => {
    loading.value = true
    try {
      const account = await getAccount(id)
      currentAccount.value = account
      return account
    } finally {
      loading.value = false
    }
  }

  // 创建账号
  const createNewAccount = async (data: CreateAccountRequest) => {
    loading.value = true
    try {
      const account = await createAccount(data)
      accounts.value.push(account)
      return account
    } finally {
      loading.value = false
    }
  }

  // 更新账号
  const updateCurrentAccount = async (id: number, data: UpdateAccountRequest) => {
    loading.value = true
    try {
      const account = await updateAccount(id, data)
      const index = accounts.value.findIndex(a => a.id === id)
      if (index !== -1) {
        accounts.value[index] = account
      }
      if (currentAccount.value?.id === id) {
        currentAccount.value = account
      }
      return account
    } finally {
      loading.value = false
    }
  }

  // 删除账号
  const removeAccount = async (id: number) => {
    loading.value = true
    try {
      await deleteAccount(id)
      accounts.value = accounts.value.filter(a => a.id !== id)
      if (currentAccount.value?.id === id) {
        currentAccount.value = null
      }
    } finally {
      loading.value = false
    }
  }

  // 设置账号状态
  const updateAccountActiveStatus = async (id: number, isActive: boolean) => {
    loading.value = true
    try {
      await setAccountStatus(id, isActive)
      const account = accounts.value.find(a => a.id === id)
      if (account) {
        account.is_active = isActive
      }
      if (currentAccount.value?.id === id) {
        currentAccount.value.is_active = isActive
      }
    } finally {
      loading.value = false
    }
  }

  // 刷新Token
  const refreshAccount = async (id: number) => {
    loading.value = true
    try {
      const account = await refreshAccountToken(id)
      const index = accounts.value.findIndex(a => a.id === id)
      if (index !== -1) {
        accounts.value[index] = account
      }
      if (currentAccount.value?.id === id) {
        currentAccount.value = account
      }
      return account
    } finally {
      loading.value = false
    }
  }

  // 触发任务执行
  const triggerTask = async (id: number) => {
    loading.value = true
    try {
      await triggerAccountTask(id)
    } finally {
      loading.value = false
    }
  }

  // 清空当前账号
  const clearCurrentAccount = () => {
    currentAccount.value = null
  }

  return {
    accounts,
    currentAccount,
    loading,
    fetchAccounts,
    fetchAccount,
    createNewAccount,
    updateCurrentAccount,
    removeAccount,
    updateAccountActiveStatus,
    refreshAccount,
    triggerTask,
    clearCurrentAccount
  }
})
