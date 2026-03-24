<template>
  <div class="admin-panel-container">
    <el-card shadow="hover">
      <template #header>
        <div class="card-header">
          <span>管理员面板</span>
        </div>
      </template>

      <el-tabs v-model="activeTab" @tab-change="handleTabChange">
        <!-- 账号概况 -->
        <el-tab-pane label="账号概况" name="summaries">
          <div class="tab-content">
            <el-table :data="summaryList" stripe v-loading="summaryLoading" style="width: 100%">
              <el-table-column prop="phone" label="手机号" width="130" />
              <el-table-column prop="owner_username" label="所属用户" width="100" />
              <el-table-column prop="cloud_count" label="当前云朵" width="100">
                <template #default="{ row }">
                  <span style="font-weight: 600; color: #3b82f6">{{ row.cloud_count }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="today_gained" label="今日获得" width="100">
                <template #default="{ row }">
                  <span v-if="row.today_gained > 0" style="color: #10b981">+{{ row.today_gained }}</span>
                  <span v-else>0</span>
                </template>
              </el-table-column>
              <el-table-column prop="yesterday_gained" label="昨日获得" width="100">
                <template #default="{ row }">
                  <span v-if="row.yesterday_gained > 0" style="color: #10b981">+{{ row.yesterday_gained }}</span>
                  <span v-else>0</span>
                </template>
              </el-table-column>
              <el-table-column label="今日任务" width="140">
                <template #default="{ row }">
                  <el-tag type="success" size="small">成功 {{ row.success_count }}</el-tag>
                  <el-tag v-if="row.failed_count > 0" type="danger" size="small" style="margin-left: 4px">
                    失败 {{ row.failed_count }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="80">
                <template #default="{ row }">
                  <el-tag :type="row.is_active ? 'success' : 'info'" size="small">
                    {{ row.is_active ? '激活' : '停用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="last_executed_at" label="最后执行" width="160">
                <template #default="{ row }">
                  {{ row.last_executed_at || '-' }}
                </template>
              </el-table-column>
              <el-table-column prop="remark" label="备注" />
              <el-table-column prop="created_at" label="添加时间" width="160" />
            </el-table>

            <el-pagination
              v-model:current-page="summaryPagination.page"
              v-model:page-size="summaryPagination.pageSize"
              :page-sizes="[20, 50, 100]"
              :total="summaryPagination.total"
              layout="total, sizes, prev, pager, next"
              @size-change="(s: number) => { summaryPagination.pageSize = s; loadSummaries() }"
              @current-change="(p: number) => { summaryPagination.page = p; loadSummaries() }"
              style="margin-top: 20px"
            />
          </div>
        </el-tab-pane>

        <!-- 任务管理 -->
        <el-tab-pane label="任务管理" name="tasks">
          <div class="tab-content">
            <p style="color: #666; margin-bottom: 16px">
              下架的任务将不会被手动执行和定时任务执行。
            </p>
            <el-table :data="taskConfigs" stripe v-loading="taskConfigLoading" style="width: 100%">
              <el-table-column prop="sort_order" label="序号" width="70" />
              <el-table-column prop="task_name" label="任务名称" width="120" />
              <el-table-column prop="task_type" label="任务标识" width="140">
                <template #default="{ row }">
                  <el-tag size="small">{{ row.task_type }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="状态" width="160">
                <template #default="{ row }">
                  <div class="task-status-cell">
                    <el-switch
                      v-model="row.is_enabled"
                      @change="handleTaskConfigChange(row)"
                    />
                    <span :class="row.is_enabled ? 'status-on' : 'status-off'">
                      {{ row.is_enabled ? '已上架' : '已下架' }}
                    </span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="updated_at" label="更新时间" min-width="180">
                <template #default="{ row }">
                  {{ formatDate(row.updated_at) }}
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <!-- 抢兑配置 -->
        <el-tab-pane label="抢兑配置" name="exchange">
          <div class="tab-content">
            <el-row :gutter="20">
              <!-- 抢兑基础配置 -->
              <el-col :span="12">
                <el-card shadow="hover" class="config-card">
                  <template #header>
                    <div class="config-header">
                      <span>基础配置</span>
                    </div>
                  </template>
                  <el-form :model="exchangeConfig" label-width="150px">
                    <el-form-item label="抢兑功能开关">
                      <el-switch v-model="exchangeConfig.enabled" />
                    </el-form-item>
                    <el-form-item label="自动更新商品库">
                      <el-switch v-model="exchangeConfig.auto_update_products" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">每天早上 8 点自动更新</span>
                    </el-form-item>
                    <el-form-item label="抢兑并发数" required>
                      <el-input-number v-model="exchangeConfig.concurrency" :min="1" :max="50" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">同时执行的抢兑任务数</span>
                    </el-form-item>
                    <el-form-item label="立即兑换功能">
                      <el-switch v-model="exchangeConfig.immediate_exchange_enabled" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">启用后用户可直接兑换，无需创建任务</span>
                    </el-form-item>
                    <el-form-item>
                      <el-button type="primary" @click="saveExchangeConfig">保存配置</el-button>
                    </el-form-item>
                  </el-form>
                </el-card>
              </el-col>

              <!-- 兑换月卡配置 -->
              <el-col :span="12">
                <el-card shadow="hover" class="config-card">
                  <template #header>
                    <div class="config-header">
                      <span>兑换月卡配置</span>
                      <el-tag v-if="exchangeConfig.exchange_monthly_enabled" type="success">已启用</el-tag>
                      <el-tag v-else type="info">已禁用</el-tag>
                    </div>
                  </template>
                  <el-form :model="exchangeConfig" label-width="150px">
                    <el-form-item label="兑换月卡开关">
                      <el-switch v-model="exchangeConfig.exchange_monthly_enabled" />
                      <span style="margin-left: 10px; font-size: 12px; color: #999;">启用后自动兑换月卡</span>
                    </el-form-item>
                    <el-form-item label="自动兑换时间">
                      <el-time-picker
                        v-model="exchangeConfig.exchange_time"
                        format="HH:mm"
                        value-format="HH:mm"
                        placeholder="选择时间"
                        style="width: 100%;"
                      />
                    </el-form-item>
                    <el-form-item label="月卡商品ID">
                      <el-input
                        v-model="exchangeConfig.monthly_prize_id"
                        placeholder="请输入月卡商品ID"
                        style="width: 100%;"
                      />
                      <span style="font-size: 12px; color: #999;">默认1001，可从商品中心查看</span>
                    </el-form-item>
                    <el-form-item>
                      <el-button type="primary" @click="saveExchangeConfig">保存配置</el-button>
                      <el-button type="success" @click="executeMonthlyExchange" :loading="monthlyExchangeLoading">
                        立即执行兑换
                      </el-button>
                    </el-form-item>
                  </el-form>
                </el-card>
              </el-col>
            </el-row>

            <!-- 商品中心管理 -->
            <el-card shadow="hover" class="config-card" style="margin-top: 20px;">
              <template #header>
                <div class="config-header">
                  <span>商品中心管理</span>
                </div>
              </template>
              <div class="product-management">
                <p style="color: #666; margin-bottom: 16px;">
                  手动更新商品中心数据，需要选择一个有效的云盘账号作为数据获取源。
                </p>
                <el-form :inline="true">
                  <el-form-item label="选择账号">
                    <el-select v-model="selectedAccountId" placeholder="请选择云盘账号" style="width: 250px;">
                      <el-option
                        v-for="acc in allAccounts"
                        :key="acc.id"
                        :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
                        :value="acc.id"
                      />
                    </el-select>
                  </el-form-item>
                  <el-form-item>
                    <el-button type="primary" @click="handleUpdateProducts" :loading="updateProductsLoading">
                      <el-icon><Refresh /></el-icon>
                      更新商品数据
                    </el-button>
                  </el-form-item>
                </el-form>
              </div>
            </el-card>
          </div>
        </el-tab-pane>

        <!-- 用户管理 -->
        <el-tab-pane label="用户管理" name="users">
          <div class="tab-content">
            <el-table :data="userList" stripe v-loading="userLoading" style="width: 100%">
              <el-table-column prop="username" label="用户名" width="150" />
              <el-table-column prop="email" label="邮箱" />
              <el-table-column prop="role" label="角色" width="120">
                <template #default="{ row }">
                  <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">
                    {{ row.role === 'admin' ? '管理员' : '普通用户' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" label="创建时间" width="180">
                <template #default="{ row }">
                  {{ formatDate(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="150" fixed="right">
                <template #default="{ row }">
                  <el-button type="primary" link @click="handleEditUserRole(row)">修改角色</el-button>
                  <el-button type="danger" link @click="handleDeleteUser(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
            <el-pagination
              v-model:current-page="userPagination.page"
              v-model:page-size="userPagination.pageSize"
              :page-sizes="[10, 20, 50]"
              :total="userPagination.total"
              layout="total, sizes, prev, pager, next"
              @size-change="(s: number) => { userPagination.pageSize = s; loadUserList() }"
              @current-change="(p: number) => { userPagination.page = p; loadUserList() }"
              style="margin-top: 20px"
            />
          </div>
        </el-tab-pane>

        <!-- 统计概览 -->
        <el-tab-pane label="统计概览" name="stats">
          <div class="tab-content">
            <el-row :gutter="20">
              <el-col :span="6" v-for="stat in statsOverview" :key="stat.key">
                <el-card shadow="hover" class="stat-card">
                  <div class="stat-content">
                    <div class="stat-icon" :style="{ background: stat.color }">
                      <el-icon><component :is="stat.icon" /></el-icon>
                    </div>
                    <div class="stat-info">
                      <div class="stat-value">{{ stat.value }}</div>
                      <div class="stat-label">{{ stat.label }}</div>
                    </div>
                  </div>
                </el-card>
              </el-col>
            </el-row>
          </div>
        </el-tab-pane>

        <!-- 公告管理 -->
        <el-tab-pane label="公告管理" name="announcements">
          <div class="tab-content">
            <div class="announcement-header">
              <el-button type="primary" @click="showAddAnnouncementDialog">
                <el-icon><Plus /></el-icon>
                发布公告
              </el-button>
            </div>
            <el-table :data="announcements" stripe v-loading="announcementLoading" style="width: 100%">
              <el-table-column type="index" width="50" />
              <el-table-column prop="title" label="标题" min-width="200">
                <template #default="{ row }">
                  <div class="title-cell">
                    <el-tag v-if="row.is_top" type="danger" size="small" effect="dark">置顶</el-tag>
                    <el-tag v-if="row.is_popup" type="warning" size="small" class="ml-2">弹窗</el-tag>
                    <span class="title-text">{{ row.title }}</span>
                  </div>
                </template>
              </el-table-column>
              <el-table-column prop="is_published" label="状态" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.is_published ? 'success' : 'info'">
                    {{ row.is_published ? '已发布' : '已下架' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column prop="created_at" label="创建时间" width="180">
                <template #default="{ row }">
                  {{ formatDate(row.created_at) }}
                </template>
              </el-table-column>
              <el-table-column label="操作" width="200" fixed="right">
                <template #default="{ row }">
                  <el-button size="small" @click="viewAnnouncement(row)">查看</el-button>
                  <el-button size="small" type="primary" @click="editAnnouncement(row)">编辑</el-button>
                  <el-button size="small" type="danger" @click="deleteAnnouncement(row)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
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
            description="开启弹窗后，用户登录后会自动弹出此公告。如有多个弹窗公告，默认只显示置顶的公告。"
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
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh, Plus } from '@element-plus/icons-vue'
import { type User } from '../api/auth'
import {
  type Account,
  type AccountSummary,
  type TaskConfig,
  getAllUsers,
  getAllAccounts,
  getAccountSummaries,
  getTaskConfigs,
  updateTaskConfig,
  updateUserRole,
  updateAccountStatus,
  deleteUser,
  deleteAdminAccount,
  getStatsOverview
} from '../api/account'
import {
  type Announcement,
  type CreateAnnouncementRequest,
  type UpdateAnnouncementRequest,
  getAllAnnouncements,
  createAnnouncement,
  updateAnnouncement,
  deleteAnnouncement
} from '../api/announcement'
import {
  type ExchangeConfig,
  getExchangeConfig,
  updateExchangeConfig,
  updateProducts as apiUpdateProducts,
  executeMonthlyExchange as apiExecuteMonthlyExchange
} from '../api/exchange'

const activeTab = ref('summaries')

// Account summaries
const summaryLoading = ref(false)
const summaryList = ref<AccountSummary[]>([])
const summaryPagination = reactive({ page: 1, pageSize: 20, total: 0 })

// Task configs
const taskConfigLoading = ref(false)
const taskConfigs = ref<TaskConfig[]>([])

// Exchange config
const exchangeConfig = reactive({
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

// User management
const userLoading = ref(false)
const userList = ref<User[]>([])
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

// Load task configs
const loadTaskConfigs = async () => {
  taskConfigLoading.value = true
  try {
    const data = await getTaskConfigs()
    taskConfigs.value = data.configs || []
  } catch { ElMessage.error('加载任务配置失败') }
  finally { taskConfigLoading.value = false }
}

const handleTaskConfigChange = async (row: TaskConfig) => {
  try {
    await updateTaskConfig(row.task_type, row.is_enabled)
    ElMessage.success(row.is_enabled ? '任务已上架' : '任务已下架')
  } catch {
    row.is_enabled = !row.is_enabled
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
    console.log('获取到的账号数据:', accountsData)
    allAccounts.value = accountsData.accounts || []
    console.log('加载账号数量:', allAccounts.value.length)
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
    userList.value = data.users as any[]
    userPagination.total = data.total
  } catch { ElMessage.error('加载用户列表失败') }
  finally { userLoading.value = false }
}

const handleEditUserRole = (row: User) => {
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

const handleDeleteUser = async (row: User) => {
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
    await deleteAnnouncement(row.id)
    ElMessage.success('删除成功')
    loadAnnouncements()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

onMounted(() => {
  loadSummaries()
})
</script>

<style scoped>
.admin-panel-container {
  padding: 20px;
  background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%);
  min-height: calc(100vh - 140px);
}
.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.tab-content {
  padding: 20px 0;
}
.config-card {
  border-radius: 12px;
}
.config-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-weight: 600;
}
.product-management {
  padding: 10px 0;
}
.stat-card {
  margin-bottom: 20px;
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
  background: rgba(255, 255, 255, 0.9);
}
.stat-content {
  display: flex;
  align-items: center;
}
.stat-icon {
  width: 50px;
  height: 50px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-right: 12px;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.1);
}
.stat-icon .el-icon {
  font-size: 28px;
  color: white;
}
.stat-info .stat-value {
  font-size: 24px;
  font-weight: bold;
  color: #333;
  margin-bottom: 4px;
}
.stat-info .stat-label {
  font-size: 14px;
  color: #666;
}
:deep(.el-tabs__item.is-active) {
  color: #3b82f6;
}
:deep(.el-tabs__active-bar) {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%);
}
.task-status-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}
.status-on {
  color: #10b981;
  font-size: 13px;
}
.status-off {
  color: #ef4444;
  font-size: 13px;
}
:deep(.el-card) {
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
  background: rgba(255, 255, 255, 0.9);
}

/* 移动端响应式优化 */
@media (max-width: 768px) {
  .admin-panel-container {
    padding: 12px;
    min-height: calc(100vh - 100px);
  }

  .tab-content {
    padding: 12px 0;
  }

  /* 配置卡片改为单列 */
  :deep(.el-col-12) {
    width: 100% !important;
    margin-bottom: 16px;
  }

  /* 统计卡片改为2列 */
  :deep(.el-col-6) {
    width: 50% !important;
    margin-bottom: 12px;
  }

  .stat-card {
    margin-bottom: 12px;
  }

  .stat-content {
    flex-direction: column;
    text-align: center;
    gap: 8px;
  }

  .stat-icon {
    margin-right: 0;
    width: 40px;
    height: 40px;
  }

  .stat-icon .el-icon {
    font-size: 22px;
  }

  .stat-info .stat-value {
    font-size: 20px;
  }

  .stat-info .stat-label {
    font-size: 12px;
  }

  /* 表格横向滚动 */
  :deep(.el-table) {
    font-size: 13px;
  }

  :deep(.el-table .cell) {
    padding: 8px 4px;
  }

  /* 分页器优化 */
  :deep(.el-pagination) {
    justify-content: center;
    flex-wrap: wrap;
    gap: 8px;
  }

  /* 表单项优化 */
  :deep(.el-form-item__label) {
    float: none;
    display: block;
    text-align: left;
    margin-bottom: 4px;
  }

  :deep(.el-form-item__content) {
    margin-left: 0 !important;
  }

  /* 按钮组优化 */
  .product-management :deep(.el-form--inline) {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .product-management :deep(.el-form-item) {
    margin-right: 0;
    margin-bottom: 0;
  }

  .product-management :deep(.el-form-item__content) {
    width: 100%;
  }

  .product-management :deep(.el-select) {
    width: 100% !important;
  }
}

/* 公告管理样式 */
.announcement-header {
  margin-bottom: 20px;
  display: flex;
  justify-content: flex-end;
}

.title-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.title-text {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ml-2 {
  margin-left: 8px;
}

.form-options {
  display: flex;
  gap: 16px;
}

.tip-item {
  margin-bottom: 0;
}

.view-content {
  padding: 10px;
}

.view-title {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
  margin: 0 0 16px 0;
  line-height: 1.4;
}

.view-meta {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 20px;
  padding-bottom: 16px;
  border-bottom: 1px solid #e4e7ed;
}

.view-time {
  color: #909399;
  font-size: 13px;
}

.view-body {
  font-size: 14px;
  line-height: 1.8;
  color: #606266;
  white-space: pre-wrap;
}
</style>
