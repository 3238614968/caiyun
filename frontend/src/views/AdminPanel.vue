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
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
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
  monthly_prize_id: '1001'
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

const handleTabChange = (tab: string) => {
  if (tab === 'summaries') loadSummaries()
  else if (tab === 'tasks') loadTaskConfigs()
  else if (tab === 'exchange') loadExchangeConfig()
  else if (tab === 'users') loadUserList()
  else if (tab === 'stats') loadStatsOverview()
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
    
    // 加载所有账号用于商品更新
    const accountsData = await getAllAccounts(1, 1000)
    allAccounts.value = accountsData.accounts || []
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
      monthly_prize_id: exchangeConfig.monthly_prize_id
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

const formatDate = (date: string) => {
  if (!date) return '-'
  return new Date(date).toLocaleString('zh-CN')
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
</style>
