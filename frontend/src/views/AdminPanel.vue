<template>
  <div class="admin-panel-container">
    <el-card shadow="hover" class="admin-shell">
      <template #header>
        <div class="card-header">
          <span>管理员面板</span>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="handleTabChange" class="admin-tabs">
        <!-- 账号概况 -->
        <el-tab-pane label="账号概况" name="summaries">
          <div class="tab-content">
            <AdminSummaryList
              :is-mobile="isMobile"
              :loading="summaryLoading"
              :summaries="summaryList"
              :page="summaryPagination.page"
              :page-size="summaryPagination.pageSize"
              :total="summaryPagination.total"
              @size-change="handleSummarySizeChange"
              @page-change="handleSummaryPageChange"
            />
          </div>
        </el-tab-pane>

        <!-- 任务管理 -->
        <el-tab-pane label="任务管理" name="tasks">
          <div class="tab-content">
            <p style="color: #666; margin-bottom: 16px">
              下架的任务将不会被手动执行和定时任务执行。
            </p>
            <AdminTaskConfigList
              :is-mobile="isMobile"
              :loading="taskConfigLoading"
              :configs="taskConfigs"
              :format-date="formatDate"
              @toggle="handleTaskConfigToggle"
            />
          </div>
        </el-tab-pane>

        <!-- 抢兑配置 -->
        <el-tab-pane label="抢兑配置" name="exchange">
          <div class="tab-content">
            <AdminExchangeConfig
              :config="exchangeConfig"
              :source-accounts="availableProductSourceAccounts"
              v-model:selected-account-id="selectedAccountId"
              :update-products-loading="updateProductsLoading"
              :monthly-exchange-loading="monthlyExchangeLoading"
              @change="patchExchangeConfig"
              @save="saveExchangeConfig"
              @execute-monthly="executeMonthlyExchange"
              @update-products="handleUpdateProducts"
            />
          </div>
        </el-tab-pane>

        <!-- 用户管理 -->
        <el-tab-pane label="用户管理" name="users">
          <div class="tab-content">
            <AdminUserList
              :is-mobile="isMobile"
              :loading="userLoading"
              :users="userList"
              :page="userPagination.page"
              :page-size="userPagination.pageSize"
              :total="userPagination.total"
              :format-date="formatDate"
              @edit-role="handleEditUserRole"
              @reset-password="handleResetUserPassword"
              @delete-user="handleDeleteUser"
              @size-change="handleUserSizeChange"
              @page-change="handleUserPageChange"
            />
          </div>
        </el-tab-pane>

        <!-- 统计概览 -->
        <el-tab-pane label="统计概览" name="stats">
          <div class="tab-content">
            <AdminStatsOverview :stats="statsOverview" />
          </div>
        </el-tab-pane>

        <!-- 公告管理 -->
        <el-tab-pane label="公告管理" name="announcements">
          <div class="tab-content">
            <AdminAnnouncementList
              :is-mobile="isMobile"
              :loading="announcementLoading"
              :announcements="announcements"
              :format-date="formatDate"
              @create="showAddAnnouncementDialog"
              @view="viewAnnouncement"
              @edit="editAnnouncement"
              @delete="deleteAnnouncement"
            />
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <!-- 修改角色对话框 -->
    <el-dialog v-model="roleDialogVisible" title="修改用户角色" width="400px">
      <el-form :model="roleForm" label-width="80px">
        <el-form-item label="角色">
          <el-radio-group v-model="roleForm.role">
            <el-radio label="user">普通用户</el-radio>
            <el-radio label="admin">管理员</el-radio>
          </el-radio-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="roleDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleRoleSubmit">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码对话框 -->
    <el-dialog v-model="passwordDialogVisible" title="重置用户密码" width="420px">
      <el-form :model="passwordForm" label-width="90px">
        <el-form-item label="用户">
          <el-input v-model="passwordForm.username" disabled />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="passwordForm.password" type="password" show-password placeholder="至少12位，包含至少三类字符" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="passwordDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handlePasswordSubmit">确定重置</el-button>
      </template>
    </el-dialog>

    <!-- 公告管理对话框 -->
    <el-dialog v-model="announcementDialogVisible" :title="isEditingAnnouncement ? '编辑公告' : '发布公告'" width="700px">
      <el-form :model="announcementForm" label-position="top" :rules="announcementRules" ref="announcementFormRef">
        <el-form-item label="公告标题" prop="title">
          <el-input v-model="announcementForm.title" placeholder="请输入公告标题" maxlength="100" show-word-limit />
        </el-form-item>
        <el-form-item label="公告内容" prop="content">
          <el-input
            v-model="announcementForm.content"
            type="textarea"
            :rows="6"
            placeholder="请输入公告内容"
            maxlength="2000"
            show-word-limit
          />
        </el-form-item>
        <el-form-item>
          <div class="form-options">
            <el-checkbox v-model="announcementForm.is_popup" label="弹窗显示" border />
            <el-checkbox v-model="announcementForm.is_top" label="置顶" border />
            <el-checkbox v-if="isEditingAnnouncement" v-model="announcementForm.is_published" label="发布状态" border />
          </div>
        </el-form-item>
        <el-form-item v-if="announcementForm.is_popup" class="tip-item">
          <el-alert
            title="弹窗公告说明"
            type="info"
            :closable="false"
            description="开启弹窗后，仅置顶且未读的弹窗公告会自动弹出一次；其他已发布公告会展示在首页公告列表中。"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="announcementDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="submitAnnouncementForm" :loading="announcementSubmitting">确定</el-button>
      </template>
    </el-dialog>

    <!-- 查看公告对话框 -->
    <el-dialog v-model="viewAnnouncementVisible" title="公告详情" width="600px" class="view-dialog">
      <div class="view-content">
        <h3 class="view-title">{{ currentAnnouncement?.title }}</h3>
        <div class="view-meta">
          <el-tag v-if="currentAnnouncement?.is_top" type="danger" size="small">置顶</el-tag>
          <el-tag v-if="currentAnnouncement?.is_popup" type="warning" size="small">弹窗</el-tag>
          <span class="view-time">{{ formatDate(currentAnnouncement?.created_at) }}</span>
        </div>
        <div class="view-body">{{ currentAnnouncement?.content }}</div>
      </div>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import '@/styles/element/admin'
