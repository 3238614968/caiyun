<template>
  <div class="exchange-center">
    <ExchangeHeader
      :active-tab="activeTab"
      :accounts-count="accounts.length"
      :tasks-count="tasks.length"
      :total-success="totalSuccess"
      :total-fail="totalFail"
      @add-account="showAddAccountDialog"
    />

    <div class="content">
      <!-- 选项卡 -->
      <el-tabs
        v-model="activeTab"
        type="border-card"
        class="exchange-tabs"
      >
        <!-- 商品列表 -->
        <el-tab-pane
          label="商品中心"
          name="products"
        >
          <ProductGallery
            v-model:keyword="searchKeyword"
            v-model:category="currentCategory"
            :is-mobile="isMobile"
            :products="filteredProducts"
            :categories="categories"
            :exchange-config="exchangeConfig"
            :get-product-image-source="getProductImageSource"
            @search="handleSearch"
            @reserve="handleReserveProduct"
            @immediate="handleImmediateExchange"
            @create-task="showCreateTaskDialog"
          />
        </el-tab-pane>

        <!-- 账号规则管理 -->
        <el-tab-pane
          label="抢兑规则"
          name="accounts"
        >
          <ExchangeAccountList
            :is-mobile="isMobile"
            :accounts="accounts"
            @add="showAddAccountDialog"
            @edit="editAccount"
            @delete="deleteAccount"
          />
        </el-tab-pane>

        <!-- 抢兑任务管理 -->
        <el-tab-pane
          label="抢兑任务"
          name="tasks"
        >
          <ExchangeTaskManager
            v-model:filters="taskFilters"
            :is-mobile="isMobile"
            :tasks="filteredTasks"
            @add="showCreateTaskDialog()"
            @execute="executeTask"
            @delete="deleteTask"
          />
        </el-tab-pane>

        <!-- 领奖专区 -->
        <el-tab-pane
          label="领奖专区"
          name="rewards"
        >
          <ExchangeRewardsPlaceholder />
        </el-tab-pane>
      </el-tabs>
    </div>

    <CreateExchangeTaskDialog
      v-model="taskDialogVisible"
      v-model:form="taskForm"
      :is-mobile="isMobile"
      :selected-product="selectedProduct"
      :accounts="accounts"
      :products="products"
      :user-accounts="userAccounts"
      :user-accounts-loading="userAccountsLoading"
      @submit="submitCreateTask"
    />

    <ImmediateExchangeDialog
      v-model="immediateExchangeDialogVisible"
      v-model:form="taskForm"
      :is-mobile="isMobile"
      :selected-product="selectedProduct"
      :accounts="accounts"
      :user-accounts="userAccounts"
      :user-accounts-loading="userAccountsLoading"
      @submit="submitImmediateExchange"
    />

    <ExchangeAccountDialog
      v-model="accountDialogVisible"
      v-model:form="accountForm"
      :is-mobile="isMobile"
      :is-admin="isAdmin"
      :is-editing-account="isEditingAccount"
      :width="accountDialogWidth"
      :top="accountDialogTop"
      :control-size="accountDialogControlSize"
      :user-accounts="userAccounts"
      :user-accounts-loading="userAccountsLoading"
      :all-accounts-search-results="allAccountsSearchResults"
      :account-search-loading="accountSearchLoading"
      :products="products"
      @search-accounts="searchAccounts"
      @submit="saveAccount"
    />

    <BatchCreateTaskResultDialog
      v-model="batchTaskResultDialogVisible"
      :is-mobile="isMobile"
      :display="batchTaskResultDisplay"
      :retryable="hasRetryableCreateTaskFailures"
      :retry-loading="retryFailedCreateTaskLoading"
      @retry="retryFailedCreateTaskItems"
    />
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  searchProducts,
  getProductCategories,
  getExchangeRules,
  addExchangeRule,
  updateExchangeRule,
  deleteExchangeRule,
  getExchangeTasks,
  deleteExchangeTask,
  executeExchangeTask,
  getExchangeConfigPublic,
  type ExchangeConfig,
  type ExchangeRule,
  type ExchangeTask,
  type Product
} from '@/api/exchange'
import { getAccounts, searchAllAccounts, type Account, type AccountSearchItem } from '@/api/account'
import { useAuthStore } from '@/store/auth'
import { useExchangeMedia } from '@/composables/exchange/useExchangeMedia'
import { useExchangeForms } from '@/composables/exchange/useExchangeForms'
import { useExchangeDisplay } from '@/composables/exchange/useExchangeDisplay'
import { useExchangeTaskFilters } from '@/composables/exchange/useExchangeTaskFilters'
import { useExchangeTaskActions } from '@/composables/exchange/useExchangeTaskActions'
import { createExchangeReservationPreset } from '@/composables/exchange/useExchangeReservationPreset'
import { operationQueuedMessage } from '@/api/operation'
import ProductGallery from '@/components/exchange/ProductGallery.vue'
import ExchangeHeader from '@/components/exchange/ExchangeHeader.vue'
import ExchangeAccountList from '@/components/exchange/ExchangeAccountList.vue'
import ExchangeTaskManager from '@/components/exchange/ExchangeTaskManager.vue'
import CreateExchangeTaskDialog from '@/components/exchange/CreateExchangeTaskDialog.vue'
import ImmediateExchangeDialog from '@/components/exchange/ImmediateExchangeDialog.vue'
import ExchangeAccountDialog from '@/components/exchange/ExchangeAccountDialog.vue'
import BatchCreateTaskResultDialog from '@/components/exchange/BatchCreateTaskResultDialog.vue'
import ExchangeRewardsPlaceholder from '@/components/exchange/ExchangeRewardsPlaceholder.vue'

