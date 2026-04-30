<template>
  <div class="account-manage-container">
    <el-card shadow="hover">
      <template #header>
        <div class="card-header">
          <span>账号管理</span>
          <el-button type="primary" @click="showAddDialog">
            <el-icon><Plus /></el-icon>
            添加账号
          </el-button>
        </div>
      </template>

      <!-- 搜索栏 -->
      <el-form :inline="true" :model="searchForm" class="search-form">
        <el-form-item label="手机号">
          <el-input v-model="searchForm.phone" placeholder="请输入手机号" clearable />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" @click="handleSearch">搜索</el-button>
          <el-button @click="handleReset">重置</el-button>
        </el-form-item>
      </el-form>

      <!-- 账号列表 -->
      <el-table :data="accountList" stripe v-loading="loading" style="width: 100%">
        <el-table-column prop="phone" label="手机号" width="150" />
        <el-table-column prop="cloud_count" label="云朵数" width="100" />
        <el-table-column prop="remark" label="备注" />
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-switch
              v-model="row.is_active"
              @change="handleStatusChange(row)"
            />
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="180">
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="{ row }">
            <div class="action-buttons">
              <el-button type="primary" link @click="handleView(row)">查看</el-button>
              <el-button type="primary" link @click="handleEdit(row)">编辑</el-button>
              <el-button type="warning" link @click="handleTriggerTask(row)" :loading="row.executing">
                执行任务
              </el-button>
              <el-button type="danger" link @click="handleDelete(row)">删除</el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>

      <!-- 分页 -->
      <el-pagination
        v-model:current-page="pagination.page"
        v-model:page-size="pagination.pageSize"
        :page-sizes="[10, 20, 50, 100]"
        :total="pagination.total"
        layout="total, sizes, prev, pager, next, jumper"
        @size-change="handleSizeChange"
        @current-change="handleCurrentChange"
        style="margin-top: 20px"
      />
    </el-card>

    <!-- 添加/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="500px"
      @close="handleDialogClose"
    >
      <!-- 添加模式：显示登录方式选择 -->
      <el-tabs v-if="!isEditMode" v-model="loginMode" class="login-tabs">
        <el-tab-pane label="CK登录" name="ck">
          <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
            <el-form-item label="手机号" prop="phone">
              <el-input v-model="form.phone" placeholder="请输入手机号" clearable />
            </el-form-item>
            <el-form-item label="Auth" prop="auth">
              <el-input
                v-model="form.auth"
                type="textarea"
                :rows="4"
                placeholder="请输入Auth"
              />
            </el-form-item>
            <el-form-item label="备注" prop="remark">
              <el-input v-model="form.remark" placeholder="请输入备注" clearable />
            </el-form-item>
          </el-form>
        </el-tab-pane>
        <el-tab-pane label="短信登录" name="sms">
          <el-form ref="smsFormRef" :model="smsForm" :rules="smsRules" label-width="80px">
            <el-form-item label="手机号" prop="phone">
              <el-input v-model="smsForm.phone" placeholder="请输入11位手机号" clearable />
            </el-form-item>
            <el-form-item label="验证码" prop="smsCode">
              <div style="display: flex; gap: 8px;">
                <el-input v-model="smsForm.smsCode" placeholder="请输入验证码" clearable />
                <el-button
                  type="primary"
                  @click="handleSendSms"
                  :loading="smsSending"
                  :disabled="smsCountdown > 0"
                  style="min-width: 110px;"
                >
                  {{ smsCountdown > 0 ? `${smsCountdown}s后重发` : '发送验证码' }}
                </el-button>
              </div>
            </el-form-item>
            <el-form-item label="备注" prop="remark">
              <el-input v-model="smsForm.remark" placeholder="请输入备注" clearable />
            </el-form-item>
          </el-form>
        </el-tab-pane>
      </el-tabs>

      <!-- 编辑模式：直接显示表单 -->
      <el-form v-else ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" placeholder="请输入手机号" clearable />
        </el-form-item>
        <el-form-item label="Auth" prop="auth">
          <el-input
            v-model="form.auth"
            type="textarea"
            :rows="4"
            placeholder="留空表示不修改Auth"
          />
        </el-form-item>
        <el-form-item label="备注" prop="remark">
          <el-input v-model="form.remark" placeholder="请输入备注" clearable />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleSubmit" :loading="submitting">
          确定
        </el-button>
      </template>
    </el-dialog>

    <!-- 查看详情对话框 -->
    <el-dialog v-model="detailVisible" title="账号详情" width="600px">
      <el-descriptions :column="2" border>
        <el-descriptions-item label="手机号">{{ currentAccount.phone }}</el-descriptions-item>
        <el-descriptions-item label="云朵数">{{ currentAccount.cloud_count }}</el-descriptions-item>
        <el-descriptions-item label="平台">{{ currentAccount.platform }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="currentAccount.is_active ? 'success' : 'danger'">
            {{ currentAccount.is_active ? '激活' : '停用' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="过期时间">
          {{ formatExpireTime(currentAccount.expire_at) }}
        </el-descriptions-item>
        <el-descriptions-item label="备注">{{ currentAccount.remark || '-' }}</el-descriptions-item>
        <el-descriptions-item label="创建时间" :span="2">
          {{ formatDate(currentAccount.created_at) }}
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import type { FormInstance, FormRules } from 'element-plus'
import {
  getAccounts,
  createAccount,
  updateAccount,
  deleteAccount,
  setAccountStatus,
  triggerAccountTask,
  sendSmsCode,
  smsLogin,
  type Account,
  type CreateAccountRequest,
  type UpdateAccountRequest
} from '../api/account'

const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const detailVisible = ref(false)
const dialogTitle = ref('添加账号')
const formRef = ref<FormInstance>()
const smsFormRef = ref<FormInstance>()
const loginMode = ref('ck')
const isEditMode = ref(false)

// 短信登录相关状态
const smsSending = ref(false)
const smsCountdown = ref(0)
let smsTimer: ReturnType<typeof setInterval> | null = null

const smsForm = reactive({
  phone: '',
  smsCode: '',
  taskId: '',
  remark: '',
  smsStatus: 'idle' as 'idle' | 'processing' | 'completed' | 'failed' | 'timeout'
})

const smsRules = reactive<FormRules>({
  phone: [
    { required: true, message: '请输入手机号', trigger: 'blur' },
    { pattern: /^1\d{10}$/, message: '手机号格式不正确', trigger: 'blur' }
  ],
  smsCode: [
    { required: true, message: '请输入验证码', trigger: 'blur' }
  ]
})

const clearSmsCountdown = () => {
  if (smsTimer) {
    clearInterval(smsTimer)
    smsTimer = null
  }
  smsCountdown.value = 0
}

const startSmsCountdown = (seconds = 60) => {
  clearSmsCountdown()
  smsCountdown.value = seconds
  smsTimer = setInterval(() => {
    if (smsCountdown.value <= 1) {
      clearSmsCountdown()
      return
    }
    smsCountdown.value -= 1
  }, 1000)
}

const resetSmsFlow = (status: 'idle' | 'processing' | 'completed' | 'failed' | 'timeout' = 'idle') => {
  clearSmsCountdown()
  smsSending.value = false
  smsForm.taskId = ''
  smsForm.smsStatus = status
}

const accountList = ref<Account[]>([])
const currentAccount = ref<Account>({} as Account)

const searchForm = reactive({
  phone: ''
})

const pagination = reactive({
  page: 1,
  pageSize: 10,
  total: 0
})

const form = reactive<CreateAccountRequest | UpdateAccountRequest>({
  phone: '',
  auth: '',
  remark: ''
})

const rules = reactive<FormRules>({
  phone: [
    { required: true, message: '请输入手机号', trigger: 'blur' }
  ],
  auth: [
    {
      validator: (_rule, value, callback) => {
        if (!isEditMode.value && !value) {
          callback(new Error('请输入Auth'))
          return
        }
        callback()
      },
      trigger: 'blur'
    }
  ]
})

// 加载账号列表
const loadAccounts = async () => {
  loading.value = true
  try {
    const data = await getAccounts(pagination.page, pagination.pageSize, searchForm.phone)
    accountList.value = data.accounts.map(acc => ({
      ...acc,
      user: { username: acc.user?.username || '' }
    }))
    pagination.total = data.total
  } catch (error) {
    ElMessage.error('加载账号列表失败')
  } finally {
    loading.value = false
  }
}

// 搜索
const handleSearch = () => {
  pagination.page = 1
  loadAccounts()
}

// 重置
const handleReset = () => {
  searchForm.phone = ''
  pagination.page = 1
  loadAccounts()
}

// 分页大小变化
const handleSizeChange = (size: number) => {
  pagination.pageSize = size
  loadAccounts()
}

// 当前页变化
const handleCurrentChange = (page: number) => {
  pagination.page = page
  loadAccounts()
}

// 显示添加对话框
const showAddDialog = () => {
  resetSmsFlow()
  dialogTitle.value = '添加账号'
  isEditMode.value = false
  loginMode.value = 'ck'
  form.phone = ''
  form.auth = ''
  form.remark = ''
  smsForm.phone = ''
  smsForm.smsCode = ''
  smsForm.taskId = ''
  smsForm.remark = ''
  dialogVisible.value = true
}

// 查看详情
const handleView = (row: Account) => {
  currentAccount.value = row
  detailVisible.value = true
}

// 编辑
const handleEdit = (row: Account) => {
  resetSmsFlow()
  dialogTitle.value = '编辑账号'
  isEditMode.value = true
  form.phone = row.phone
  form.auth = ''
  form.remark = row.remark
  ;(form as any).id = row.id
  dialogVisible.value = true
}

// 状态变化
const handleStatusChange = async (row: Account) => {
  try {
    await setAccountStatus(row.id, row.is_active)
    ElMessage.success('状态更新成功')
  } catch (error) {
    ElMessage.error('状态更新失败')
    row.is_active = !row.is_active
  }
}

// 执行任务
const handleTriggerTask = async (row: Account & { executing?: boolean }) => {
  row.executing = true
  try {
    await triggerAccountTask(row.id)
    ElMessage.success('任务已提交执行')
    // 3秒后刷新账号列表以获取最新状态
    setTimeout(() => {
      loadAccounts()
    }, 3000)
  } catch (error) {
    ElMessage.error('任务提交失败')
  } finally {
    row.executing = false
  }
}

// 删除
const handleDelete = async (row: Account) => {
  try {
    await ElMessageBox.confirm('确定要删除该账号吗？', '提示', {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    })

    await deleteAccount(row.id)
    ElMessage.success('删除成功')
    loadAccounts()
  } catch (error: any) {
    if (error !== 'cancel') {
      ElMessage.error('删除失败')
    }
  }
}

// 发送短信验证码
const handleSendSms = async () => {
  if (!smsForm.phone || !/^1\d{10}$/.test(smsForm.phone)) {
    ElMessage.warning('请输入正确的手机号')
    return
  }

  resetSmsFlow('processing')
  smsForm.smsCode = ''

  try {
    smsSending.value = true
    const res = await sendSmsCode(smsForm.phone)
    smsForm.taskId = res.data?.task_id || ''
    if (!smsForm.taskId) {
      throw new Error('未获取到验证码会话ID')
    }
    smsForm.smsStatus = 'completed'
    ElMessage.success('验证码已发送，请输入验证码')
    startSmsCountdown(60)
  } catch (error) {
    resetSmsFlow('failed')
  } finally {
    smsSending.value = false
  }
}

// 提交表单
const handleSubmit = async () => {
  // 编辑模式或CK登录模式
  if (isEditMode.value || loginMode.value === 'ck') {
    if (!formRef.value) return
    await formRef.value.validate(async (valid) => {
      if (valid) {
        submitting.value = true
        try {
          const id = (form as any).id
          if (id) {
            const payload: UpdateAccountRequest = {
              phone: form.phone,
              remark: form.remark
            }
            if (form.auth) {
              payload.auth = form.auth
            }
            await updateAccount(id, payload)
            ElMessage.success('更新成功')
          } else {
            await createAccount(form as CreateAccountRequest)
            ElMessage.success('创建成功')
          }
          dialogVisible.value = false
          loadAccounts()
        } catch (error: any) {
          ElMessage.error(error.response?.data?.message || '操作失败')
        } finally {
          submitting.value = false
        }
      }
    })
  } else {
    // 短信登录模式
    if (!smsFormRef.value) return
    await smsFormRef.value.validate(async (valid) => {
      if (valid) {
        // 检查验证码是否发送成功
        if (smsForm.smsStatus !== 'completed') {
          if (smsForm.smsStatus === 'processing') {
            ElMessage.warning('验证码发送中，请稍候')
          } else if (smsForm.smsStatus === 'failed') {
            ElMessage.error('验证码发送失败，请重新发送')
          } else if (smsForm.smsStatus === 'timeout') {
            ElMessage.warning('验证码会话已超时，请重新发送')
          } else {
            ElMessage.warning('请先发送验证码')
          }
          return
        }
        
        submitting.value = true
        try {
          if (!smsForm.taskId) {
            ElMessage.warning('验证码会话不存在，请重新发送验证码')
            return
          }
          await smsLogin({
            phone: smsForm.phone,
            sms_code: smsForm.smsCode,
            task_id: smsForm.taskId,
            remark: smsForm.remark
          })
          ElMessage.success('账号创建成功')
          dialogVisible.value = false
          loadAccounts()
        } catch (error: any) {
          ElMessage.error(error.response?.data?.message || '登录失败')
        } finally {
          submitting.value = false
        }
      }
    })
  }
}

// 对话框关闭
const handleDialogClose = () => {
  formRef.value?.resetFields()
  smsFormRef.value?.resetFields()
  resetSmsFlow()
}

// 格式化日期
const formatDate = (date: string) => {
  return new Date(date).toLocaleString('zh-CN')
}

// 格式化过期时间
const formatExpireTime = (expireAt: number) => {
  if (!expireAt) return '-'
  return new Date(expireAt).toLocaleString('zh-CN')
}

onMounted(() => {
  loadAccounts()
})

onUnmounted(() => {
  resetSmsFlow()
})
</script>

<style scoped>
.account-manage-container {
  padding: 20px;
}

.account-manage-container {
  padding: 20px;
  background: linear-gradient(135deg, #f0f9ff 0%, #e0f2fe 100%);
  min-height: calc(100vh - 140px);
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.search-form {
  margin-bottom: 20px;
  padding: 20px;
  background: rgba(255, 255, 255, 0.8);
  border-radius: 12px;
  backdrop-filter: blur(10px);
}

:deep(.el-button--primary:not(.is-link):not(.is-text)) {
  background: linear-gradient(135deg, #3b82f6 0%, #0ea5e9 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(59, 130, 246, 0.3);
}

:deep(.el-button--primary:not(.is-link):not(.is-text):hover) {
  background: linear-gradient(135deg, #2563eb 0%, #0284c7 100%);
  box-shadow: 0 6px 20px rgba(59, 130, 246, 0.4);
}

:deep(.el-button.is-link) {
  background: none !important;
  box-shadow: none !important;
}

:deep(.el-card) {
  border-radius: 16px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.5);
  backdrop-filter: blur(10px);
  background: rgba(255, 255, 255, 0.9);
}

.action-buttons {
  display: flex;
  align-items: center;
  flex-wrap: nowrap;
  gap: 4px;
}

.login-tabs {
  margin-top: -10px;
}

:deep(.login-tabs .el-tabs__header) {
  margin-bottom: 16px;
}
</style>