import { ref, reactive, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  type Account,
  type AccountSummary,
  type AdminUser,
  type TaskConfig,
  getAllUsers,
  getAllAccounts,
  getAccountSummaries,
  getTaskConfigs,
  updateTaskConfig,
  updateUserRole,
  resetUserPassword,
  deleteUser,
  getStatsOverview
} from '../api/account'
import {
  type Announcement,
  type CreateAnnouncementRequest,
  type UpdateAnnouncementRequest,
  getAllAnnouncements,
  createAnnouncement,
  updateAnnouncement,
  deleteAnnouncement as apiDeleteAnnouncement
} from '../api/announcement'
import {
  type ExchangeConfig,
  getExchangeConfig,
  updateExchangeConfig,
  updateProducts as apiUpdateProducts,
  executeMonthlyExchange as apiExecuteMonthlyExchange
} from '../api/exchange'
import AdminSummaryList from '@/components/admin/AdminSummaryList.vue'
import AdminTaskConfigList from '@/components/admin/AdminTaskConfigList.vue'
import AdminExchangeConfig from '@/components/admin/AdminExchangeConfig.vue'
import AdminUserList from '@/components/admin/AdminUserList.vue'
import AdminStatsOverview from '@/components/admin/AdminStatsOverview.vue'
import AdminAnnouncementList from '@/components/admin/AdminAnnouncementList.vue'

const activeTab = ref('summaries')
const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const isMobile = computed(() => viewportWidth.value <= 768)

const syncViewport = () => {
  viewportWidth.value = window.innerWidth
}

// Account summaries
const summaryLoading = ref(false)
const summaryList = ref<AccountSummary[]>([])
const summaryPagination = reactive({ page: 1, pageSize: 20, total: 0 })