// 状态
const activeTab = ref('products')
const searchKeyword = ref('')
const currentCategory = ref('')
const exchangeConfig = ref<ExchangeConfig | null>(null)
const categories = ref<string[]>([])
const products = ref<Product[]>([])
const accounts = ref<ExchangeRule[]>([])
const tasks = ref<ExchangeTask[]>([])
const userAccounts = ref<Account[]>([])
const userAccountsLoading = ref(false)
const compactAccountDialog = ref(false)
// 用户权限
const authStore = useAuthStore()
const isAdmin = computed(() => authStore.user?.role === 'admin')

// 管理员搜索所有账号
const allAccountsSearchResults = ref<AccountSearchItem[]>([])
const accountSearchLoading = ref(false)

const {
  isMobile,
  checkMobile,
  loadLocalImageMap,
  getProductImageSource
} = useExchangeMedia()

const {
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
} = useExchangeForms()

const {
  filteredProducts,
  totalSuccess,
  totalFail
} = useExchangeDisplay(products, tasks, currentCategory, searchKeyword)

const {
  taskFilters,
  filteredTasks
} = useExchangeTaskFilters(tasks)

const accountDialogWidth = computed(() => {
  if (isMobile.value) return '95%'
  return compactAccountDialog.value ? '620px' : '680px'
})

const accountDialogTop = computed(() => (
  isMobile.value ? '2vh' : compactAccountDialog.value ? '4vh' : '6vh'
))

const accountDialogControlSize = computed(() => (
  isMobile.value || compactAccountDialog.value ? 'default' : 'large'
))

const sortCloudAccounts = (list: Account[]) => {
  const getTime = (value?: string) => {
    const time = value ? new Date(value).getTime() : 0
    return Number.isNaN(time) ? 0 : time
  }

  return [...list].sort((a, b) => {
    const activeDiff = Number(Boolean(b.is_active)) - Number(Boolean(a.is_active))
    if (activeDiff !== 0) return activeDiff

    const cloudDiff = Number(b.cloud_count || 0) - Number(a.cloud_count || 0)
    if (cloudDiff !== 0) return cloudDiff

    return getTime(b.created_at) - getTime(a.created_at)
  })
}

const syncViewportState = () => {
  checkMobile()
  compactAccountDialog.value = window.innerHeight <= 980 || window.innerWidth <= 1440
}

const syncDefaultAccountSelection = () => {
  if (isAdmin.value || isEditingAccount.value) return
  if (userAccounts.value.length === 0) {
    accountForm.value.account_id = null
    return
  }

  const hasSelectedAccount = userAccounts.value.some((acc) => acc.id === accountForm.value.account_id)
  if (!hasSelectedAccount) {
    const preferredAccount = userAccounts.value.find((acc) => acc.is_active) || userAccounts.value[0]
    accountForm.value.account_id = preferredAccount?.id ?? null
  }
}

