<template>
  <div>
    <div
      v-loading="loading"
      class="responsive-data-shell"
    >
      <el-table
        v-if="!isMobile"
        :data="summaries"
        stripe
        style="width: 100%"
      >
        <el-table-column
          prop="phone"
          label="手机号"
          width="130"
        />
        <el-table-column
          prop="owner_username"
          label="所属用户"
          width="100"
        />
        <el-table-column
          prop="cloud_count"
          label="当前云朵"
          width="100"
        >
          <template #default="{ row }">
            <span style="font-weight: 600; color: #3b82f6">{{ row.cloud_count }}</span>
          </template>
        </el-table-column>
        <el-table-column
          prop="today_gained"
          label="今日获得"
          width="100"
        >
          <template #default="{ row }">
            <span
              v-if="row.today_gained > 0"
              style="color: #10b981"
            >+{{ row.today_gained }}</span>
            <span v-else>0</span>
          </template>
        </el-table-column>
        <el-table-column
          prop="yesterday_gained"
          label="昨日获得"
          width="100"
        >
          <template #default="{ row }">
            <span
              v-if="row.yesterday_gained > 0"
              style="color: #10b981"
            >+{{ row.yesterday_gained }}</span>
            <span v-else>0</span>
          </template>
        </el-table-column>
        <el-table-column
          label="今日任务"
          width="140"
        >
          <template #default="{ row }">
            <el-tag
              type="success"
              size="small"
            >
              成功 {{ row.success_count }}
            </el-tag>
            <el-tag
              v-if="row.failed_count > 0"
              type="danger"
              size="small"
              style="margin-left: 4px"
            >
              失败 {{ row.failed_count }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          label="状态"
          width="80"
        >
          <template #default="{ row }">
            <el-tag
              :type="row.is_active ? 'success' : 'info'"
              size="small"
            >
              {{ row.is_active ? '激活' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="last_executed_at"
          label="最后执行"
          width="160"
        >
          <template #default="{ row }">
            {{ row.last_executed_at || '-' }}
          </template>
        </el-table-column>
        <el-table-column
          prop="remark"
          label="备注"
        />
        <el-table-column
          prop="created_at"
          label="添加时间"
          width="160"
        />
      </el-table>

      <div
        v-else
        class="mobile-admin-list"
      >
        <el-empty
          v-if="summaries.length === 0"
          description="暂无账号概况"
        />
        <template v-else>
          <el-card
            v-for="row in summaries"
            :key="`${row.phone}-${row.owner_username}-${row.created_at}`"
            class="mobile-admin-card mobile-summary-card"
            shadow="never"
          >
            <div class="mobile-admin-card-head">
              <div>
                <div class="mobile-admin-card-title">
                  {{ row.phone }}
                </div>
                <div class="mobile-admin-card-meta">
                  {{ row.owner_username || '-' }}
                </div>
              </div>
              <el-tag
                :type="row.is_active ? 'success' : 'info'"
                effect="light"
              >
                {{ row.is_active ? '激活' : '停用' }}
              </el-tag>
            </div>
            <div class="mobile-admin-card-grid">
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">当前云朵</span>
                <span class="mobile-admin-card-value strong">{{ row.cloud_count }}</span>
              </div>
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">今日获得</span>
                <span class="mobile-admin-card-value success">{{ row.today_gained > 0 ? `+${row.today_gained}` : '0' }}</span>
              </div>
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">昨日获得</span>
                <span class="mobile-admin-card-value">{{ row.yesterday_gained > 0 ? `+${row.yesterday_gained}` : '0' }}</span>
              </div>
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">最后执行</span>
                <span class="mobile-admin-card-value">{{ row.last_executed_at || '-' }}</span>
              </div>
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">备注</span>
                <span class="mobile-admin-card-value">{{ row.remark || '-' }}</span>
              </div>
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">添加时间</span>
                <span class="mobile-admin-card-value">{{ row.created_at }}</span>
              </div>
            </div>
            <div class="mobile-admin-inline-tags">
              <el-tag
                type="success"
                size="small"
              >
                成功 {{ row.success_count }}
              </el-tag>
              <el-tag
                v-if="row.failed_count > 0"
                type="danger"
                size="small"
              >
                失败 {{ row.failed_count }}
              </el-tag>
            </div>
          </el-card>
        </template>
      </div>
    </div>

    <el-pagination
      :current-page="page"
      :page-size="pageSize"
      :page-sizes="[20, 50, 100]"
      :total="total"
      layout="total, sizes, prev, pager, next"
      style="margin-top: 20px"
      @current-change="$emit('pageChange', $event)"
      @size-change="$emit('sizeChange', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import type { AccountSummary } from '@/api/account'

defineProps<{
  isMobile: boolean
  loading: boolean
  summaries: AccountSummary[]
  page: number
  pageSize: number
  total: number
}>()

defineEmits<{
  pageChange: [page: number]
  sizeChange: [size: number]
}>()
</script>

<style scoped>
.responsive-data-shell { min-height: 120px; }
.mobile-admin-list { display: grid; gap: 12px; }
.mobile-admin-card { border-radius: 18px; border: 1px solid rgba(226, 232, 240, 0.9); background: rgba(255, 255, 255, 0.94); box-shadow: 0 14px 28px rgba(37, 99, 235, 0.08); }
.mobile-admin-card :deep(.el-card__body) { padding: 16px; }
.mobile-admin-card-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; margin-bottom: 12px; }
.mobile-admin-card-title { font-size: 15px; font-weight: 700; color: #0f172a; line-height: 1.4; word-break: break-word; }
.mobile-admin-card-meta { margin-top: 4px; font-size: 12px; color: #64748b; }
.mobile-admin-card-grid { display: grid; gap: 10px; }
.mobile-admin-card-row { display: grid; grid-template-columns: 84px minmax(0, 1fr); gap: 10px; align-items: start; }
.mobile-admin-card-label { font-size: 12px; color: #64748b; font-weight: 600; }
.mobile-admin-card-value { font-size: 13px; color: #334155; word-break: break-word; }
.mobile-admin-card-value.strong { font-weight: 700; color: #2563eb; }
.mobile-admin-card-value.success { color: #059669; font-weight: 600; }
.mobile-admin-inline-tags { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 10px; }
@media (max-width: 768px) { .mobile-admin-card-row { grid-template-columns: 78px minmax(0, 1fr); } }
</style>
