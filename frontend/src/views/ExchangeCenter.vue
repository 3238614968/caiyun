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
      <el-tabs v-model="activeTab" type="border-card" class="exchange-tabs">
        <!-- 商品列表 -->
        <el-tab-pane label="商品中心" name="products">
          <ProductGallery
            v-model:keyword="searchKeyword"
            v-model:category="currentCategory"
            :is-mobile="isMobile"
            :products="filteredProducts"
            :categories="categories"
            :exchange-config="exchangeConfig"
            :get-product-image-url="getProductImageUrl"
            @search="handleSearch"
            @reserve="handleReserveProduct"
            @immediate="handleImmediateExchange"
            @create-task="showCreateTaskDialog"
          />
        </el-tab-pane>

        <!-- 兑换账号管理 -->
        <el-tab-pane label="兑换账号" name="accounts">
          <ExchangeAccountList
            :is-mobile="isMobile"
            :accounts="accounts"
            @add="showAddAccountDialog"
            @edit="editAccount"
            @delete="deleteAccount"
          />
        </el-tab-pane>

        <!-- 抢兑任务管理 -->
        <el-tab-pane label="抢兑任务" name="tasks">
          <ExchangeTaskList
            :is-mobile="isMobile"
            :tasks="tasks"
            @execute="executeTask"
            @delete="deleteTask"
          />
        </el-tab-pane>

        <!-- 领奖专区 -->
        <el-tab-pane label="领奖专区" name="rewards">
          <div class="rewards-section">
            <el-empty description="领奖专区功能开发中，敬请期待...">
              <template #image>
                <el-icon :size="60" color="#909399"><Present /></el-icon>
              </template>
            </el-empty>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <CreateExchangeTaskDialog
      v-model="taskDialogVisible"
      v-model:form="taskForm"
      :is-mobile="isMobile"
      :selected-product="selectedProduct"
      :accounts="accounts"
      @submit="createTask"
    />

    <ImmediateExchangeDialog
      v-model="immediateExchangeDialogVisible"
      v-model:form="taskForm"
      :is-mobile="isMobile"
      :selected-product="selectedProduct"
      :accounts="accounts"
      @submit="confirmImmediateExchange"
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
  </div>
</template>

<script setup lang="ts">
import '@/styles/element/exchange'
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Present } from '@element-plus/icons-vue'
import {
  searchProducts,
  getProductCategories,
  getExchangeAccounts,
  addExchangeAccount,
  updateExchangeAccount,
  deleteExchangeAccount,
  createExchangeTask,
  getExchangeTasks,
  deleteExchangeTask,
  executeExchangeTask,
  immediateExchange,
  getExchangeConfigPublic,
  type ExchangeConfig
} from '@/api/exchange'
import { getAccounts, searchAllAccounts, type AccountSearchItem } from '@/api/account'
import { useAuthStore } from '@/store/auth'
import { useExchangeMedia } from '@/composables/exchange/useExchangeMedia'
import { useExchangeForms } from '@/composables/exchange/useExchangeForms'
import { useExchangeDisplay } from '@/composables/exchange/useExchangeDisplay'
import ProductGallery from '@/components/exchange/ProductGallery.vue'
import ExchangeHeader from '@/components/exchange/ExchangeHeader.vue'
import ExchangeAccountList from '@/components/exchange/ExchangeAccountList.vue'
import ExchangeTaskList from '@/components/exchange/ExchangeTaskList.vue'
import CreateExchangeTaskDialog from '@/components/exchange/CreateExchangeTaskDialog.vue'
import ImmediateExchangeDialog from '@/components/exchange/ImmediateExchangeDialog.vue'
import ExchangeAccountDialog from '@/components/exchange/ExchangeAccountDialog.vue'

// 状态
const activeTab = ref('products')
const searchKeyword = ref('')
const currentCategory = ref('')
const exchangeConfig = ref<ExchangeConfig | null>(null)
const categories = ref<string[]>([])
const products = ref<any[]>([])
const accounts = ref<any[]>([])
const tasks = ref<any[]>([])
const userAccounts = ref<any[]>([])
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
  getProductImageUrl
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
  resetAccountForm
} = useExchangeForms()

const {
  filteredProducts,
  totalSuccess,
  totalFail
} = useExchangeDisplay(products, tasks, currentCategory, searchKeyword)

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

  const hasSelectedAccount = userAccounts.value.some((acc: any) => acc.id === accountForm.value.account_id)
  if (!hasSelectedAccount) {
    const preferredAccount = userAccounts.value.find((acc: any) => acc.is_active) || userAccounts.value[0]
    accountForm.value.account_id = preferredAccount?.id ?? null
  }
}

const syncDefaultProductSelection = () => {
  if (isEditingAccount.value) return
  if (products.value.length === 0) {
    accountForm.value.product_id = null
    return
  }

  const hasSelectedProduct = products.value.some((product: any) => product.id === accountForm.value.product_id)
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
    const res = await getExchangeAccounts()
    accounts.value = res.accounts || []
  } catch (error: any) {
    ElMessage.error('加载兑换账号失败：' + error.message)
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
    userAccounts.value = (res.accounts || []).filter((account: any) => !!account?.id)
    syncDefaultAccountSelection()
  } catch (error: any) {
    userAccounts.value = []
    ElMessage.error('加载云盘账号失败：' + error.message)
  } finally {
    userAccountsLoading.value = false
  }
}

// 管理员搜索所有账号
const searchAccounts = async (keyword: string) => {
  if (!isAdmin.value) return
  if (!keyword || keyword.length < 2) {
    allAccountsSearchResults.value = []
    return
  }
  accountSearchLoading.value = true
  try {
    const res = await searchAllAccounts(keyword, 20)
    allAccountsSearchResults.value = res.accounts || []
  } catch (error: any) {
    console.error('搜索账号失败：', error.message)
  } finally {
    accountSearchLoading.value = false
  }
}