const syncDefaultProductSelection = () => {
  if (isEditingAccount.value) return
  if (products.value.length === 0) {
    accountForm.value.product_id = null
    return
  }

  const hasSelectedProduct = products.value.some((product) => product.id === accountForm.value.product_id)
  if (!hasSelectedProduct) {
    accountForm.value.product_id = products.value[0].id
  }
}

// 方法
const loadProducts = async () => {
  try {
    const res = await searchProducts('', 100)
    products.value = res.products || []
    syncDefaultProductSelection()
  } catch (error: any) {
    ElMessage.error('加载商品失败：' + error.message)
  }
}

const loadCategories = async () => {
  try {
    const res = await getProductCategories()
    categories.value = res.categories || []
  } catch (error: any) {
    ElMessage.error('加载分类失败：' + error.message)
  }
}

const loadAccounts = async () => {
  try {
    const res = await getExchangeRules()
    accounts.value = res.rules || res.accounts || []
  } catch (error: any) {
    ElMessage.error('加载抢兑规则失败：' + error.message)
  }
}

const loadTasks = async () => {
  try {
    const res = await getExchangeTasks()
    tasks.value = res.tasks || []
  } catch (error: any) {
    ElMessage.error('加载任务失败：' + error.message)
  }
}

const loadUserAccounts = async (force = false) => {
  if (isAdmin.value) {
    userAccounts.value = []
    return
  }
  if (userAccountsLoading.value) return
  if (!force && userAccounts.value.length > 0) {
    syncDefaultAccountSelection()
    return
  }

  userAccountsLoading.value = true
  try {
    const res = await getAccounts(1, 200)
    userAccounts.value = sortCloudAccounts((res.accounts || []).filter((account) => !!account?.id))
    syncDefaultAccountSelection()
  } catch (error: any) {
    userAccounts.value = []
    ElMessage.error('加载云盘账号失败：' + error.message)
  } finally {
    userAccountsLoading.value = false
  }
}

const {
  batchTaskResultDialogVisible,
  batchTaskResultDisplay,
  retryFailedCreateTaskLoading,
  selectableCloudAccounts,
  hasRetryableCreateTaskFailures,
  getTaskAccountCount,
  prepareTaskForm,
  createTask,
  retryFailedCreateTaskItems,
  confirmImmediateExchange: runImmediateExchange
} = useExchangeTaskActions({
  isAdmin,
  products,
  accounts,
  userAccounts,
  selectedProduct,
  taskForm,
  resetTaskForm,
  loadProducts,
  loadAccounts,
  loadTasks,
  loadUserAccounts,
  syncDefaultProductSelection
})

// 管理员搜索所有账号
const searchAccounts = async (keyword: string) => {
  if (!isAdmin.value) return
  const trimmedKeyword = keyword.trim()
  if (!trimmedKeyword) {
    allAccountsSearchResults.value = []
    return
  }
  accountSearchLoading.value = true
  try {
    const res = await searchAllAccounts(trimmedKeyword, 20)
    allAccountsSearchResults.value = [...(res.accounts || [])].sort((a, b) => Number(Boolean(b.is_active)) - Number(Boolean(a.is_active)))
  } catch (error: any) {
    console.error('搜索账号失败：', error.message)
  } finally {
    accountSearchLoading.value = false
  }
}

const handleSearch = () => {
  // 搜索已在前端完成，无需额外请求
}

const showCreateTaskDialog = async (product?: Product) => {
  const prepared = await prepareTaskForm(product, 'fixed', 1)
  if (!prepared) return
  taskDialogVisible.value = true
}

const showAddAccountDialog = async () => {
  editingAccountId.value = null
  resetAccountForm()

  if (products.value.length === 0) {
    await loadProducts()
  } else {
    syncDefaultProductSelection()
  }

  if (products.value.length === 0) {
    ElMessage.warning('暂无可用商品，请先刷新商品列表')
    return
  }

  if (!isAdmin.value) {
    await loadUserAccounts(true)
    if (userAccounts.value.length === 0) {
      ElMessage.warning('暂无云盘账号，请先到账号页面添加')
      return
    }
    if (selectableCloudAccounts.value.length === 0) {
      ElMessage.warning('暂无可用云盘账号，请先启用账号后再添加抢兑规则')
      return
    }
  } else {
    allAccountsSearchResults.value = []
  }

  accountDialogVisible.value = true
}