// Task configs
const taskConfigLoading = ref(false)
const taskConfigs = ref<TaskConfig[]>([])

// Exchange config
const exchangeConfig = reactive<ExchangeConfig>({
  auto_update_products: false,
  concurrency: 10,
  enabled: true,
  exchange_monthly_enabled: false,
  exchange_time: '10:00',
  monthly_prize_id: '1001',
  immediate_exchange_enabled: false
})
const updateProductsLoading = ref(false)
const monthlyExchangeLoading = ref(false)
const selectedAccountId = ref<number | null>(null)
const allAccounts = ref<Account[]>([])
const availableProductSourceAccounts = computed(() =>
  allAccounts.value.filter(account => account.is_active)
)

const patchExchangeConfig = (patch: Partial<ExchangeConfig>) => {
  Object.assign(exchangeConfig, patch)
}

// User management
const userLoading = ref(false)
const userList = ref<AdminUser[]>([])
const userPagination = reactive({ page: 1, pageSize: 10, total: 0 })

// Stats
const statsOverview = ref([
  { key: 'user_count', label: '用户总数', value: 0 as any, icon: 'User', color: 'linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%)' },
  { key: 'account_count', label: '账号总数', value: 0 as any, icon: 'Document', color: 'linear-gradient(135deg, #10b981 0%, #34d399 100%)' },
  { key: 'total_cloud', label: '总云朵数', value: 0 as any, icon: 'Cloudy', color: 'linear-gradient(135deg, #f59e0b 0%, #fbbf24 100%)' },
  { key: 'active_tasks', label: '活跃任务', value: 0 as any, icon: 'TrendCharts', color: 'linear-gradient(135deg, #ef4444 0%, #f87171 100%)' }
])

// Role dialog
const roleDialogVisible = ref(false)
const roleForm = reactive({ id: 0, role: 'user' })
const passwordDialogVisible = ref(false)
const passwordForm = reactive({ id: 0, username: '', password: '' })

// Announcements
const announcementLoading = ref(false)
const announcements = ref<Announcement[]>([])
const announcementDialogVisible = ref(false)
const viewAnnouncementVisible = ref(false)
const isEditingAnnouncement = ref(false)
const announcementSubmitting = ref(false)
const currentAnnouncement = ref<Announcement | null>(null)
const announcementFormRef = ref()
const announcementForm = reactive({
  id: 0,
  title: '',
  content: '',
  is_popup: false,
  is_top: false,
  is_published: true
})
const announcementRules = {
  title: [{ required: true, message: '请输入公告标题', trigger: 'blur' }],
  content: [{ required: true, message: '请输入公告内容', trigger: 'blur' }]
}

const handleTabChange = (tab: string) => {
  if (tab === 'summaries') loadSummaries()
  else if (tab === 'tasks') loadTaskConfigs()
  else if (tab === 'exchange') loadExchangeConfig()
  else if (tab === 'users') loadUserList()
  else if (tab === 'stats') loadStatsOverview()
  else if (tab === 'announcements') loadAnnouncements()
}

// Load account summaries
const loadSummaries = async () => {
  summaryLoading.value = true
  try {
    const data = await getAccountSummaries(summaryPagination.page, summaryPagination.pageSize)
    summaryList.value = data.summaries || []
    summaryPagination.total = data.total
  } catch { ElMessage.error('加载账号概况失败') }
  finally { summaryLoading.value = false }
}

const handleSummarySizeChange = (size: number) => {
  summaryPagination.pageSize = size
  summaryPagination.page = 1
  loadSummaries()
}

const handleSummaryPageChange = (page: number) => {
  summaryPagination.page = page
  loadSummaries()
}

// Load task configs
const loadTaskConfigs = async () => {
  taskConfigLoading.value = true
  try {
    const data = await getTaskConfigs()
    taskConfigs.value = data.configs || []
  } catch { ElMessage.error('加载任务配置失败') }
  finally { taskConfigLoading.value = false }
}

