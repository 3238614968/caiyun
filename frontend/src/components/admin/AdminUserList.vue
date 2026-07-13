<template>
  <div>
    <div
      v-loading="loading"
      class="responsive-data-shell"
    >
      <el-table
        v-if="!isMobile"
        :data="users"
        stripe
        style="width: 100%"
      >
        <el-table-column
          prop="username"
          label="用户名"
          width="150"
        />
        <el-table-column
          prop="email"
          label="邮箱"
        />
        <el-table-column
          prop="role"
          label="角色"
          width="120"
        >
          <template #default="{ row }">
            <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">
              {{ row.role === 'admin' ? '管理员' : '普通用户' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column
          prop="created_at"
          label="创建时间"
          width="180"
        >
          <template #default="{ row }">
            {{ formatDate(row.created_at) }}
          </template>
        </el-table-column>
        <el-table-column
          label="操作"
          width="220"
          fixed="right"
        >
          <template #default="{ row }">
            <el-button
              type="primary"
              link
              @click="$emit('editRole', row)"
            >
              修改角色
            </el-button>
            <el-button
              type="warning"
              link
              @click="$emit('resetPassword', row)"
            >
              重置密码
            </el-button>
            <el-button
              type="danger"
              link
              @click="$emit('deleteUser', row)"
            >
              删除
            </el-button>
          </template>
        </el-table-column>
      </el-table>

      <div
        v-else
        class="mobile-admin-list"
      >
        <el-empty
          v-if="users.length === 0"
          description="暂无用户数据"
        />
        <template v-else>
          <el-card
            v-for="row in users"
            :key="row.id"
            class="mobile-admin-card"
            shadow="never"
          >
            <div class="mobile-admin-card-head">
              <div>
                <div class="mobile-admin-card-title">
                  {{ row.username }}
                </div>
                <div class="mobile-admin-card-meta">
                  {{ row.email || '-' }}
                </div>
              </div>
              <el-tag :type="row.role === 'admin' ? 'danger' : 'primary'">
                {{ row.role === 'admin' ? '管理员' : '普通用户' }}
              </el-tag>
            </div>
            <div class="mobile-admin-card-grid">
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">邮箱</span>
                <span class="mobile-admin-card-value">{{ row.email || '-' }}</span>
              </div>
              <div class="mobile-admin-card-row">
                <span class="mobile-admin-card-label">创建时间</span>
                <span class="mobile-admin-card-value">{{ formatDate(row.created_at) }}</span>
              </div>
            </div>
            <div class="mobile-admin-card-actions">
              <el-button
                type="primary"
                plain
                @click="$emit('editRole', row)"
              >
                修改角色
              </el-button>
              <el-button
                type="warning"
                plain
                @click="$emit('resetPassword', row)"
              >
                重置密码
              </el-button>
              <el-button
                type="danger"
                plain
                @click="$emit('deleteUser', row)"
              >
                删除
              </el-button>
            </div>
          </el-card>
        </template>
      </div>
    </div>

    <el-pagination
      :current-page="page"
      :page-size="pageSize"
      :page-sizes="[10, 20, 50]"
      :total="total"
      layout="total, sizes, prev, pager, next"
      style="margin-top: 20px"
      @size-change="$emit('sizeChange', $event)"
      @current-change="$emit('pageChange', $event)"
    />
  </div>
</template>

<script setup lang="ts">
import type { AdminUser } from '@/api/account'

defineProps<{
  isMobile: boolean
  loading: boolean
  users: AdminUser[]
  page: number
  pageSize: number
  total: number
  formatDate: (date?: string) => string
}>()

defineEmits<{
  editRole: [row: AdminUser]
  resetPassword: [row: AdminUser]
  deleteUser: [row: AdminUser]
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
.mobile-admin-card-actions { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 8px; margin-top: 14px; padding-top: 12px; border-top: 1px solid rgba(226, 232, 240, 0.8); }
.mobile-admin-card-actions :deep(.el-button) { margin-left: 0; min-width: 0; padding-left: 6px; padding-right: 6px; }
@media (max-width: 768px) {
  .mobile-admin-card-row { grid-template-columns: 78px minmax(0, 1fr); }
  .mobile-admin-card-actions { grid-template-columns: 1fr; }
  :deep(.el-pagination) { justify-content: center; flex-wrap: wrap; gap: 8px; }
}
</style>