const loadExchangeConfig = async () => {
  try {
    const res = await getExchangeConfigPublic()
    exchangeConfig.value = {
      enabled: res.enabled,
      immediate_exchange_enabled: res.immediate_exchange_enabled,
      auto_update_products: false,
      concurrency: 10,
      exchange_monthly_enabled: false,
      exchange_time: '00:00',
      monthly_prize_id: ''
    }
  } catch (error: any) {
    console.error('加载兑换配置失败：', error.message)
  }
}

const handleImmediateExchange = async (product: Product) => {
  const prepared = await prepareTaskForm(product, 'fixed', 1)
  if (!prepared) return

  if (getTaskAccountCount() > 1) {
    immediateExchangeDialogVisible.value = true
    return
  }

  try {
    await ElMessageBox.confirm(
      `确定要立即兑换 "${product.prize_name}" 吗？`,
      '确认兑换',
      {
        confirmButtonText: '确定',
        cancelButtonText: '取消',
        type: 'warning'
      }
    )
    await submitImmediateExchange()
  } catch (error: any) {
    if (error !== 'cancel') {
      console.error('立即兑换确认失败：', error)
    }
  }
}

// 预定商品（创建长期抢兑任务）
const handleReserveProduct = async (product: Product) => {
  const preset = createExchangeReservationPreset(product)
  const prepared = await prepareTaskForm(product, 'long_term', 10)
  if (!prepared) return

  taskForm.value.scheduled_exchange_time = preset.exchangeTime
  taskForm.value.restock_cycle = preset.restockCycle
  taskForm.value.restock_weekday = preset.restockWeekday
  taskForm.value.restock_day_of_month = preset.restockDayOfMonth
  taskForm.value.restock_times = preset.restockTimes
  taskForm.value.custom_cron = preset.customCron
  taskForm.value.calendar_policy = preset.calendarPolicy
  taskDialogVisible.value = true
  ElMessage.info(`请确认预定配置：抢兑时间 ${preset.exchangeTime.substring(0, 5)}，补货周期可在弹窗中调整`)
}

const submitCreateTask = async () => {
  const success = await createTask()
  if (success) {
    taskDialogVisible.value = false
  }
}

const submitImmediateExchange = async () => {
  const success = await runImmediateExchange()
  if (success) {
    immediateExchangeDialogVisible.value = false
  }
  return success
}

const saveAccount = async () => {
  if (!accountForm.value.account_id) {
    ElMessage.warning('请选择云盘账号')
    return
  }
  if (!accountForm.value.product_id) {
    ElMessage.warning('请选择商品')
    return
  }

  const remark = accountForm.value.remark.trim()

  try {
    if (isEditingAccount.value) {
      await updateExchangeRule(editingAccountId.value!, {
        remark,
        exchange_time_1: accountForm.value.exchange_time_1,
        exchange_time_2: accountForm.value.exchange_time_2,
        is_active: accountForm.value.is_active,
        product_id: accountForm.value.product_id
      })
      ElMessage.success('更新抢兑规则成功')
    } else {
      await addExchangeRule({
        account_id: accountForm.value.account_id,
        remark,
        exchange_time_1: accountForm.value.exchange_time_1,
        exchange_time_2: accountForm.value.exchange_time_2,
        product_id: accountForm.value.product_id
      })
      ElMessage.success('添加抢兑规则成功')
    }
    accountDialogVisible.value = false
    await Promise.all([loadAccounts(), loadTasks()])
  } catch (error: any) {
    ElMessage.error('保存抢兑规则失败：' + error.message)
  }
}

const editAccount = async (row: any) => {
  editingAccountId.value = row.id
  // 获取当前商品ID：优先从 current_product 获取，否则尝试从 tasks 中获取
  let currentProductId = null
  if (row.current_product) {
    currentProductId = row.current_product.id
  } else if (row.tasks && row.tasks.length > 0) {
    // 查找待执行或进行中的任务
    const activeTask = row.tasks.find((t: any) => t.status === 'pending' || t.status === 'running')
    if (activeTask) {
      currentProductId = activeTask.product_id
    }
  }

  accountForm.value = {
    account_id: row.account_id,
    product_id: currentProductId,
    remark: row.remark,
    exchange_time_1: row.exchange_time_1,
    exchange_time_2: row.exchange_time_2,
    is_active: row.is_active
  }

  if (products.value.length === 0) {
    await loadProducts()
  }
  if (!isAdmin.value) {
    await loadUserAccounts(true)
  }

  accountDialogVisible.value = true
}

