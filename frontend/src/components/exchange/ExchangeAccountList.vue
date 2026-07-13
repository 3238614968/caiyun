<template>
  <div class="account-section">
    <div class="section-header">
      <el-button
        type="primary"
        :size="isMobile ? 'small' : 'default'"
        icon="Plus"
        @click="$emit('add')"
      >
        添加抢兑规则
      </el-button>
    </div>

    <template v-if="isMobile">
      <div class="mobile-card-list">
        <el-card
          v-for="acc in accounts"
          :key="acc.id"
          class="mobile-account-card"
          shadow="hover"
        >
          <div class="mobile-account-header">
            <span class="mobile-account-title">{{ acc.remark || '未命名规则' }}</span>
            <el-tag
              :type="acc.is_active ? 'success' : 'danger'"
              size="small"
            >
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
            <el-button
              size="small"
              @click="$emit('edit', acc)"
            >
              编辑
            </el-button>
            <el-button
              size="small"
              type="danger"
              @click="$emit('delete', acc.id)"
            >
              删除
            </el-button>
          </div>
        </el-card>
      </div>
    </template>

    <el-table
      v-else
      :data="accounts"
      border
      stripe
    >
      <el-table-column
        prop="remark"
        label="备注"
        min-width="100"
      />
      <el-table-column
        prop="phone"
        label="手机号"
        min-width="110"
      />
      <el-table-column
        prop="exchange_time_1"
        label="第一次抢兑"
        width="100"
      />
      <el-table-column
        prop="exchange_time_2"
        label="第二次抢兑"
        width="100"
      />
      <el-table-column
        label="状态"
        width="70"
      >
        <template #default="{ row }">
          <el-tag
            :type="row.is_active ? 'success' : 'danger'"
            size="small"
          >
            {{ row.is_active ? '启用' : '禁用' }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column
        label="操作"
        width="150"
        fixed="right"
      >
        <template #default="{ row }">
          <el-button
            size="small"
            @click="$emit('edit', row)"
          >
            编辑
          </el-button>
          <el-button
            size="small"
            type="danger"
            @click="$emit('delete', row.id)"
          >
            删除
          </el-button>
        </template>
      </el-table-column>
    </el-table>
  </div>
</template>

<script setup lang="ts">
defineProps<{
  isMobile: boolean
  accounts: any[]
}>()

defineEmits<{
  add: []
  edit: [account: any]
  delete: [id: number]
}>()
</script>

<style scoped>
.account-section { padding-top:0; display:flex; flex-direction:column; gap:10px; }
.section-header { display:flex; justify-content:flex-end; align-items:center; min-height:34px; margin-bottom:0; }
.mobile-card-list { display:grid; gap:12px; }
.mobile-account-card { border-radius:18px; border:1px solid rgba(255,255,255,.78); background: rgba(255,255,255,.84); box-shadow:0 12px 28px rgba(37,99,235,.08); }
.mobile-account-card :deep(.el-card__body) { padding:16px; }
.mobile-account-header { display:flex; justify-content:space-between; align-items:flex-start; gap:10px; margin-bottom:12px; }
.mobile-account-title { font-size:15px; font-weight:700; color:#0f172a; }
.mobile-account-info { display:flex; flex-direction:column; gap:8px; margin-bottom:12px; }
.mobile-account-item { display:flex; gap:8px; font-size:13px; }
.mobile-account-item .label { min-width:68px; color:#64748b; flex-shrink:0; }
.mobile-account-item .value { color:#334155; flex:1; word-break:break-word; }
.mobile-account-actions { display:flex; gap:8px; justify-content:flex-end; flex-wrap:wrap; }
@media (max-width: 768px) {
  .section-header { justify-content:stretch; }
  .section-header :deep(.el-button) { width:100%; }
}
@media (max-width: 520px) {
  .mobile-account-actions { flex-direction:column; }
  .mobile-account-actions :deep(.el-button) { width:100%; }
}
</style>