const handleTaskConfigToggle = async (row: TaskConfig, enabled: boolean) => {
  const previous = row.is_enabled
  row.is_enabled = enabled
  try {
    await updateTaskConfig(row.task_type, row.is_enabled)
    ElMessage.success(row.is_enabled ? '任务已上架' : '任务已下架')
  } catch {
    row.is_enabled = previous
    ElMessage.error('操作失败')
  }
}

// Load exchange config
const loadExchangeConfig = async () => {
  try {
    const data = await getExchangeConfig()
    exchangeConfig.auto_update_products = data.auto_update_products
    exchangeConfig.concurrency = data.concurrency
    exchangeConfig.enabled = data.enabled
    exchangeConfig.exchange_monthly_enabled = data.exchange_monthly_enabled || false
    exchangeConfig.exchange_time = data.exchange_time || '10:00'
    exchangeConfig.monthly_prize_id = data.monthly_prize_id || '1001'
    exchangeConfig.immediate_exchange_enabled = data.immediate_exchange_enabled || false
    
    // 加载所有账号用于商品更新
    const accountsData = await getAllAccounts(1, 1000)
    allAccounts.value = accountsData.accounts || []

    const hasSelectedAvailableAccount = availableProductSourceAccounts.value.some(
      account => account.id === selectedAccountId.value
    )
    if (!hasSelectedAvailableAccount) {
      selectedAccountId.value = availableProductSourceAccounts.value[0]?.id ?? null
    }
  } catch (error: any) {
    ElMessage.error('加载抢兑配置失败：' + error.message)
  }
}

// Save exchange config
const saveExchangeConfig = async () => {
  try {
    await updateExchangeConfig({
      auto_update_products: exchangeConfig.auto_update_products,
      concurrency: exchangeConfig.concurrency,
      enabled: exchangeConfig.enabled,
      exchange_monthly_enabled: exchangeConfig.exchange_monthly_enabled,
      exchange_time: exchangeConfig.exchange_time,
      monthly_prize_id: exchangeConfig.monthly_prize_id,
      immediate_exchange_enabled: exchangeConfig.immediate_exchange_enabled
    })
    ElMessage.success('保存配置成功')
  } catch (error: any) {
    ElMessage.error('保存配置失败：' + error.message)
  }
}

// Update products
const handleUpdateProducts = async () => {
  if (!selectedAccountId.value) {
    ElMessage.warning('请先选择一个云盘账号')
    return
  }
  updateProductsLoading.value = true
  try {
    await apiUpdateProducts(selectedAccountId.value)
    ElMessage.success('商品数据更新成功')
  } catch (error: any) {
    ElMessage.error('更新商品数据失败：' + error.message)
  } finally {
    updateProductsLoading.value = false
  }
}

// Execute monthly exchange
const executeMonthlyExchange = async () => {
  monthlyExchangeLoading.value = true
  try {
    await apiExecuteMonthlyExchange()
    ElMessage.success('已开始执行兑换月卡任务')
  } catch (error: any) {
    ElMessage.error('执行兑换月卡失败：' + error.message)
  } finally {
    monthlyExchangeLoading.value = false
  }
}

// Load users
const loadUserList = async () => {
  userLoading.value = true
  try {
    const data = await getAllUsers(userPagination.page, userPagination.pageSize)
    userList.value = data.users
    userPagination.total = data.total
  } catch { ElMessage.error('加载用户列表失败') }
  finally { userLoading.value = false }
}

const handleUserSizeChange = (size: number) => {
  userPagination.pageSize = size
  userPagination.page = 1
  loadUserList()
}

const handleUserPageChange = (page: number) => {
  userPagination.page = page
  loadUserList()
}

const handleEditUserRole = (row: AdminUser) => {
  roleForm.id = row.id
  roleForm.role = row.role
  roleDialogVisible.value = true
}

const handleRoleSubmit = async () => {
  try {
    await updateUserRole(roleForm.id, roleForm.role)
    ElMessage.success('角色修改成功')
    roleDialogVisible.value = false
    loadUserList()
  } catch { ElMessage.error('角色修改失败') }
}

const handleResetUserPassword = (row: AdminUser) => {
  passwordForm.id = row.id
  passwordForm.username = row.username
  passwordForm.password = ''
  passwordDialogVisible.value = true
}