const deleteAccount = async (id: number) => {
  try {
    await ElMessageBox.confirm('确定要删除这个抢兑规则吗？', '提示', {
      type: 'warning'
    })
    await deleteExchangeRule(id)
    ElMessage.success('删除成功')
    await Promise.all([loadAccounts(), loadTasks()])
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

const deleteTask = async (id: number) => {
  try {
    await ElMessageBox.confirm('确定要删除这个抢兑任务吗？', '提示', {
      type: 'warning'
    })
    await deleteExchangeTask(id)
    ElMessage.success('删除成功')
    loadTasks()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

const executeTask = async (id: number) => {
  try {
    const operation = await executeExchangeTask(id)
    ElMessage.success(operationQueuedMessage(operation))
    loadTasks()
  } catch (error: any) {
    ElMessage.error('执行失败：' + error.message)
  }
}

onMounted(async () => {
  syncViewportState()
  window.addEventListener('resize', syncViewportState)
  await loadLocalImageMap() // 先加载本地图片映射
  loadExchangeConfig() // 加载兑换配置
  loadProducts()
  loadCategories()
  loadAccounts()
  loadTasks()
  loadUserAccounts()
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewportState)
})
</script>

<style scoped>
.exchange-center { padding: clamp(12px, 2vw, 24px); max-width: 1680px; margin: 0 auto; }
.content { background: rgba(255,255,255,.74); backdrop-filter: blur(18px); -webkit-backdrop-filter: blur(18px); border-radius:24px; padding: clamp(16px, 2vw, 24px); box-shadow: 0 18px 42px rgba(37,99,235,.1); border:1px solid rgba(255,255,255,.76); }
.exchange-tabs { min-height:0; }
.exchange-tabs :deep(.el-tabs__header) { margin:0 0 18px; }
.exchange-tabs :deep(.el-tabs__nav-wrap) { overflow-x:auto; scrollbar-width:none; }
.exchange-tabs :deep(.el-tabs__nav-wrap::-webkit-scrollbar) { display:none; }
.exchange-tabs :deep(.el-tabs__nav-scroll) { display:flex; }
.exchange-tabs :deep(.el-tabs__nav) { flex-wrap:nowrap; }
.exchange-tabs :deep(.el-tabs__item) { height:42px; padding:0 18px; font-size:14px; font-weight:600; white-space:nowrap; }
.exchange-tabs :deep(.el-tabs__content) { padding:8px 0 0; }
:deep(.el-table) { border-radius:18px; overflow:hidden; --el-table-border-color: rgba(148,163,184,.18); --el-table-header-bg-color: rgba(248,250,252,.9); --el-table-row-hover-bg-color: rgba(239,246,255,.74); }
:deep(.el-table .cell) { line-height:1.45; }
:deep(.account-dialog .el-dialog) { max-width: calc(100vw - 32px); }
:deep(.account-dialog .el-dialog__header) { margin:0; padding: clamp(14px, 1.8vh, 20px) clamp(16px, 2.2vw, 24px); border-bottom:1px solid rgba(226,232,240,.9); }
:deep(.account-dialog .el-dialog__title) { font-size:18px; font-weight:700; color:#0f172a; }
:deep(.account-dialog .el-dialog__body) { padding:0; }
:deep(.account-dialog .el-dialog__footer) { padding: clamp(12px, 1.6vh, 16px) clamp(16px, 2.2vw, 24px); border-top:1px solid rgba(226,232,240,.9); }
:deep(.account-dialog .el-form-item__label) { font-weight:600; color:#475569; }
:deep(.account-dialog .el-input__wrapper), :deep(.account-dialog .el-select .el-input__wrapper) { min-height:40px; border-radius:12px; box-shadow: 0 0 0 1px rgba(203,213,225,.9) inset; }
@media (max-width: 1280px) { .exchange-center { padding:14px; } }
@media (max-width: 768px) { .exchange-center { padding:0; } .content { border-radius:20px; padding:14px; } .exchange-tabs :deep(.el-tabs__item) { padding:0 14px; font-size:13px; } }
</style>