const handleSearch = () => {
  // 搜索已在前端完成，无需额外请求
}

const showCreateTaskDialog = (product: any) => {
  // 检查是否有兑换账号
  if (accounts.value.length === 0) {
    ElMessage.warning('请先添加兑换账号')
    activeTab.value = 'accounts'
    return
  }
  selectedProduct.value = product
  taskForm.value.product_id = product.id
  taskForm.value.exchange_account_id = accounts.value[0]?.id || 0
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

const handleImmediateExchange = async (product: any) => {
  // 检查是否有兑换账号
  if (accounts.value.length === 0) {
    ElMessage.warning('请先添加兑换账号')
    activeTab.value = 'accounts'
    return
  }

  // 如果只有一个账号，直接兑换；否则让用户选择
  if (accounts.value.length === 1) {
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
      await immediateExchange({
        exchange_account_id: accounts.value[0].id,
        product_id: product.id
      })
      ElMessage.success('兑换任务已启动')
    } catch (error: any) {
      if (error !== 'cancel') {
        ElMessage.error('兑换失败：' + error.message)
      }
    }
  } else {
    // 多个账号，弹出选择框
    selectedProduct.value = product
    taskForm.value.product_id = product.id
    taskForm.value.exchange_account_id = accounts.value[0]?.id || 0
    immediateExchangeDialogVisible.value = true
  }
}

// 预定商品（创建定时抢兑任务）
const handleReserveProduct = async (product: any) => {
  // 检查是否有兑换账号
  if (accounts.value.length === 0) {
    ElMessage.warning('请先添加兑换账号')
    activeTab.value = 'accounts'
    return
  }

  // 判断商品分类，设置不同的抢兑时间
  let exchangeTime = '10:00:00' // 默认10点
  const category = product.category || ''
  const prizeName = product.prize_name || ''

  // Group 10 奶茶券是每周五 10:30
  if (category.includes('奶茶') || category.includes('饮品') || prizeName.includes('茶') || prizeName.includes('喜茶') || prizeName.includes('蜜雪')) {
    // 检查今天是否是周五
    const today = new Date().getDay()
    if (today === 5) { // 周五
      exchangeTime = '10:30:00'
    }
  }

  // 如果只有一个账号，直接创建预定任务
  if (accounts.value.length === 1) {
    try {
      await ElMessageBox.confirm(
        `确定要预定 "${product.prize_name}" 吗？\n系统将在 ${exchangeTime.substring(0, 5)} 自动尝试抢兑。`,
        '确认预定',
        {
          confirmButtonText: '确定',
          cancelButtonText: '取消',
          type: 'warning'
        }
      )
      // 创建抢兑任务（长期任务，持续尝试）
      await createExchangeTask({
        exchange_account_id: accounts.value[0].id,
        product_id: product.id,
        task_type: 'long_term',
        max_attempts: 10
      })
      ElMessage.success('预定成功，将在指定时间自动抢兑')
      loadTasks()
    } catch (error: any) {
      if (error !== 'cancel') {
        ElMessage.error('预定失败：' + error.message)
      }
    }
  } else {
    // 多个账号，弹出选择框
    selectedProduct.value = product
    taskForm.value.product_id = product.id
    taskForm.value.exchange_account_id = accounts.value[0]?.id || 0
    taskForm.value.task_type = 'long_term'
    taskForm.value.max_attempts = 10
    taskDialogVisible.value = true
    ElMessage.info(`已为您选择长期抢兑模式，将在 ${exchangeTime.substring(0, 5)} 开始自动抢兑`)
  }
}

const createTask = async () => {
  try {
    await createExchangeTask(taskForm.value)
    ElMessage.success('创建任务成功')
    taskDialogVisible.value = false
    loadTasks()
  } catch (error: any) {
    ElMessage.error('创建任务失败：' + error.message)
  }
}

const confirmImmediateExchange = async () => {
  if (!taskForm.value.exchange_account_id) {
    ElMessage.warning('请选择兑换账号')
    return
  }
  try {
    await immediateExchange({
      exchange_account_id: taskForm.value.exchange_account_id,
      product_id: taskForm.value.product_id
    })
    ElMessage.success('兑换任务已启动')
    immediateExchangeDialogVisible.value = false
  } catch (error: any) {
    ElMessage.error('兑换失败：' + error.message)
  }
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
      await updateExchangeAccount(editingAccountId.value!, {
        remark,
        exchange_time_1: accountForm.value.exchange_time_1,
        exchange_time_2: accountForm.value.exchange_time_2,
        is_active: accountForm.value.is_active,
        product_id: accountForm.value.product_id
      })
      ElMessage.success('更新账号成功')
    } else {
      await addExchangeAccount({
        account_id: accountForm.value.account_id,
        remark,
        exchange_time_1: accountForm.value.exchange_time_1,
        exchange_time_2: accountForm.value.exchange_time_2,
        product_id: accountForm.value.product_id
      })
      ElMessage.success('添加账号成功')
    }
    accountDialogVisible.value = false
    await Promise.all([loadAccounts(), loadTasks()])
  } catch (error: any) {
    ElMessage.error('保存账号失败：' + error.message)
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
    await ElMessageBox.confirm('确定要删除这个兑换账号吗？', '提示', {
      type: 'warning'
    })
    await deleteExchangeAccount(id)
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
    await executeExchangeTask(id)
    ElMessage.success('任务执行成功')
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
.rewards-section { padding-top:0; min-height:220px; display:flex; align-items:center; justify-content:center; }
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