const handlePasswordSubmit = async () => {
  if (passwordForm.password.length < 12) {
    ElMessage.warning('新密码长度不能少于12个字符')
    return
  }
  try {
    await resetUserPassword(passwordForm.id, passwordForm.password)
    ElMessage.success('密码重置成功')
    passwordDialogVisible.value = false
  } catch (error: any) {
    ElMessage.error(error.response?.data?.message || '密码重置失败')
  }
}

const handleDeleteUser = async (row: AdminUser) => {
  try {
    await ElMessageBox.confirm('确定要删除该用户吗？', '提示', { type: 'warning' })
    await deleteUser(row.id)
    ElMessage.success('删除成功')
    loadUserList()
  } catch (e: any) { if (e !== 'cancel') ElMessage.error('删除失败') }
}

// Load stats
const loadStatsOverview = async () => {
  try {
    const data = await getStatsOverview()
    statsOverview.value[0].value = data.user_count
    statsOverview.value[1].value = data.account_count
    statsOverview.value[2].value = data.total_cloud
    statsOverview.value[3].value = data.active_tasks
  } catch { console.error('加载统计概览失败') }
}

const formatDate = (date: string | undefined) => {
  if (!date) return ''
  return new Date(date).toLocaleString('zh-CN')
}

// Announcement methods
const loadAnnouncements = async () => {
  announcementLoading.value = true
  try {
    const res: any = await getAllAnnouncements()
    announcements.value = res.announcements || []
  } catch (error: any) {
    ElMessage.error('加载公告失败：' + error.message)
  } finally {
    announcementLoading.value = false
  }
}

const showAddAnnouncementDialog = () => {
  isEditingAnnouncement.value = false
  announcementForm.id = 0
  announcementForm.title = ''
  announcementForm.content = ''
  announcementForm.is_popup = false
  announcementForm.is_top = false
  announcementForm.is_published = true
  announcementDialogVisible.value = true
}

const editAnnouncement = (row: Announcement) => {
  isEditingAnnouncement.value = true
  announcementForm.id = row.id
  announcementForm.title = row.title
  announcementForm.content = row.content
  announcementForm.is_popup = row.is_popup
  announcementForm.is_top = row.is_top
  announcementForm.is_published = row.is_published
  announcementDialogVisible.value = true
}

const viewAnnouncement = (row: Announcement) => {
  currentAnnouncement.value = row
  viewAnnouncementVisible.value = true
}

const submitAnnouncementForm = async () => {
  const valid = await announcementFormRef.value?.validate().catch(() => false)
  if (!valid) return

  announcementSubmitting.value = true
  try {
    if (isEditingAnnouncement.value) {
      const data: UpdateAnnouncementRequest = {
        title: announcementForm.title,
        content: announcementForm.content,
        is_popup: announcementForm.is_popup,
        is_top: announcementForm.is_top,
        is_published: announcementForm.is_published
      }
      await updateAnnouncement(announcementForm.id, data)
      ElMessage.success('更新成功')
    } else {
      const data: CreateAnnouncementRequest = {
        title: announcementForm.title,
        content: announcementForm.content,
        is_popup: announcementForm.is_popup,
        is_top: announcementForm.is_top
      }
      await createAnnouncement(data)
      ElMessage.success('发布成功')
    }
    announcementDialogVisible.value = false
    loadAnnouncements()
  } catch (error: any) {
    ElMessage.error(isEditingAnnouncement.value ? '更新失败：' : '发布失败：' + error.message)
  } finally {
    announcementSubmitting.value = false
  }
}

