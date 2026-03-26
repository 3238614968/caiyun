<template>
  <div class="exchange-center">
    <!-- 简洁页面头部 -->
    <div class="page-header">
      <div class="header-main">
        <div class="header-title-section">
          <el-icon :size="24" color="#2563eb"><Present /></el-icon>
          <div class="header-text">
            <span class="title">兑换中心</span>
            <span class="subtitle">使用云朵兑换心仪商品</span>
          </div>
        </div>
        <div class="header-actions">
          <el-button type="primary" :icon="Plus" @click="showAddAccountDialog" v-if="activeTab === 'accounts'">
            添加兑换账号
          </el-button>
        </div>
      </div>
      <!-- 统计卡片 -->
      <div class="stats-row">
        <div class="stat-item">
          <div class="stat-icon-bg blue">
            <el-icon><User /></el-icon>
          </div>
          <div class="stat-content">
            <span class="stat-num">{{ accounts.length }}</span>
            <span class="stat-name">兑换账号</span>
          </div>
        </div>
        <div class="stat-item">
          <div class="stat-icon-bg indigo">
            <el-icon><Timer /></el-icon>
          </div>
          <div class="stat-content">
            <span class="stat-num">{{ tasks.length }}</span>
            <span class="stat-name">抢兑任务</span>
          </div>
        </div>
        <div class="stat-item">
          <div class="stat-icon-bg success">
            <el-icon><CircleCheck /></el-icon>
          </div>
          <div class="stat-content">
            <span class="stat-num">{{ totalSuccess }}</span>
            <span class="stat-name">成功</span>
          </div>
        </div>
        <div class="stat-item">
          <div class="stat-icon-bg danger">
            <el-icon><CircleClose /></el-icon>
          </div>
          <div class="stat-content">
            <span class="stat-num">{{ totalFail }}</span>
            <span class="stat-name">失败</span>
          </div>
        </div>
      </div>
    </div>

    <div class="content">
      <!-- 选项卡 -->
      <el-tabs v-model="activeTab" type="border-card" class="exchange-tabs">
        <!-- 商品列表 -->
        <el-tab-pane label="商品中心" name="products">
          <div class="product-section">
            <!-- 搜索和分类区域 -->
            <div class="filter-area">
              <div class="filter-head">
                <div class="filter-copy">
                  <span class="filter-title">商品筛选</span>
                  <span class="filter-hint">当前展示 {{ filteredProducts.length }} 个商品</span>
                </div>
                <el-tag size="small" effect="plain" type="primary">{{ currentCategory || '全部分类' }}</el-tag>
              </div>

              <el-input
                v-model="searchKeyword"
                placeholder="搜索商品名称..."
                clearable
                prefix-icon="Search"
                class="search-input"
                @change="handleSearch"
              />
              
              <!-- 商品分类 - 移动端横向滚动 -->
              <div class="category-list" :class="{ 'mobile-scroll': isMobile }">
                <el-radio-group v-model="currentCategory" size="small">
                  <el-radio-button label="">全部</el-radio-button>
                  <el-radio-button 
                    v-for="cat in categories" 
                    :key="cat" 
                    :label="cat"
                  >
                    {{ cat }}
                  </el-radio-button>
                </el-radio-group>
              </div>
            </div>

            <!-- 商品列表 - 移动端单列，桌面端网格 -->
            <div class="product-grid" :class="{ 'mobile-grid': isMobile }">
              <el-empty v-if="filteredProducts.length === 0" description="暂无商品" />
              <el-card 
                v-for="product in filteredProducts" 
                :key="product.id"
                shadow="hover" 
                class="product-card"
                :class="{ 'sold-out': product.stock_status === 'sold_out' || product.daily_remainder_count === 0, 'mobile': isMobile }"
              >
                <div class="product-layout" :class="{ 'mobile': isMobile }">
                  <div class="product-image" :class="{ 'mobile': isMobile, 'has-image': getProductImageUrl(product) }">
                    <img 
                      v-if="getProductImageUrl(product)" 
                      :src="getProductImageUrl(product)" 
                      :alt="product.prize_name"
                      class="product-img"
                      @error="$event.target.style.display='none'"
                    />
                    <div class="product-image-fallback">
                      <el-icon :size="isMobile ? 36 : 48" color="#409EFF"><Present /></el-icon>
                    </div>
                  </div>
                  <div class="product-content" :class="{ 'mobile': isMobile }">
                    <div class="product-title">{{ product.prize_name }}</div>
                    <div class="product-meta" :class="{ 'mobile': isMobile }">
                      <el-tag size="small" effect="plain" type="info">{{ product.category }}</el-tag>
                      <span class="product-price">
                        <svg class="cloud-icon" viewBox="0 0 24 24" fill="currentColor" xmlns="http://www.w3.org/2000/svg">
                          <path d="M19.35 10.04C18.67 6.59 15.64 4 12 4 9.11 4 6.6 5.64 5.35 8.04 2.34 8.36 0 10.91 0 14c0 3.31 2.69 6 6 6h13c2.76 0 5-2.24 5-5 0-2.64-2.05-4.78-4.65-4.96z"/>
                        </svg>
                        <span class="price-value">{{ product.p_order }}</span>
                      </span>
                    </div>
                    <div class="product-stock">
                      <div class="stock-header">
                        <span class="stock-label">
                          <el-icon><Box /></el-icon>
                          库存
                        </span>
                        <span class="stock-value" :class="{ 'low': product.daily_remainder_count <= 10, 'empty': product.daily_remainder_count === 0 }">
                          {{ product.daily_remainder_count }}/{{ product.daily_count || product.daily_limit_count || '-' }}
                        </span>
                      </div>
                      <el-progress 
                        :percentage="getStockPercentage(product)" 
                        :status="getStockStatus(product)"
                        :stroke-width="isMobile ? 6 : 10"
                        :show-text="false"
                        class="stock-progress"
                      />
                    </div>
                    <el-button 
                      v-if="exchangeConfig?.immediate_exchange_enabled"
                      :type="product.stock_status === 'sold_out' || product.daily_remainder_count === 0 ? 'warning' : 'success'"
                      class="exchange-btn"
                      :size="isMobile ? 'small' : 'default'"
                      @click="product.stock_status === 'sold_out' || product.daily_remainder_count === 0 ? handleReserveProduct(product) : handleImmediateExchange(product)"
                    >
                      {{ product.stock_status === 'sold_out' || product.daily_remainder_count === 0 ? '预定' : '立即兑换' }}
                    </el-button>
                    <el-button 
                      v-else
                      :type="product.stock_status === 'sold_out' || product.daily_remainder_count === 0 ? 'warning' : 'primary'"
                      class="exchange-btn"
                      :size="isMobile ? 'small' : 'default'"
                      @click="product.stock_status === 'sold_out' || product.daily_remainder_count === 0 ? handleReserveProduct(product) : showCreateTaskDialog(product)"
                    >
                      {{ product.stock_status === 'sold_out' || product.daily_remainder_count === 0 ? '预定' : '立即抢兑' }}
                    </el-button>
                  </div>
                </div>
              </el-card>
            </div>
          </div>
        </el-tab-pane>

        <!-- 兑换账号管理 -->
        <el-tab-pane label="兑换账号" name="accounts">
          <div class="account-section">
            <div class="section-header">
              <el-button type="primary" :size="isMobile ? 'small' : 'default'" icon="Plus" @click="showAddAccountDialog">添加兑换账号</el-button>
            </div>
            
            <!-- 移动端卡片列表 -->
            <template v-if="isMobile">
              <div class="mobile-card-list">
                <el-card v-for="acc in accounts" :key="acc.id" class="mobile-account-card" shadow="hover">
                  <div class="mobile-account-header">
                    <span class="mobile-account-title">{{ acc.remark || '未命名账号' }}</span>
                    <el-tag :type="acc.is_active ? 'success' : 'danger'" size="small">
                      {{ acc.is_active ? '启用' : '禁用' }}
                    </el-tag>
                  </div>
                  <div class="mobile-account-info">
                    <div class="mobile-account-item">
                      <span class="label">手机号：</span>
                      <span class="value">{{ acc.phone }}</span>
                    </div>
                    <div class="mobile-account-item">
                      <span class="label">抢兑时间：</span>
                      <span class="value">{{ acc.exchange_time_1 }} / {{ acc.exchange_time_2 }}</span>
                    </div>
                  </div>
                  <div class="mobile-account-actions">
                    <el-button size="small" @click="editAccount(acc)">编辑</el-button>
                    <el-button size="small" type="danger" @click="deleteAccount(acc.id)">删除</el-button>
                  </div>
                </el-card>
              </div>
            </template>
            
            <!-- 桌面端表格 -->
            <el-table v-else :data="accounts" border stripe>
              <el-table-column prop="remark" label="备注" min-width="100" />
              <el-table-column prop="phone" label="手机号" min-width="110" />
              <el-table-column prop="exchange_time_1" label="第一次抢兑" width="100" />
              <el-table-column prop="exchange_time_2" label="第二次抢兑" width="100" />
              <el-table-column label="状态" width="70">
                <template #default="{ row }">
                  <el-tag :type="row.is_active ? 'success' : 'danger'" size="small">
                    {{ row.is_active ? '启用' : '禁用' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="150" fixed="right">
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
            <!-- 移动端卡片列表 -->
            <template v-if="isMobile">
              <div class="mobile-card-list">
                <el-card v-for="task in tasks" :key="task.id" class="mobile-task-card" shadow="hover">
                  <div class="mobile-task-header">
                    <span class="mobile-task-title">{{ task.prize_name }}</span>
                    <el-tag :type="task.task_type === 'long_term' ? 'warning' : 'primary'" size="small">
                      {{ task.task_type === 'long_term' ? '长期' : '固定' }}
                    </el-tag>
                  </div>
                  <div class="mobile-task-info">
                    <div class="mobile-task-item">
                      <span class="label">兑换账号：</span>
                      <span class="value">{{ task.exchange_account?.remark || '账号' + task.exchange_account_id }}</span>
                    </div>
                    <div class="mobile-task-item" v-if="task.last_result">
                      <span class="label">执行结果：</span>
                      <el-tag v-if="task.last_result.includes('成功')" type="success" size="small">成功</el-tag>
                      <el-tag v-else-if="task.last_result.includes('失败') || task.last_result.includes('错误')" type="danger" size="small">失败</el-tag>
                      <el-tag v-else type="info" size="small">{{ task.last_result.substring(0, 8) }}</el-tag>
                    </div>
                  </div>
                  <div class="mobile-task-actions">
                    <el-button 
                      size="small" 
                      type="primary" 
                      @click="executeTask(task.id)" 
                      :disabled="task.last_result && task.last_result.includes('成功')"
                    >
                      立即抢兑
                    </el-button>
                    <el-button size="small" type="danger" @click="deleteTask(task.id)">删除</el-button>
                  </div>
                </el-card>
              </div>
            </template>
            
            <!-- 桌面端表格 -->
            <el-table v-else :data="tasks" border stripe>
              <el-table-column prop="prize_name" label="商品名称" min-width="150" />
              <el-table-column label="兑换账号" min-width="120">
                <template #default="{ row }">
                  {{ row.exchange_account?.remark || '账号' + row.exchange_account_id }}
                </template>
              </el-table-column>
              <el-table-column label="任务类型" width="100">
                <template #default="{ row }">
                  <el-tag :type="row.task_type === 'long_term' ? 'warning' : 'primary'" size="small">
                    {{ row.task_type === 'long_term' ? '长期' : '固定' }}
                  </el-tag>
                </template>
              </el-table-column>
              <el-table-column label="执行结果" min-width="200">
                <template #default="{ row }">
                  <div v-if="row.last_result" class="task-result">
                    <el-tag v-if="row.last_result.includes('成功')" type="success" size="small">成功</el-tag>
                    <el-tag v-else-if="row.last_result.includes('失败') || row.last_result.includes('错误')" type="danger" size="small">失败</el-tag>
                    <el-tag v-else type="info" size="small">{{ row.last_result }}</el-tag>
                    <el-tooltip v-if="row.last_result.length > 10" :content="row.last_result" placement="top">
                      <span class="result-text">{{ row.last_result.substring(0, 10) }}...</span>
                    </el-tooltip>
                    <span v-else class="result-text">{{ row.last_result }}</span>
                  </div>
                  <span v-else class="text-gray">-</span>
                </template>
              </el-table-column>
              <el-table-column label="操作" width="180" fixed="right">
                <template #default="{ row }">
                  <el-button 
                    size="small" 
                    type="primary" 
                    @click="executeTask(row.id)" 
                    :disabled="row.last_result && row.last_result.includes('成功')"
                  >
                    立即抢兑
                  </el-button>
                  <el-button size="small" type="danger" @click="deleteTask(row.id)">删除</el-button>
                </template>
              </el-table-column>
            </el-table>
          </div>
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

    <!-- 创建抢兑任务对话框 -->
    <el-dialog v-model="taskDialogVisible" title="创建抢兑任务" :width="isMobile ? '90%' : '500px'">
      <el-form :model="taskForm" label-width="isMobile ? '100px' : '120px'">
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

    <!-- 立即兑换对话框 -->
    <el-dialog
      v-model="immediateExchangeDialogVisible"
      title="立即兑换"
      :width="isMobile ? '95%' : '480px'"
      :close-on-click-modal="false"
    >
      <el-form label-position="top">
        <el-form-item label="兑换商品">
          <el-input :model-value="selectedProduct?.prize_name" disabled />
        </el-form-item>
        <el-form-item label="选择账号" required>
          <el-select v-model="taskForm.exchange_account_id" placeholder="请选择兑换账号" style="width: 100%;">
            <el-option
              v-for="acc in accounts"
              :key="acc.id"
              :label="acc.remark || acc.phone"
              :value="acc.id"
            />
          </el-select>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="immediateExchangeDialogVisible = false">取消</el-button>
        <el-button type="success" @click="confirmImmediateExchange">立即兑换</el-button>
      </template>
    </el-dialog>

    <!-- 兑换账号对话框（新增/编辑） -->
    <el-dialog 
      v-model="accountDialogVisible" 
      :title="isEditingAccount ? '编辑兑换账号' : '添加兑换账号'" 
      :width="accountDialogWidth"
      :top="accountDialogTop"
      class="account-dialog"
      :close-on-click-modal="false"
    >
      <div class="dialog-content">
        <!-- 基本信息卡片 -->
        <div class="form-section">
          <div class="section-title">
            <el-icon><User /></el-icon>
            <span>基本信息</span>
          </div>
          <el-form :model="accountForm" label-position="top">
            <el-row :gutter="16">
              <el-col :span="isMobile ? 24 : 12">
                <el-form-item label="云盘账号" required>
                  <!-- 普通用户：使用普通下拉选择 -->
                  <el-select 
                    v-if="!isAdmin"
                    v-model="accountForm.account_id" 
                    placeholder="请选择云盘账号" 
                    style="width: 100%;" 
                    :disabled="isEditingAccount"
                    :loading="userAccountsLoading"
                    :no-data-text="userAccountsLoading ? '正在加载云盘账号...' : '暂无云盘账号，请先到账号页面添加'"
                    :size="accountDialogControlSize"
                  >
                    <el-option
                      v-for="acc in userAccounts"
                      :key="acc.id"
                      :label="acc.remark ? `${acc.remark} (${acc.phone})` : acc.phone"
                      :value="acc.id"
                    />
                  </el-select>
                  <!-- 管理员：使用可搜索下拉选择 -->
                  <el-select 
                    v-else
                    v-model="accountForm.account_id" 
                    placeholder="搜索并选择云盘账号" 
                    style="width: 100%;" 
                    :disabled="isEditingAccount"
                    :size="accountDialogControlSize"
                    filterable
                    remote
                    :remote-method="searchAccounts"
                    :loading="accountSearchLoading"
                  >
                    <el-option
                      v-for="acc in allAccountsSearchResults"
                      :key="acc.id"
                      :label="acc.remark ? `${acc.remark} (${acc.phone}) [${acc.username}]` : `${acc.phone} [${acc.username}]`"
                      :value="acc.id"
                    />
                  </el-select>
                </el-form-item>
              </el-col>
              <el-col :span="isMobile ? 24 : 12">
                <el-form-item label="选择商品" required>
                  <el-select 
                    v-model="accountForm.product_id" 
                    placeholder="选择要兑换的商品" 
                    style="width: 100%;" 
                    filterable 
                    :size="accountDialogControlSize"
                  >
                    <el-option
                      v-for="product in products"
                      :key="product.id"
                      :label="`${product.prize_name} (${product.p_order}云朵)`"
                      :value="product.id"
                    />
                  </el-select>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item label="备注">
              <el-input 
                v-model="accountForm.remark" 
                placeholder="给这个账号添加备注（可选）"
                :size="accountDialogControlSize"
              />
            </el-form-item>
          </el-form>
        </div>

        <!-- 抢兑时间卡片 -->
        <div class="form-section">
          <div class="section-title">
            <el-icon><Timer /></el-icon>
            <span>抢兑时间设置</span>
          </div>
          <el-form :model="accountForm" label-position="top">
            <el-row :gutter="16">
              <el-col :span="isMobile ? 24 : 12">
                <el-form-item label="第一次抢兑" required>
                  <el-time-picker
                    v-model="accountForm.exchange_time_1"
                    format="HH:mm"
                    value-format="HH:mm:ss"
                    placeholder="选择时间"
                    style="width: 100%;"
                    :size="accountDialogControlSize"
                  />
                </el-form-item>
              </el-col>
              <el-col :span="isMobile ? 24 : 12">
                <el-form-item label="第二次抢兑" required>
                  <el-time-picker
                    v-model="accountForm.exchange_time_2"
                    format="HH:mm"
                    value-format="HH:mm:ss"
                    placeholder="选择时间"
                    style="width: 100%;"
                    :size="accountDialogControlSize"
                  />
                </el-form-item>
              </el-col>
            </el-row>
          </el-form>
        </div>

        <!-- 状态设置 -->
        <div class="form-section" v-if="isEditingAccount">
          <div class="section-title">
            <el-icon><Setting /></el-icon>
            <span>账号状态</span>
          </div>
          <el-form :model="accountForm" label-position="top">
            <el-form-item>
              <el-switch
                v-model="accountForm.is_active"
                active-text="启用抢兑"
                inactive-text="暂停抢兑"
                :size="accountDialogControlSize"
              />
            </el-form-item>
          </el-form>
        </div>
      </div>
      <template #footer>
        <div class="dialog-footer">
          <el-button @click="accountDialogVisible = false" :size="accountDialogControlSize">取消</el-button>
          <el-button type="primary" @click="saveAccount" :size="accountDialogControlSize">确定</el-button>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Present, Plus } from '@element-plus/icons-vue'
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
  totalFail,
  getStockPercentage,
  getStockStatus
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
.page-header { background: rgba(255,255,255,.78); backdrop-filter: blur(18px); -webkit-backdrop-filter: blur(18px); border-radius: 24px; padding: clamp(18px, 2.4vw, 26px); margin-bottom: 18px; box-shadow: 0 18px 42px rgba(37,99,235,.1); border: 1px solid rgba(255,255,255,.76); }
.header-main { display:flex; justify-content:space-between; align-items:flex-start; gap:16px; margin-bottom:18px; }
.header-title-section { display:flex; align-items:center; gap:14px; }
.header-text { display:flex; flex-direction:column; gap:4px; }
.header-text .title { font-size: clamp(22px, 3vw, 28px); font-weight:700; color:#1e3a8a; }
.header-text .subtitle { font-size:13px; color:#3b82f6; }
.header-actions { display:flex; flex-wrap:wrap; gap:10px; }
.stats-row { display:grid; grid-template-columns: repeat(4, minmax(0,1fr)); gap:14px; }
.stat-item { display:flex; align-items:center; gap:12px; min-width:0; padding:16px 18px; background: rgba(255,255,255,.62); border-radius:18px; border:1px solid rgba(255,255,255,.76); box-shadow: 0 10px 24px rgba(37,99,235,.08); }
.stat-icon-bg { width:48px; height:48px; border-radius:14px; display:flex; align-items:center; justify-content:center; font-size:20px; flex-shrink:0; }
.stat-icon-bg.blue { background: rgba(219,234,254,.9); color:#2563eb; }
.stat-icon-bg.indigo { background: rgba(224,231,255,.9); color:#4f46e5; }
.stat-icon-bg.success { background: rgba(209,250,229,.9); color:#059669; }
.stat-icon-bg.danger { background: rgba(254,226,226,.9); color:#dc2626; }
.stat-content { display:flex; flex-direction:column; gap:2px; min-width:0; }
.stat-num { font-size: clamp(22px, 2.6vw, 28px); font-weight:800; color:#1e3a8a; line-height:1; }
.stat-name { font-size:13px; color:#3b82f6; }
.content { background: rgba(255,255,255,.74); backdrop-filter: blur(18px); -webkit-backdrop-filter: blur(18px); border-radius:24px; padding: clamp(16px, 2vw, 24px); box-shadow: 0 18px 42px rgba(37,99,235,.1); border:1px solid rgba(255,255,255,.76); }
.exchange-tabs { min-height:0; }
.exchange-tabs :deep(.el-tabs__header) { margin:0 0 18px; }
.exchange-tabs :deep(.el-tabs__nav-wrap) { overflow-x:auto; scrollbar-width:none; }
.exchange-tabs :deep(.el-tabs__nav-wrap::-webkit-scrollbar) { display:none; }
.exchange-tabs :deep(.el-tabs__nav-scroll) { display:flex; }
.exchange-tabs :deep(.el-tabs__nav) { flex-wrap:nowrap; }
.exchange-tabs :deep(.el-tabs__item) { height:42px; padding:0 18px; font-size:14px; font-weight:600; white-space:nowrap; }
.exchange-tabs :deep(.el-tabs__content) { padding:8px 0 0; }
.product-section { display:flex; flex-direction:column; gap:16px; }
.filter-area { display:grid; grid-template-columns:minmax(240px,320px) 1fr; gap:14px 18px; align-items:start; margin-bottom:4px; padding:16px; background: rgba(255,255,255,.52); border-radius:18px; border:1px solid rgba(255,255,255,.76); }
.filter-head { display:flex; align-items:center; justify-content:space-between; gap:12px; grid-column:1 / -1; }
.filter-copy { display:flex; flex-direction:column; gap:4px; }
.filter-title { font-size:15px; font-weight:700; color:#0f172a; }
.filter-hint { font-size:12px; color:#64748b; }
.search-input { width:100%; max-width:none; }
.search-input :deep(.el-input__wrapper) { min-height:42px; border-radius:14px; box-shadow: 0 0 0 1px rgba(191,219,254,.8) inset; background: rgba(255,255,255,.84); }
.category-list { min-width:0; overflow-x:auto; padding-bottom:4px; scrollbar-width:none; }
.category-list::-webkit-scrollbar { display:none; }
.category-list :deep(.el-radio-group) { display:flex; flex-wrap:nowrap; width:max-content; }
.category-list :deep(.el-radio-button__inner) { border-radius:999px; padding:8px 16px; font-size:13px; }
.product-grid { display:grid; grid-template-columns: repeat(auto-fill, minmax(248px,1fr)); gap:18px; }
.product-grid.mobile-grid { grid-template-columns:1fr; }
.product-card { border-radius:22px; overflow:hidden; border:1px solid rgba(255,255,255,.78); background: rgba(255,255,255,.84); box-shadow: 0 16px 32px rgba(37,99,235,.09); backdrop-filter: blur(12px); transition: transform .2s ease, box-shadow .2s ease, border-color .2s ease; }
.product-card :deep(.el-card__body) { padding:14px; height:100%; }
.product-card:hover { transform: translateY(-4px); box-shadow: 0 20px 38px rgba(37,99,235,.14); border-color: rgba(59,130,246,.32); }
.product-card.sold-out { opacity:.76; }
.product-card.sold-out .product-image { filter: grayscale(100%); }
.product-layout { display:flex; flex-direction:column; height:100%; }
.product-layout.mobile { flex-direction:row; align-items:stretch; gap:12px; }
.product-image { position:relative; display:flex; align-items:center; justify-content:center; height:122px; margin:-14px -14px 14px; overflow:hidden; border-radius:22px 22px 0 0; background: linear-gradient(135deg, rgba(224,242,254,.72) 0%, rgba(239,246,255,.5) 100%); border-bottom:1px solid rgba(255,255,255,.72); }
.product-image.mobile { width:88px; height:88px; margin:0; border-radius:16px; flex-shrink:0; border-bottom:none; }
.product-img { width:100%; height:100%; object-fit:contain; padding:14px; transition:transform .2s ease; position:relative; z-index:1; }
.product-card:hover .product-img { transform:scale(1.05); }
.product-image-fallback { position:absolute; inset:0; display:flex; align-items:center; justify-content:center; background: linear-gradient(135deg, rgba(224,242,254,.52), rgba(186,230,253,.3)); z-index:0; }
.product-image.has-image .product-image-fallback { z-index:-1; }
.product-content { display:flex; flex-direction:column; gap:10px; height:100%; }
.product-content.mobile { gap:8px; }
.product-title { font-size:15px; font-weight:700; color:#1e3a8a; line-height:1.45; min-height:44px; display:-webkit-box; -webkit-line-clamp:2; -webkit-box-orient:vertical; overflow:hidden; }
.product-meta { display:flex; align-items:center; justify-content:space-between; gap:8px; flex-wrap:wrap; }
.product-price { display:inline-flex; align-items:center; gap:4px; margin-left:auto; font-size:14px; font-weight:700; color:#2563eb; background: rgba(239,246,255,.92); padding:5px 10px; border-radius:999px; border:1px solid rgba(191,219,254,.9); }
.cloud-icon { width:16px; height:16px; color:#2563eb; }
.product-stock { display:flex; flex-direction:column; gap:6px; margin-top:auto; padding:10px 12px; border-radius:14px; background: rgba(248,250,252,.84); border:1px solid rgba(226,232,240,.8); }
.stock-header { display:flex; justify-content:space-between; align-items:center; gap:12px; }
.stock-label { display:inline-flex; align-items:center; gap:4px; font-size:12px; color:#64748b; }
.stock-value { font-size:13px; font-weight:700; color:#059669; }
.stock-value.low { color:#d97706; }
.stock-value.empty { color:#dc2626; }
.stock-progress { border-radius:999px; overflow:hidden; }
.exchange-btn { width:100%; min-height:40px; border-radius:14px; font-weight:700; }
.account-section, .task-section, .rewards-section { padding-top:4px; }
.section-header { display:flex; justify-content:flex-end; margin-bottom:16px; }
.task-result { display:flex; align-items:center; gap:8px; flex-wrap:wrap; }
.result-text { color:#64748b; max-width:180px; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.text-gray { color:#94a3b8; }
.mobile-card-list { display:grid; gap:12px; }
.mobile-account-card, .mobile-task-card { border-radius:18px; border:1px solid rgba(255,255,255,.78); background: rgba(255,255,255,.84); box-shadow:0 12px 28px rgba(37,99,235,.08); }
.mobile-account-card :deep(.el-card__body), .mobile-task-card :deep(.el-card__body) { padding:16px; }
.mobile-account-header, .mobile-task-header { display:flex; justify-content:space-between; align-items:flex-start; gap:10px; margin-bottom:12px; }
.mobile-account-title, .mobile-task-title { font-size:15px; font-weight:700; color:#0f172a; }
.mobile-account-info, .mobile-task-info { display:flex; flex-direction:column; gap:8px; margin-bottom:12px; }
.mobile-account-item, .mobile-task-item { display:flex; gap:8px; font-size:13px; }
.mobile-account-item .label, .mobile-task-item .label { min-width:68px; color:#64748b; flex-shrink:0; }
.mobile-account-item .value, .mobile-task-item .value { color:#334155; flex:1; word-break:break-word; }
.mobile-account-actions, .mobile-task-actions { display:flex; gap:8px; justify-content:flex-end; flex-wrap:wrap; }
.rewards-section { min-height:320px; display:flex; align-items:center; justify-content:center; }
:deep(.el-table) { border-radius:18px; overflow:hidden; --el-table-border-color: rgba(148,163,184,.18); --el-table-header-bg-color: rgba(248,250,252,.9); --el-table-row-hover-bg-color: rgba(239,246,255,.74); }
:deep(.el-table .cell) { line-height:1.45; }
:deep(.account-dialog .el-dialog) { max-width: calc(100vw - 32px); }
:deep(.account-dialog .el-dialog__header) { margin:0; padding: clamp(14px, 1.8vh, 20px) clamp(16px, 2.2vw, 24px); border-bottom:1px solid rgba(226,232,240,.9); }
:deep(.account-dialog .el-dialog__title) { font-size:18px; font-weight:700; color:#0f172a; }
:deep(.account-dialog .el-dialog__body) { padding:0; }
:deep(.account-dialog .el-dialog__footer) { padding: clamp(12px, 1.6vh, 16px) clamp(16px, 2.2vw, 24px); border-top:1px solid rgba(226,232,240,.9); }
.dialog-content { padding: clamp(16px, 2vw, 22px); }
.form-section { background: rgba(248,250,252,.9); border-radius:18px; padding: clamp(14px, 1.8vh, 18px); margin-bottom:14px; border:1px solid rgba(226,232,240,.74); }
.form-section:last-child { margin-bottom:0; }
.section-title { display:flex; align-items:center; gap:8px; font-size:15px; font-weight:700; color:#0f172a; margin-bottom:14px; }
.section-title .el-icon { color:#3b82f6; }
.dialog-footer { display:flex; justify-content:flex-end; gap:12px; }
:deep(.account-dialog .el-form-item__label) { font-weight:600; color:#475569; }
:deep(.account-dialog .el-input__wrapper), :deep(.account-dialog .el-select .el-input__wrapper) { min-height:40px; border-radius:12px; box-shadow: 0 0 0 1px rgba(203,213,225,.9) inset; }
@media (max-width: 1280px) { .exchange-center { padding:14px; } .header-main { flex-wrap:wrap; } .stats-row { grid-template-columns: repeat(2, minmax(0,1fr)); } .filter-area { grid-template-columns:1fr; } .product-grid { grid-template-columns: repeat(auto-fill, minmax(228px,1fr)); } }
@media (max-width: 768px) { .exchange-center { padding:0; } .page-header, .content { border-radius:20px; } .page-header { padding:16px; } .header-main { margin-bottom:14px; } .header-actions { width:100%; } .header-actions :deep(.el-button) { width:100%; } .stats-row { grid-template-columns: repeat(2, minmax(0,1fr)); gap:10px; } .stat-item { padding:14px; border-radius:16px; } .stat-icon-bg { width:42px; height:42px; } .content { padding:14px; } .exchange-tabs :deep(.el-tabs__item) { padding:0 14px; font-size:13px; } .filter-area { padding:14px; gap:12px; } .filter-head { align-items:flex-start; flex-direction:column; } .product-layout.mobile { flex-direction:row; } .product-image.mobile { width:84px; height:84px; } .product-title { min-height:auto; font-size:14px; } .section-header { justify-content:stretch; } .section-header :deep(.el-button) { width:100%; } .dialog-content { padding:16px; } .dialog-footer { gap:10px; } }
@media (max-width: 520px) { .stats-row { grid-template-columns:1fr; } .mobile-account-actions, .mobile-task-actions, .dialog-footer { flex-direction:column; } .mobile-account-actions :deep(.el-button), .mobile-task-actions :deep(.el-button), .dialog-footer :deep(.el-button) { width:100%; } }
</style>






