<template>
  <div class="exchange-center">
    <!-- 页面标题 -->
    <page-header 
      title="兑换中心" 
      subtitle="使用云朵兑换心仪商品"
    />

    <div class="content">
      <!-- 统计卡片 -->
      <el-row :gutter="20" class="mb-4">
        <el-col :span="6">
          <stat-card title="我的兑换账号" :value="accounts.length" icon="Account" color="#409EFF" />
        </el-col>
        <el-col :span="6">
          <stat-card title="抢兑任务数" :value="tasks.length" icon="Document" color="#67C23A" />
        </el-col>
        <el-col :span="6">
          <stat-card title="成功次数" :value="totalSuccess" icon="Success" color="#E6A23C" />
        </el-col>
        <el-col :span="6">
          <stat-card title="失败次数" :value="totalFail" icon="Error" color="#F56C6C" />
        </el-col>
      </el-row>

      <!-- 选项卡 -->
      <el-tabs v-model="activeTab" type="border-card">
        <!-- 商品列表 -->
        <el-tab-pane label="商品中心" name="products">
          <div class="product-section">
            <!-- 搜索栏 -->
            <el-input
              v-model="searchKeyword"
              placeholder="搜索商品名称..."
              clearable
              prefix-icon="Search"
              style="width: 300px; margin-bottom: 20px;"
              @change="handleSearch"
            />
            
            <!-- 商品分类 -->
            <el-tag
              v-for="cat in categories"
              :key="cat"
              :type="currentCategory === cat ? 'primary' : 'info'"
              style="margin-right: 10px; margin-bottom: 10px; cursor: pointer;"
              @click="currentCategory = cat"
            >
              {{ cat }}
            </el-tag>

            <!-- 商品列表 -->
            <el-row :gutter="20" style="margin-top: 20px;">
              <el-col :span="8" v-for="product in filteredProducts" :key="product.id">
                <el-card shadow="hover" class="product-card">
                  <template #header>
                    <div class="product-header">
                      <span class="product-name">{{ product.prize_name }}</span>
                      <el-tag size="small" type="warning">{{ product.p_order }}云朵</el-tag>
                    </div>
                  </template>
                  <div class="product-info">
                    <p class="product-category">分类：{{ product.category }}</p>
                    <p class="product-stock">剩余：{{ product.daily_remainder_count }}</p>
                  </div>
                  <el-button 
                    type="primary" 
                    size="small" 
                    style="width: 100%;"
                    @click="showCreateTaskDialog(product)"
                  >
                    立即抢兑
                  </el-button>
                </el-card>
              </el-col>
            </el-row>
          </div>
        </el-tab-pane>

        <!-- 兑换账号管理 -->
        <el-tab-pane label="兑换账号" name="accounts">
          <div class="account-section">
            <el-button type="primary" icon="Plus" @click="showAddAccountDialog">添加兑换账号</el-button>
            
            <el-table :data="accounts" style="margin-top: 20px;" border>
              <el-table-column prop="remark" label="备注" />
              <el-table-column prop="phone" label="手机号" />
              <el-table-column prop="exchange_time_1" label="第一次抢兑时间" />
              <el-table-column prop="exchange_time_2" label="第二次抢兑时间" />
              <el-table-column label="状态">
                <template #default="{ row }">
                  <el-tag :type="row.is_active ? 'success' : 'danger'">
                    {{ row.is_active ? '启用' : '禁用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="200">
                <template #default="{ row }">
                  <el-button size="small" @click="editAccount(row)">编辑</el-button>
                  <el-button size="small" type="danger" @click="deleteAccount(row.id)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>

        <!-- 抢兑任务管理 -->
        <el-tab-pane label="抢兑任务" name="tasks">
          <div class="task-section">
            <el-table :data="tasks" style="margin-top: 20px;" border>
              <el-table-column prop="prize_name" label="商品名称" />
              <el-table-column label="兑换账号">
                <template #default="{ row }">
                  {{ row.exchange_account?.remark || '账号' + row.exchange_account_id }}
                </template>
              </el-table-column>
              <el-table-column label="任务类型">
                <template #default="{ row }">
                  <el-tag :type="row.task_type === 'long_term' ? 'warning' : 'primary'">
                    {{ row.task_type === 'long_term' ? '长期抢兑' : '固定次数' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="进度">
                <template #default="{ row }">
                  {{ row.attempted_count }} / {{ row.max_attempts }}
                </template>
              </el-table-column>
              <el-table-column label="状态">
                <template #default="{ row }">
                  <el-tag :type="getStatusType(row.status)">
                    {{ getStatusText(row.status) }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="200">
                <template #default="{ row }">
                  <el-button size="small" @click="executeTask(row.id)" :disabled="row.status !== 'pending'">立即抢兑</el-button>
                  <el-button size="small" type="danger" @click="deleteTask(row.id)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
        </el-tab-pane>
      </el-tabs>
    </div>

    <!-- 创建抢兑任务对话框 -->
    <el-dialog v-model="taskDialogVisible" title="创建抢兑任务" width="500px">
      <el-form :model="taskForm" label-width="120px">
        <el-form-item label="商品名称">
          <span>{{ selectedProduct?.prize_name }}</span>
        </el-form-item>
        <el-form-item label="云朵价格">
          <span>{{ selectedProduct?.p_order }}云朵</span>
        </el-form-item>
        <el-form-item label="兑换账号" required>
          <el-select v-model="taskForm.exchange_account_id" placeholder="请选择兑换账号" style="width: 100%;">
            <el-option
              v-for="acc in accounts"
              :key="acc.id"
              :label="acc.remark || acc.phone"
              :value="acc.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="任务类型" required>
          <el-radio-group v-model="taskForm.task_type">
            <el-radio label="fixed">固定次数</el-radio>
            <el-radio label="long_term">长期抢兑</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="最大次数" v-if="taskForm.task_type === 'fixed'">
          <el-input-number v-model="taskForm.max_attempts" :min="1" :max="100" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="taskDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="createTask">确定</el-button>
      </template>
    </el-dialog>

    <!-- 兑换账号对话框（新增/编辑） -->
    <el-dialog v-model="accountDialogVisible" :title="isEditingAccount ? '编辑兑换账号' : '添加兑换账号'" width="600px">
      <el-form :model="accountForm" label-width="120px">
        <el-form-item label="云盘账号" required>
          <el-select v-model="accountForm.account_id" placeholder="请选择云盘账号" style="width: 100%;" :disabled="isEditingAccount">
            <el-option
              v-for="acc in userAccounts"
              :key="acc.id"
              :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
              :value="acc.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="选择商品" required>
          <el-select v-model="accountForm.product_id" placeholder="请选择要兑换的商品" style="width: 100%;" filterable :disabled="isEditingAccount">
            <el-option
              v-for="product in products"
              :key="product.id"
              :label="`${product.prize_name} (${product.p_order}云朵)`"
              :value="product.id"
            />
          </el-select>
          <span style="font-size: 12px; color: #999;">选择该账号要抢兑的商品</span>
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="accountForm.remark" placeholder="可选" />
        </el-form-item>
        <el-form-item label="第一次抢兑时间" required>
          <el-time-picker
            v-model="accountForm.exchange_time_1"
            format="HH:mm:ss"
            value-format="HH:mm:ss"
            placeholder="选择时间"
            style="width: 100%;"
          />
        </el-form-item>
        <el-form-item label="第二次抢兑时间" required>
          <el-time-picker
            v-model="accountForm.exchange_time_2"
            format="HH:mm:ss"
            value-format="HH:mm:ss"
            placeholder="选择时间"
            style="width: 100%;"
          />
        </el-form-item>
        <el-form-item label="状态" v-if="isEditingAccount">
          <el-switch
            v-model="accountForm.is_active"
            active-text="启用"
            inactive-text="禁用"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="accountDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveAccount">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import PageHeader from '@/components/PageHeader.vue'
import StatCard from '@/components/StatCard.vue'
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
  executeExchangeTask
} from '@/api/exchange'
import { getAccounts } from '@/api/account'

// 状态
const activeTab = ref('products')
const searchKeyword = ref('')
const currentCategory = ref('')
const categories = ref<string[]>([])
const products = ref<any[]>([])
const accounts = ref<any[]>([])
const tasks = ref<any[]>([])
const userAccounts = ref<any[]>([])

// 对话框
const taskDialogVisible = ref(false)
const accountDialogVisible = ref(false)
const editingAccountId = ref<number | null>(null)

// 表单
const selectedProduct = ref<any>(null)
const taskForm = ref({
  exchange_account_id: 0,
  product_id: 0,
  task_type: 'fixed',
  max_attempts: 1
})

const accountForm = ref({
  account_id: 0,
  product_id: 0,
  remark: '',
  exchange_time_1: '10:00:00',
  exchange_time_2: '16:00:00',
  is_active: true
})

// 计算属性
const filteredProducts = computed(() => {
  if (!currentCategory.value) return products.value
  return products.value.filter(p => p.category === currentCategory.value)
})
const isEditingAccount = computed(() => editingAccountId.value !== null)

const totalSuccess = computed(() => {
  return tasks.value.reduce((sum, task) => sum + (task.success_count || 0), 0)
})

const totalFail = computed(() => {
  return tasks.value.reduce((sum, task) => sum + (task.fail_count || 0), 0)
})

// 方法
const loadProducts = async () => {
  try {
    const res = await searchProducts('', 100)
    products.value = res.products || []
  } catch (error: any) {
    ElMessage.error('加载商品失败：' + error.message)
  }
}

const loadCategories = async () => {
  try {
    const res = await getProductCategories()
    categories.value = res.categories || []
    if (categories.value.length > 0) {
      currentCategory.value = categories.value[0]
    }
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

const loadUserAccounts = async () => {
  try {
    const res = await getAccounts()
    userAccounts.value = res.accounts || []
  } catch (error: any) {
    ElMessage.error('加载云盘账号失败：' + error.message)
  }
}

const handleSearch = () => {
  searchProducts(searchKeyword.value, 20).then(res => {
    products.value = res.products || []
  })
}

const showCreateTaskDialog = (product: any) => {
  selectedProduct.value = product
  taskForm.value.product_id = product.id
  taskForm.value.exchange_account_id = accounts.value[0]?.id || 0
  taskDialogVisible.value = true
}

const showAddAccountDialog = () => {
  editingAccountId.value = null
  accountForm.value = {
    account_id: 0,
    product_id: 0,
    remark: '',
    exchange_time_1: '10:00:00',
    exchange_time_2: '16:00:00',
    is_active: true
  }
  accountDialogVisible.value = true
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

const saveAccount = async () => {
  try {
    if (isEditingAccount.value) {
      await updateExchangeAccount(editingAccountId.value!, {
        remark: accountForm.value.remark,
        exchange_time_1: accountForm.value.exchange_time_1,
        exchange_time_2: accountForm.value.exchange_time_2,
        is_active: accountForm.value.is_active
      })
      ElMessage.success('更新账号成功')
    } else {
      if (!accountForm.value.account_id) {
        ElMessage.error('请选择云盘账号')
        return
      }
      if (!accountForm.value.product_id) {
        ElMessage.error('请选择要兑换的商品')
        return
      }
      await addExchangeAccount(accountForm.value)
      ElMessage.success('添加账号成功')
    }
    accountDialogVisible.value = false
    editingAccountId.value = null
    loadAccounts()
  } catch (error: any) {
    ElMessage.error((isEditingAccount.value ? '更新账号失败：' : '添加账号失败：') + error.message)
  }
}

const editAccount = (account: any) => {
  editingAccountId.value = account.id
  accountForm.value = {
    account_id: account.account_id || 0,
    product_id: account.product_id || 0,
    remark: account.remark || '',
    exchange_time_1: account.exchange_time_1 || '10:00:00',
    exchange_time_2: account.exchange_time_2 || '16:00:00',
    is_active: account.is_active ?? true
  }
  accountDialogVisible.value = true
}

const deleteAccount = async (id: number) => {
  try {
    await ElMessageBox.confirm('确定要删除该兑换账号吗？', '提示', {
      type: 'warning'
    })
    await deleteExchangeAccount(id)
    ElMessage.success('删除成功')
    loadAccounts()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败：' + error.message)
    }
  }
}

const deleteTask = async (id: number) => {
  try {
    await ElMessageBox.confirm('确定要删除该抢兑任务吗？', '提示', {
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
    ElMessage.success('已开始执行抢兑任务')
    loadTasks()
  } catch (error: any) {
    ElMessage.error('执行失败：' + error.message)
  }
}

const getStatusType = (status: string) => {
  const types: any = {
    pending: 'info',
    running: 'warning',
    completed: 'success',
    cancelled: 'danger',
    failed: 'danger'
  }
  return types[status] || 'info'
}

const getStatusText = (status: string) => {
  const texts: any = {
    pending: '待执行',
    running: '进行中',
    completed: '已完成',
    cancelled: '已取消',
    failed: '失败'
  }
  return texts[status] || status
}

// 生命周期
onMounted(() => {
  loadProducts()
  loadCategories()
  loadAccounts()
  loadTasks()
  loadUserAccounts()
})
</script>

<style scoped>
.exchange-center {
  padding: 20px;
}

.content {
  background: #fff;
  border-radius: 4px;
  padding: 20px;
}

.product-card {
  margin-bottom: 20px;
}

.product-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.product-name {
  font-weight: bold;
  font-size: 14px;
}

.product-info {
  padding: 10px 0;
}

.product-category,
.product-stock {
  margin: 5px 0;
  font-size: 12px;
  color: #666;
}

.mb-4 {
  margin-bottom: 20px;
}
</style>
