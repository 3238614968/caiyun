<template>
  <div class="exchange-records">
    <!-- 页面标题 -->
    <page-header 
      title="抢兑历史记录" 
      subtitle="查看抢兑任务执行结果"
    />

    <div class="content">
      <!-- 筛选条件 -->
      <el-card class="filter-card" shadow="hover">
        <el-form :model="filterForm" label-width="100px" size="small">
          <el-row :gutter="20">
            <el-col :span="6">
              <el-form-item label="时间范围">
                <el-date-picker
                  v-model="dateRange"
                  type="daterange"
                  range-separator="至"
                  start-placeholder="开始日期"
                  end-placeholder="结束日期"
                  value-format="YYYY-MM-DD"
                  style="width: 100%;"
                />
              </el-form-item>
            </el-col>
            <el-col :span="5">
              <el-form-item label="账号">
                <el-select v-model="filterForm.account_id" placeholder="选择账号" clearable>
                  <el-option
                    v-for="acc in accounts"
                    :key="acc.id"
                    :label="acc.remark || acc.phone"
                    :value="acc.id"
                  />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="5">
              <el-form-item label="商品">
                <el-input v-model="filterForm.product_name" placeholder="商品名称" clearable />
              </el-form-item>
            </el-col>
            <el-col :span="4">
              <el-form-item label="状态">
                <el-select v-model="filterForm.status" placeholder="全部状态" clearable>
                  <el-option label="成功" value="success" />
                  <el-option label="失败" value="failed" />
                </el-select>
              </el-form-item>
            </el-col>
            <el-col :span="4">
              <el-form-item>
                <el-button type="primary" icon="Search" @click="loadRecords">查询</el-button>
                <el-button icon="Refresh" @click="resetFilter">重置</el-button>
                <el-button type="success" icon="Download" @click="exportCurrentRecords">导出</el-button>
              </el-form-item>
            </el-col>
          </el-row>
        </el-form>
      </el-card>

      <!-- 统计卡片 -->
      <el-row :gutter="20" class="mb-4 mt-4">
        <el-col :span="6">
          <stat-card label="总记录数" :value="total" icon="Document" color="#409EFF" />
        </el-col>
        <el-col :span="6">
          <stat-card label="成功次数" :value="stats.success" icon="Success" color="#67C23A" />
        </el-col>
        <el-col :span="6">
          <stat-card label="失败次数" :value="stats.failed" icon="Error" color="#F56C6C" />
        </el-col>
        <el-col :span="6">
          <stat-card label="成功率" :value="successRate + '%'" icon="PieChart" color="#E6A23C" />
        </el-col>
      </el-row>

      <!-- 数据表格 -->
      <el-card shadow="hover">
        <el-table 
          :data="records" 
          v-loading="loading"
          border
          stripe
          style="width: 100%;"
        >
          <el-table-column prop="id" label="ID" width="80" />
          <el-table-column label="抢兑时间" width="180">
            <template #default="{ row }">
              {{ formatTime(row.created_at) }}
            </template>
          </el-table-column>
          <el-table-column label="账号" width="150">
            <template #default="{ row }">
              {{ row.exchange_account?.remark || row.exchange_account?.phone || '账号' + row.exchange_account_id }}
            </template>
          </el-table-column>
          <el-table-column prop="prize_name" label="商品名称" min-width="200" />
          <el-table-column label="云朵消耗" width="100">
            <template #default="{ row }">
              <el-tag type="warning">{{ row.product?.p_order || 0 }}云朵</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default="{ row }">
              <el-tag :type="row.status === 'success' ? 'success' : 'danger'">
                {{ row.status === 'success' ? '成功' : '失败' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="结果消息" min-width="160">
            <template #default="{ row }">
              <el-tag :type="formatExchangeResult(row.message, row.status).type" size="small">
                {{ formatExchangeResult(row.message, row.status).label }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="执行耗时" width="100">
            <template #default="{ row }">
              <span :class="getDurationClass(row.execution_time_ms)">
                {{ row.execution_time_ms }}ms
              </span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button size="small" type="text" @click="showDetail(row)">详情</el-button>
              <el-button size="small" type="text" @click="exportRecord(row)">导出</el-button>
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
          @size-change="loadRecords"
          @current-change="loadRecords"
          style="margin-top: 20px; justify-content: flex-end;"
        />
      </el-card>
    </div>

    <!-- 详情对话框 -->
    <el-dialog v-model="detailVisible" title="抢兑详情" width="600px">
      <el-descriptions :column="2" border v-if="selectedRecord">
        <el-descriptions-item label="记录 ID">{{ selectedRecord.id }}</el-descriptions-item>
        <el-descriptions-item label="抢兑时间">{{ formatTime(selectedRecord.created_at) }}</el-descriptions-item>
        <el-descriptions-item label="账号">{{ selectedRecord.exchange_account?.remark || selectedRecord.exchange_account?.phone }}</el-descriptions-item>
        <el-descriptions-item label="商品">{{ selectedRecord.prize_name }}</el-descriptions-item>
        <el-descriptions-item label="云朵消耗">{{ selectedRecord.product?.p_order }}云朵</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="selectedRecord.status === 'success' ? 'success' : 'danger'">
            {{ selectedRecord.status === 'success' ? '成功' : '失败' }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="执行耗时" :span="2">{{ selectedRecord.execution_time_ms }}ms</el-descriptions-item>
        <el-descriptions-item label="结果消息" :span="2">
          <el-tag :type="formatExchangeResult(selectedRecord.message, selectedRecord.status).type">
            {{ formatExchangeResult(selectedRecord.message, selectedRecord.status).label }}
          </el-tag>
        </el-descriptions-item>
      </el-descriptions>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import '@/styles/element/records'
import { ref, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import PageHeader from '@/components/PageHeader.vue'
import StatCard from '@/components/StatCard.vue'
import { getExchangeRecords, getExchangeAccounts, exportExchangeRecords, type ExchangeRecord } from '@/api/exchange'
import { formatExchangeResult } from '@/utils/exchange-result'

// 状态
const loading = ref(false)
const records = ref<ExchangeRecord[]>([])
const accounts = ref<any[]>([])
const total = ref(0)
const stats = ref({ success: 0, failed: 0 })

// 筛选表单
const dateRange = ref<[string, string] | null>(null)
const filterForm = ref({
  account_id: 0,
  product_name: '',
  status: ''
})

// 分页
const pagination = ref({
  page: 1,
  pageSize: 20,
  total: 0
})

// 详情对话框
const detailVisible = ref(false)
const selectedRecord = ref<ExchangeRecord | null>(null)

// 计算属性
const successRate = computed(() => {
  if (total.value === 0) return 0
  return ((stats.value.success / total.value) * 100).toFixed(1)
})

// 方法
const loadRecords = async () => {
  loading.value = true
  try {
    const params: any = {
      page: pagination.value.page,
      limit: pagination.value.pageSize,
      ...filterForm.value
    }

    if (dateRange.value) {
      params.start_date = dateRange.value[0]
      params.end_date = dateRange.value[1]
    }

    const res = await getExchangeRecords(params)
    records.value = res.records || []
    total.value = res.total || 0
    pagination.value.total = res.total || 0
    
    // 更新统计
    stats.value.success = res.stats?.success || 0
    stats.value.failed = res.stats?.failed || 0
  } catch (error: any) {
    ElMessage.error('加载记录失败：' + error.message)
  } finally {
    loading.value = false
  }
}

const resetFilter = () => {
  dateRange.value = null
  filterForm.value = {
    account_id: 0,
    product_name: '',
    status: ''
  }
  pagination.value.page = 1
  loadRecords()
}

const loadAccounts = async () => {
  try {
    const res = await getExchangeAccounts()
    accounts.value = res.accounts || []
  } catch (error: any) {
    console.error('加载账号失败:', error)
  }
}

const showDetail = (record: ExchangeRecord) => {
  selectedRecord.value = record
  detailVisible.value = true
}

const buildExportParams = () => {
  const params: Record<string, any> = {
    account_id: filterForm.value.account_id || undefined,
    product_name: filterForm.value.product_name || undefined,
    status: filterForm.value.status || undefined
  }

  if (dateRange.value) {
    params.start_date = dateRange.value[0]
    params.end_date = dateRange.value[1]
  }

  return params
}

const downloadBlob = (blob: Blob, filename: string) => {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  document.body.removeChild(link)
  URL.revokeObjectURL(url)
}

const exportCurrentRecords = async () => {
  try {
    const blob = await exportExchangeRecords({
      ...buildExportParams(),
      format: 'csv'
    })
    const now = new Date().toISOString().slice(0, 19).replace(/[:T]/g, '-')
    downloadBlob(blob, `exchange-records-${now}.csv`)
    ElMessage.success('导出成功')
  } catch (error: any) {
    ElMessage.error('导出失败：' + error.message)
  }
}

const exportRecord = (record: ExchangeRecord) => {
  try {
    const payload = JSON.stringify(record, null, 2)
    const blob = new Blob([payload], { type: 'application/json;charset=utf-8' })
    downloadBlob(blob, `exchange-record-${record.id}.json`)
    ElMessage.success('单条记录导出成功')
  } catch (error: any) {
    ElMessage.error('导出失败：' + error.message)
  }
}

const formatTime = (time: string) => {
  return new Date(time).toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  })
}

const getDurationClass = (duration: number) => {
  if (duration < 1000) return 'duration-fast'
  if (duration < 3000) return 'duration-normal'
  return 'duration-slow'
}

onMounted(() => {
  loadRecords()
  loadAccounts()
})
</script>

<style scoped>
.exchange-records {
  padding: 20px;
}

.content {
  background: #fff;
  border-radius: 4px;
  padding: 20px;
}

.filter-card {
  margin-bottom: 20px;
}

.duration-fast {
  color: #67C23A;
  font-weight: bold;
}

.duration-normal {
  color: #E6A23C;
  font-weight: bold;
}

.duration-slow {
  color: #F56C6C;
  font-weight: bold;
}

.mt-4 {
  margin-top: 20px;
}

.mb-4 {
  margin-bottom: 20px;
}
</style>
