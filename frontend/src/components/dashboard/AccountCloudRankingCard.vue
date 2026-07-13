<template>
  <el-card
    shadow="hover"
    class="chart-card"
  >
    <template #header>
      <div class="card-header">
        <span>{{ isAdmin ? '全局账号云朵排名' : '账号云朵排名' }}</span>
      </div>
    </template>
    <div class="ranking-wrapper">
      <el-table
        :data="ranking"
        stripe
        size="small"
        style="width: 100%"
        class="ranking-table"
      >
        <el-table-column
          type="index"
          label="#"
          width="40"
          align="center"
        />
        <el-table-column
          label="手机号"
          min-width="100"
        >
          <template #default="{ row }">
            <span class="nowrap">{{ maskPhone(row.phone) }}</span>
          </template>
        </el-table-column>
        <el-table-column
          prop="remark"
          label="备注"
          min-width="60"
          show-overflow-tooltip
        />
        <el-table-column
          prop="cloud_count"
          label="云朵"
          width="65"
          align="right"
        />
        <el-table-column
          v-if="isAdmin"
          label="今日"
          width="60"
          align="right"
        >
          <template #default="{ row }">
            <span
              v-if="Number(row.today_gained || 0) > 0"
              style="color: #10b981"
            >+{{ row.today_gained }}</span>
            <span
              v-else
              style="color: #999"
            >0</span>
          </template>
        </el-table-column>
      </el-table>
    </div>
  </el-card>
</template>

<script setup lang="ts">
export interface AccountCloudRankingRow {
  phone: string
  remark?: string
  cloud_count: number
  today_gained?: number
}

defineProps<{
  ranking: AccountCloudRankingRow[]
  isAdmin: boolean
}>()

const maskPhone = (phone: string) => {
  if (!phone || phone.length < 7) return phone
  return phone.slice(0, 3) + '****' + phone.slice(-4)
}
</script>

<style scoped>
.chart-card {
  height: 100%;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  color: #1e40af;
  font-weight: 600;
}

.ranking-table {
  font-size: 13px;
}

.ranking-table .nowrap {
  white-space: nowrap;
}

:deep(.ranking-table .el-table__cell) {
  padding: 6px 0;
}

.ranking-wrapper {
  height: 350px;
  overflow-y: auto;
}
</style>