const deleteAnnouncement = async (row: Announcement) => {
  try {
    await ElMessageBox.confirm('确定要删除这条公告吗？', '确认删除', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })
    await apiDeleteAnnouncement(row.id)
    ElMessage.success('删除成功')
    loadAnnouncements()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

onMounted(() => {
  syncViewport()
  window.addEventListener('resize', syncViewport)
  loadSummaries()
})

onUnmounted(() => {
  window.removeEventListener('resize', syncViewport)
})
</script>

<style scoped>
.admin-panel-container { padding: clamp(12px, 2vw, 24px); max-width: 1680px; margin: 0 auto; }
.admin-shell { overflow: hidden; }
:deep(.el-card) { border-radius: 24px; box-shadow: 0 16px 42px rgba(37, 99, 235, 0.1); border: 1px solid rgba(255,255,255,.74); background: rgba(255,255,255,.88); backdrop-filter: blur(14px); }
.card-header { display:flex; justify-content:space-between; align-items:center; gap:12px; font-size:clamp(20px,2.4vw,26px); font-weight:700; color:#0f172a; }
.admin-tabs { margin-top: 6px; }
.admin-tabs :deep(.el-tabs__header) { margin:0; padding-bottom:10px; }
.admin-tabs :deep(.el-tabs__nav-wrap) { overflow-x:auto; scrollbar-width:none; }
.admin-tabs :deep(.el-tabs__nav-wrap::-webkit-scrollbar) { display:none; }
.admin-tabs :deep(.el-tabs__nav-scroll) { display:flex; }
.admin-tabs :deep(.el-tabs__nav) { flex-wrap:nowrap; }
.admin-tabs :deep(.el-tabs__item) { height:44px; padding:0 18px; font-size:15px; font-weight:600; white-space:nowrap; }
.admin-tabs :deep(.el-tabs__item.is-active) { color:#2563eb; }
.admin-tabs :deep(.el-tabs__active-bar) { background: linear-gradient(90deg, #2563eb, #0ea5e9); }
.tab-content { padding: 22px 0 0; display:flex; flex-direction:column; gap:20px; }
.tab-content > p { margin:0; line-height:1.6; }
.form-options { display:flex; gap:14px; flex-wrap:wrap; }
.tip-item { margin-bottom:0; }
.view-content { padding:10px 4px 4px; }
.view-title { font-size:20px; font-weight:700; color:#0f172a; margin:0 0 16px; line-height:1.45; }
.view-meta { display:flex; align-items:center; gap:10px; flex-wrap:wrap; margin-bottom:20px; padding-bottom:14px; border-bottom:1px solid rgba(148,163,184,.2); }
.view-time { color:#64748b; font-size:13px; }
.view-body { font-size:14px; line-height:1.8; color:#475569; white-space:pre-wrap; }
:deep(.el-table) { border-radius:18px; overflow:hidden; --el-table-border-color: rgba(148,163,184,.18); --el-table-header-bg-color: rgba(248,250,252,.9); --el-table-row-hover-bg-color: rgba(239,246,255,.72); }
:deep(.el-table .cell) { line-height:1.45; }
:deep(.el-table th.el-table__cell) { color:#475569; font-size:13px; font-weight:700; }
:deep(.el-table td.el-table__cell) { color:#334155; }
:deep(.el-pagination) { justify-content:flex-end; flex-wrap:wrap; gap:8px; }
:deep(.el-dialog) { max-width: calc(100vw - 32px); border-radius:22px; }
@media (max-width: 1280px) {
  .admin-panel-container { padding:14px; }
  .tab-content { gap:16px; padding-top:18px; }
  :deep(.el-col-12) { width:100%!important; max-width:100%!important; flex:0 0 100%!important; }
  :deep(.el-col-6) { width:50%!important; max-width:50%!important; flex:0 0 50%!important; }
}
@media (max-width: 768px) {
  .admin-panel-container { padding:0; }
  .card-header { font-size:20px; }
  .admin-tabs :deep(.el-tabs__item) { height:40px; padding:0 14px; font-size:13px; }
  .tab-content { padding-top:16px; gap:14px; }
  .form-options { flex-direction:column; gap:10px; }
  .view-title { font-size:18px; }
  :deep(.el-col-6) { width:50%!important; max-width:50%!important; flex:0 0 50%!important; }
  :deep(.el-pagination) { justify-content:center; }
}
@media (max-width: 520px) {
  :deep(.el-col-6) { width:100%!important; max-width:100%!important; flex:0 0 100%!important; }
}
</style>
